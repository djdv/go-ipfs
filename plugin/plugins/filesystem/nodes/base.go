package fsnodes

import (
	"context"

	"github.com/djdv/p9/p9"
	"github.com/djdv/p9/unimplfs"
	files "github.com/ipfs/go-ipfs-files"
	logging "github.com/ipfs/go-log"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

const ( //device - attempts to comply with standard multicodec table
	dMemory = 0x2f // generic "path"
	dIPFS   = 0xe4
)

var _ p9.File = (*Base)(nil)

// Base is a foundational file system node that provides common file metadata as well as stubs for unimplemented methods
type Base struct {
	// Provide stubs for unimplemented methods
	unimplfs.NoopFile
	p9.DefaultWalkGetAttr

	// Storage for file's metadata
	Qid      p9.QID
	meta     p9.Attr
	metaMask p9.AttrMask

	// The base context should be valid for the lifetime of the file system
	// and used to derive call specific contexts from
	Ctx    context.Context
	Logger logging.EventLogger

	parent walkRef // parent should be set and used to handle ".." requests
	child  walkRef // child is an optional field, used to hand off child requests to another file system
}

// IPFSBase is much like Base but extends it to hold IPFS specific metadata
type IPFSBase struct {
	Base

	Path corepath.Resolved
	core coreiface.CoreAPI

	// you will typically want to derive a context from the base context within one operation (like Open)
	// use it with the CoreAPI for something
	// and cancel it in another operation (like Close)
	// that pointer should be stored here between calls
	operationsCancel context.CancelFunc

	// operation handle storage
	file      files.File
	directory *directoryStream
}

func (b Base) Close() error {
	if b.child != nil {
		return b.child.Close()
	}

	return nil
}

func (ib *IPFSBase) Close() error {
	var lastErr error

	if err := ib.Base.Close(); err != nil {
		ib.Logger.Errorf("base close: %s", err)
		lastErr = err
	}

	if ib.file != nil {
		if err := ib.file.Close(); err != nil {
			ib.Logger.Errorf("files.File close: %s", err)
			lastErr = err
		}
	}

	if ib.operationsCancel != nil {
		ib.operationsCancel()
	}

	return lastErr
}

type directoryStream struct {
	entryChan <-chan coreiface.DirEntry
	cursor    uint64
	eos       bool // have seen end of stream?
	err       error
}

type walkRef interface {
	p9.File
	QID() p9.QID
	Parent() walkRef
	Child() walkRef
}

func (b Base) Self() walkRef {
	return b
}

func (b Base) Parent() walkRef {
	return b.parent
}

func (b Base) Child() walkRef {
	return b.child
}

func (b Base) QID() p9.QID {
	return b.Qid
}
