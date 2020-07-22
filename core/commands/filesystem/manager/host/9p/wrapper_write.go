package p9fsp

import (
	"errors"
	"fmt"
	"io"
	"sync/atomic"

	"github.com/hugelgupf/p9/p9"
	ninelib "github.com/hugelgupf/p9/p9"
	"github.com/ipfs/go-ipfs/filesystem"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

func (f *fid) SetAttr(setFields p9.SetAttrMask, new p9.SetAttr) error {
	f.log.Debugf("SetAttr %v %v", setFields, new)

	if setFields.Size {
		existing, _, err := f.nodeInterface.Info(f.path.String(), filesystem.StatRequest{Size: true, Type: true})
		if err != nil {
			return err
		}

		if existing.Type == coreiface.TDirectory && new.Size != 0 {
			return errors.New("cannot change directory size")
		}

		// Truncate or extend
		if existing.Size != new.Size {

			file := f.File
			if f.File == nil {
				var err error
				file, err = f.nodeInterface.Open(f.path.String(), filesystem.IOWriteOnly)
				if err != nil {
					f.log.Error(err)
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
	f.log.Debugf("Create %v %q", permissions, f.path.Join(name))
	subPath := f.path.Join(name)
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

func (f *fid) WriteAt(p []byte, offset int64) (int, error) {
	f.log.Debugf("WriteAt {%d} %q", offset, f.path.String())
	if f.File == nil {
		// TODO: system error
		err := fmt.Errorf("%q is not open for writing", f.path.String())
		f.log.Error(err)
		return 0, err
	}

	if _, err := f.File.Seek(offset, io.SeekStart); err != nil {
		f.log.Error(err)
		return 0, err
	}

	written, err := f.File.Write(p)
	if err != nil {
		f.log.Error(err)
	}

	return written, err
}
