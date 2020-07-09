package manager

type API uint

//go:generate stringer -type=API --linecomment
const (
	_             API = iota
	Plan9Protocol     // 9P
	Fuse
	// WindowsProjectedFileSystem
	// PowerShellFileSystemProvider
	// AndroidFileSystemProvider
)
