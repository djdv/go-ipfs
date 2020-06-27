package p9fsp

import (
	"context"
	"errors"
	"hash/fnv"
	"io"
	gopath "path"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hugelgupf/p9/fsimpl/templatefs"
	"github.com/hugelgupf/p9/p9"
	ninelib "github.com/hugelgupf/p9/p9"
	"github.com/ipfs/go-ipfs/filesystem"
	logging "github.com/ipfs/go-log"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

type path []string

func (p *path) String() string { return gopath.Join(append([]string{"/"}, *p...)...) }
func (p *path) Join(component string) string {
	return gopath.Join(append(append([]string{"/"}, *p...), component)...)
}

/*
TODO:
to handle the data and system version, as well as ".."
we need to store the FID's parent on it during steps
an FID will also have knowledge of its children
map[name]version{data;path}
when UnlinkAt is called, directly modify fid.children[name].path++
when WriteAt is called, indirectly modify parent, fid.parent.children[myName].data++
references will only fall out of scope when nobody is walking those paths
when the references fall out of scope, they'll be inherently reset
if a child doesn't exist then nobody walked there, if it does, re-use it
*/

// NOTE: do not use this structure's methods as a valid 9P server directly
// it is upgraded to one via `ninelib.NewServer`
// operations depend on the library to track and validate references and state
type fid struct {
	templatefs.NoopFile // TODO remove
	sync.RWMutex

	intf filesystem.Interface // interface between 9P and the target API

	log logging.EventLogger // general operations log

	path          path      // the path this FID is currently at
	filesWritable bool      // switch for metadata fields and operation avilability
	initTime      time.Time // artificial file time signatures

	ninelib.QID // 9P metadata about this file

	filesystem.File      // when opened, this FID will assign to one of these slots
	filesystem.Directory // depending on the QID type its path references
}

func (f *fid) template() *fid {
	return &fid{
		intf:          f.intf,
		log:           f.log,
		filesWritable: f.filesWritable,
		initTime:      f.initTime,
	}
}

func (f *fid) name() string { return f.path[len(f.path)-1] }

func (f *fid) Attach() (p9.File, error) {
	f.log.Debugf("Attach")

	if len(f.path) != 0 {
		f.log.Errorf("attach called on %q", f.path.String())
		return nil, errors.New("FID is not a root reference")
	}

	newFid := f.template()
	newFid.QID = ninelib.QID{Type: ninelib.TypeDir}

	return newFid, nil
}

// NOTE: the server should guarantee that we won't walk a path that's already been traversed
// same goes for empty walk messages (clone requests)
// While an FID is still referenced; the server will return the existing reference from a previous call
// implying each endpoint can be considered/constructed fresh
func (f *fid) Walk(components []string) ([]ninelib.QID, ninelib.File, error) {
	if len(components) == 0 {
		f.log.Debugf("Walk: %s", f.path.String())
		return []ninelib.QID{f.QID}, f, nil
	}
	f.log.Debugf("Walk: %s -> %v", f.path.String(), components)

	qids := make([]ninelib.QID, 0, len(components))
	subQid := f.QID
	hasher := fnv.New64a()
	ver := uint64(atomic.LoadUint32(&f.QID.Version))

	comLen := len(components)
	subPath := make(path, len(f.path), len(f.path)+comLen)
	copy(subPath, f.path)

	comLen-- // 1 -> 0 base; allocate needed 1b, we're comparing against 0b

	for i, component := range components {
		subPath = append(subPath, component)
		subString := subPath.String()

		fidInfo, _, err := f.intf.Info(subString, filesystem.StatRequest{Type: true})
		if err != nil {
			return nil, nil, interpretError(err)
		}

		// only the last component may be a non-directory
		if i != comLen && fidInfo.Type != coreiface.TDirectory {
			return qids, nil, errors.New("TODO error message: middle component is not a directory")
		}

		if _, err = hasher.Write([]byte(subString)); err != nil {
			return nil, nil, err // TODO: 9Error not GoError
		}

		/* TODO:
		we need to test to make sure the server is re-using open references in a specific case
		someDir := fid.Walk("a")
		b1 := someDir.Walk("b")
		someDir.Create("c")
		b2 := someDir.Walk("b")

		b1 == b2 should be true; specifically the qid.path

		the qid.path should only change when b1 and b2 are not referenced
		a b3 would have a new path value, by virtue of create modifying someDir's version

		if ninelib doesn't behave this way, we need to keep track of qids ourself

		old paths should not change just because the directory added something
		*/

		subQid = ninelib.QID{
			Type: coreTypeTo9PType(fidInfo.Type).QIDType(),
			Path: hasher.Sum64() + ver,
			// TODO:
			// we need some QID generation abstraction, provided in the constructor
			// The only spec requirement is that `Path` be unique per file
			// (not per `name`. if a file at `name` is deleted and recreated with the same `name`
			// the new file will have a different `Path`)
			// however, we have no long term storage
			// so we can't store a file's path when it's created
			// nor can we retrieve it later
		}

		hasher.Reset()
		qids = append(qids, subQid)
	}

	newFid := f.template()
	newFid.path = subPath
	newFid.QID = subQid

	return qids, newFid, nil
}

func (f *fid) Create(name string, flags ninelib.OpenFlags, permissions ninelib.FileMode, uid ninelib.UID, gid ninelib.GID) (ninelib.File, ninelib.QID, uint32, error) {
	subPath := f.path.Join(name)
	if err := f.intf.Make(subPath); err != nil {
		return nil, ninelib.QID{}, 0, interpretError(err)
	}

	// directory has changed, so too will its version
	ver := uint64(atomic.AddUint32(&f.QID.Version, 1))

	file, err := f.intf.Open(subPath, ioFlagsFrom9P(flags))
	if err != nil {
		return nil, ninelib.QID{}, 0, interpretError(err)
	}

	hasher := fnv.New64a()
	if _, err = hasher.Write([]byte(subPath)); err != nil {
		return nil, ninelib.QID{}, 0, err // TODO: 9Error not GoError
	}

	newFid := f.template()
	newFid.path = append(newFid.path, name)
	newFid.QID = ninelib.QID{
		Type: ninelib.TypeRegular,
		Path: hasher.Sum64() + ver,
	}
	newFid.File = file

	return newFid, newFid.QID, 0, nil // TODO: we need to get IOUnit from the constructor
}

func (f *fid) Open(mode p9.OpenFlags) (p9.QID, uint32, error) {
	f.log.Debugf("Open: %s", f.path.String())

	switch f.QID.Type {
	case ninelib.TypeRegular:
		file, err := f.intf.Open(f.path.String(), ioFlagsFrom9P(mode))
		if err != nil {
			f.log.Error(err)
			return ninelib.QID{}, 0, interpretError(err)
		}
		f.File = file
		// TODO iounit
		return f.QID, 0, nil
	case ninelib.TypeDir:
		dir, err := f.intf.OpenDirectory(f.path.String())
		if err != nil {
			f.log.Error(err)
			return ninelib.QID{}, 0, interpretError(err)
		}
		f.Directory = dir
		// TODO iounit
		return f.QID, 0, nil
	default:
		err := errors.New("unexpected type") // TODO: proper error
		f.log.Error(err)
		return f.QID, 0, err
	}
}

func (f *fid) Close() error {
	if f.File != nil {
		return f.File.Close()
	}

	if f.Directory != nil {
		return f.Directory.Close()
	}

	return nil // no I/O was opened, just the reference
}

func (f *fid) ReadAt(p []byte, offset int64) (int, error) {
	if _, err := f.File.Seek(offset, io.SeekStart); err != nil {
		return 0, interpretError(err)
	}

	readBytes, err := f.File.Read(p)
	if err != nil {
		err = interpretError(err)
	}
	return readBytes, err
}

func (f *fid) Readdir(offset uint64, count uint32) (p9.Dirents, error) {
	f.log.Debugf("Readdir: {%d|%d} %s", offset, count, f.path.String())
	if f.Directory == nil {
		return nil, errors.New("file is not open") // TODO: 9Error
	}

	// TODO: we might want to reset the directory on 0
	// check the specs on what it says about this
	//if offset == 0 {f.dir.reset}

	ver := uint64(atomic.LoadUint32(&f.QID.Version))
	hasher := fnv.New64a()

	// TODO: const timeout and maybe embed a ctx on fid
	callCtx, cancel := context.WithTimeout(context.TODO(), 20*time.Second)
	defer cancel()

	nineEnts := make(ninelib.Dirents, 0)
	for ent := range f.Directory.List(callCtx, offset) {
		entName := ent.Name()
		entPath := f.path.Join(entName)

		fidInfo, _, err := f.intf.Info(entPath, filesystem.StatRequest{Type: true})
		if err != nil {
			return nineEnts, interpretError(err)
		}

		if _, err = hasher.Write([]byte(entPath)); err != nil {
			return nineEnts, err // TODO: 9Error not GoError
		}

		nineEnts = append(nineEnts, ninelib.Dirent{
			Name:   entName,
			Offset: ent.Offset(),
			QID: ninelib.QID{
				Type: coreTypeTo9PType(fidInfo.Type).QIDType(),
				Path: hasher.Sum64() + ver,
			},
		})
		hasher.Reset()

		if uint32(len(nineEnts)) == count {
			break
		}
	}

	return nineEnts, nil
}

func (f *fid) GetAttr(req p9.AttrMask) (p9.QID, p9.AttrMask, p9.Attr, error) {
	f.log.Debugf("GetAttr: %s", f.path.String())

	fidInfo, infoFilled, err := f.intf.Info(f.path.String(), requestFrom9P(req))
	if err != nil {
		return f.QID, ninelib.AttrMask{}, ninelib.Attr{}, interpretError(err)
	}

	attr := attrFromCore(fidInfo) // TODO: maybe resolve IDs
	tg := timeGroup{atime: f.initTime, mtime: f.initTime, ctime: f.initTime, btime: f.initTime}
	applyCommonsToAttr(&attr, f.filesWritable, tg, idGroup{uid: ninelib.NoUID, gid: ninelib.NoGID})

	return f.QID, filledFromCore(infoFilled), attr, nil
}
