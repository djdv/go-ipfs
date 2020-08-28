package p9fsp

import (
	"encoding/binary"
	"errors"
	"hash/fnv"
	gopath "path"
	"sync"
	"sync/atomic"
	"time"

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
	//templatefs.NoopFile // TODO remove
	sync.RWMutex

	nodeInterface filesystem.Interface // interface between 9P and the target API

	log logging.EventLogger // general operations log

	path          path      // the path this FID has walked
	filesWritable bool      // switch for metadata fields and operation availability
	initTime      time.Time // artificial file time signatures

	ninelib.QID // 9P metadata about this file

	filesystem.File      // when opened, this FID will assign to one of these slots
	filesystem.Directory // depending on the QID type its path references
}

func (f *fid) template() *fid {
	return &fid{
		nodeInterface: f.nodeInterface,
		log:           f.log,
		filesWritable: f.filesWritable,
		initTime:      f.initTime,
	}
}

func (f *fid) name() string { return f.path[len(f.path)-1] }

// TODO: split up fid types
// root: has attach
// file, dir: has common base with  relevant storage and methods for particulars only
// dispatch would be in the base Walk method
// (file? name) => file{base:parent.template()}; (dir? name) => directory{base:parent.template()}
func (f *fid) Attach() (ninelib.File, error) {
	f.log.Debugf("Attach")

	if len(f.path) != 0 {
		f.log.Errorf("attach called on %q", f.path.String())
		return nil, errors.New("FID is not a root reference")
	}

	newFid := f.template()
	newFid.QID = ninelib.QID{Type: ninelib.TypeDir}

	return newFid, nil
}

func pathGenerator() func(version uint32, component string) (uint64, error) {
	hasher := fnv.New64a()

	// path = version, component | {version, component};
	// format can be whatever as long as it's unique to a specific file (and its version)
	return func(version uint32, component string) (uint64, error) {
		if err := binary.Write(hasher, binary.LittleEndian, version); err != nil {
			return 0, err
		}

		if _, err := hasher.Write([]byte(component)); err != nil {
			return 0, err
		}

		return hasher.Sum64(), nil
	}
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

	comLen := len(components)
	subPath := make(path, len(f.path), len(f.path)+comLen)
	copy(subPath, f.path)
	comLen-- // index base changes 1 -> 0

	ver := atomic.LoadUint32(&f.QID.Version)
	pathGen := pathGenerator()
	subQid := f.QID
	for i, component := range components {
		if component == "" { // TODO: clean components prior to this loop, or check for all here
			continue
		}

		subPath = append(subPath, component)

		fidInfo, _, err := f.nodeInterface.Info(subPath.String(), filesystem.StatRequest{Type: true})
		if err != nil {
			return nil, nil, interpretError(err)
		}

		// only the last component may be a non-directory
		if i != comLen && fidInfo.Type != coreiface.TDirectory {
			return qids, nil, errors.New("TODO error message: middle component is not a directory")
		}

		path, err := pathGen(ver, component)
		if err != nil {
			return nil, nil, err
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
			Path: path,
			// TODO:
			// we need some QID generation abstraction, provided in the constructor
			// The only spec requirement is that `Path` be unique per file
			// (not per `name`. if a file at `name` is deleted and recreated with the same `name`
			// the new file will have a different `Path`)
			// however, we have no long term storage
			// so we can't store a file's path when it's created
			// nor can we retrieve it later
		}

		qids = append(qids, subQid)
	}

	newFid := f.template()
	newFid.path = subPath
	newFid.QID = subQid

	return qids, newFid, nil
}

func (f *fid) Open(mode ninelib.OpenFlags) (ninelib.QID, uint32, error) {
	f.log.Debugf("Open: %s", f.path.String())

	switch f.QID.Type {
	case ninelib.TypeRegular:
		file, err := f.nodeInterface.Open(f.path.String(), ioFlagsFrom9P(mode))
		if err != nil {
			f.log.Error(err)
			return ninelib.QID{}, 0, interpretError(err)
		}
		f.File = file
		// TODO iounit
		return f.QID, 0, nil
	case ninelib.TypeDir:
		dir, err := f.nodeInterface.OpenDirectory(f.path.String())
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
