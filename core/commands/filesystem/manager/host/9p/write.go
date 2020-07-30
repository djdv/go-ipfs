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

	if !setFields.Size {
		return nil // nothing to modify, nothing to do
	}

	// make sure we exist
	existing, _, err := f.nodeInterface.Info(f.path.String(), filesystem.StatRequest{Size: true, Type: true})
	if err != nil {
		return err
	}

	// make sure the request actually requires a modification
	if existing.Size == new.Size {
		return nil
	}

	// and is legal
	if existing.Type == coreiface.TDirectory && new.Size != 0 {
		return errors.New("cannot change directory size")
	}

	// finally truncate or extend
	file := f.File
	if f.File == nil { // with an existing handle or a new temporary one
		var err error
		file, err = f.nodeInterface.Open(f.path.String(), filesystem.IOWriteOnly)
		if err != nil {
			f.log.Error(err)
			return err
		}
		defer file.Close()
	}
	return file.Truncate(new.Size)
}

func (f *fid) WriteAt(p []byte, offset int64) (int, error) {
	f.log.Debugf("WriteAt {%d} %s", offset, f.path.String())
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

func (f *fid) UnlinkAt(name string, flags uint32) (err error) {
	subPath := f.path.Join(name)
	f.log.Debugf("UnlinkAt %s", subPath)

	var childStat *filesystem.Stat
	childStat, _, err = f.nodeInterface.Info(subPath, filesystem.StatRequest{Type: true})
	if err != nil {
		return interpretError(err) // go.fs -> 9P
	}

	switch childStat.Type {
	case coreiface.TFile:
		err = f.nodeInterface.Remove(subPath)
	case coreiface.TDirectory:
		err = f.nodeInterface.RemoveDirectory(subPath)
	case coreiface.TSymlink:
		err = f.nodeInterface.RemoveLink(subPath)
	default:
		return fmt.Errorf("unexpected type: %v", childStat.Type)
	}

	if err != nil {
		return interpretError(err)
	}
	return
}
