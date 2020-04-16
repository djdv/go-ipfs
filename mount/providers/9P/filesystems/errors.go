package common

import (
	goerrors "errors"
	"syscall"
)

const (
	// TODO: when all of these are defined and tested, this should be upstreamed to "p9/errors"
	// Linux errno values for non-Linux systems; 9p2000.L compliance
	ENOTDIR = syscall.Errno(0x14)
	ENOENT  = syscall.ENOENT //TODO: migrate to platform independent value
)

var (
	FSCtxNotInitalized = goerrors.New("a required file system context was not initalized")
	FileOpen           = goerrors.New("file is open")
	FileNotOpen        = goerrors.New("file is not open")
	FileClosed         = goerrors.New("file was closed")
)
