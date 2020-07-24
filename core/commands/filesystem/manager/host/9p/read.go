package p9fsp

import (
	"context"
	"errors"
	"hash/fnv"
	"io"
	"sync/atomic"
	"time"

	ninelib "github.com/hugelgupf/p9/p9"
	"github.com/ipfs/go-ipfs/filesystem"
)

func (f *fid) ReadAt(p []byte, offset int64) (int, error) {
	if _, err := f.File.Seek(offset, io.SeekStart); err != nil {
		return 0, interpretError(err)
	}

	readBytes, err := f.File.Read(p)
	if err != nil && err != io.EOF {
		err = interpretError(err)
	}
	return readBytes, err
}

func (f *fid) Readdir(offset uint64, count uint32) (ninelib.Dirents, error) {
	f.log.Debugf("Readdir: {%d|%d} %s", offset, count, f.path.String())
	if f.Directory == nil {
		return nil, errors.New("file is not open") // TODO: 9Error
	}

	// TODO: we might want to reset the directory on 0
	// check the specs on what it says about this
	//if offset == 0 {f.dir.reset}

	ver := uint64(atomic.LoadUint32(&f.QID.Version))
	hasher := fnv.New64a()

	// TODO: const timeout and maybe embed a srvCtx on fid
	callCtx, cancel := context.WithTimeout(context.TODO(), 20*time.Second)
	defer cancel()

	nineEnts := make(ninelib.Dirents, 0)
	for ent := range f.Directory.List(callCtx, offset) {
		entName := ent.Name()
		entPath := f.path.Join(entName)

		fidInfo, _, err := f.nodeInterface.Info(entPath, filesystem.StatRequest{Type: true})
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

func (f *fid) Readlink() (string, error) { return f.nodeInterface.ExtractLink(f.path.String()) }
