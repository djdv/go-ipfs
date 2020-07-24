package p9fsp

import (
	"errors"
	"fmt"
	"io"

	ninelib "github.com/hugelgupf/p9/p9"
	"github.com/ipfs/go-ipfs/filesystem"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

func (f *fid) SetAttr(setFields ninelib.SetAttrMask, new ninelib.SetAttr) error {
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

func (f *fid) UnlinkAt(name string, flags uint32) error {
	if err := f.nodeInterface.Remove(f.path.Join(name)); err != nil {
		return interpretError(err)
	}
	return nil
}
