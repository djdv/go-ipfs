package p9fsp

import (
	"errors"
	"sync/atomic"

	"github.com/hugelgupf/p9/p9"
	ninelib "github.com/hugelgupf/p9/p9"
	"github.com/ipfs/go-ipfs/filesystem"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

func (md *fid) SetAttr(setFields p9.SetAttrMask, new p9.SetAttr) error {
	md.log.Debugf("SetAttr %v %v", setFields, new)

	if setFields.Size {
		existing, _, err := md.nodeInterface.Info(md.path.String(), filesystem.StatRequest{Size: true, Type: true})
		if err != nil {
			return err
		}

		if existing.Type == coreiface.TDirectory && new.Size != 0 {
			return errors.New("cannot change directory size")
		}

		// Truncate or extend
		if existing.Size != new.Size {

			file := md.File
			if md.File == nil {
				var err error
				file, err = md.nodeInterface.Open(md.path.String(), filesystem.IOWriteOnly)
				if err != nil {
					md.log.Error(err)
					return err
				}
				defer file.Close()
			}

			if err := file.Truncate(new.Size); err != nil {
				return err
			}
		}
	}

	return nil
}

func (f *fid) Create(name string, flags ninelib.OpenFlags, permissions ninelib.FileMode, uid ninelib.UID, gid ninelib.GID) (ninelib.File, ninelib.QID, uint32, error) {
	subPath := f.path.Join(name)
	if err := f.nodeInterface.Make(subPath); err != nil {
		return nil, ninelib.QID{}, 0, interpretError(err)
	}

	// directory has changed, so too will its version
	atomic.AddUint32(&f.QID.Version, 1)

	newFid := f.template()
	qid, ioUnit, err := newFid.Open(flags)

	return newFid, qid, ioUnit, err
}
