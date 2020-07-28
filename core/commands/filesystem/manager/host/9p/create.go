package p9fsp

import (
	"errors"
	"strings"
	"sync/atomic"

	ninelib "github.com/hugelgupf/p9/p9"
)

// TODO: simplify child quid generation if possible (don't call walk directly, call components of it)
// relative
func (f *fid) subQid(name string) (ninelib.QID, error) {
	qids, file, err := f.Walk(strings.Split(f.path.Join(name), "/"))
	if err != nil {
		return ninelib.QID{}, err
	}
	defer file.Close()

	// TODO [spec]: proper error/check; maybe it's legal to link to "a/b"
	// need to test a reference implementation for behaviour
	// this is a .L extension, not in base styx or .u
	if len(qids) > 1 {
		return ninelib.QID{}, errors.New("walking to link returned multiple component qids")
	}
	return qids[0], nil
}

func (f *fid) Create(name string, flags ninelib.OpenFlags, permissions ninelib.FileMode, uid ninelib.UID, gid ninelib.GID) (ninelib.File, ninelib.QID, uint32, error) {
	subPath := f.path.Join(name)
	f.log.Debugf("Create %v %s", permissions, subPath)

	if err := f.nodeInterface.Make(subPath); err != nil {
		return nil, ninelib.QID{}, 0, interpretError(err)
	}

	// directory has changed, so too will its version
	atomic.AddUint32(&f.QID.Version, 1)

	newFid := f.template()
	newFid.path = append(newFid.path, name)

	qid, ioUnit, err := newFid.Open(flags)

	return newFid, qid, ioUnit, err
}

func (f *fid) Mkdir(name string, permissions ninelib.FileMode, uid ninelib.UID, gid ninelib.GID) (ninelib.QID, error) {
	f.log.Debugf("Mkdir: %s", f.path.Join(name))
	if err := f.nodeInterface.MakeDirectory(f.path.Join(name)); err != nil {
		return ninelib.QID{}, err
	}

	return f.subQid(name)
}

func (f *fid) Symlink(linkName string, linkTarget string, uid ninelib.UID, gid ninelib.GID) (ninelib.QID, error) {
	f.log.Debugf("Symlink: %s <-> %s", f.path.Join(linkName), linkTarget)
	if err := f.nodeInterface.MakeLink(f.path.Join(linkName), linkTarget); err != nil {
		return ninelib.QID{}, err
	}

	return f.subQid(linkName)
}
