package fusecommon

import "github.com/ipfs/go-ipfs/mount/utils/transform"

// TODO: something more efficient than a builtin map
type handle = uint64
type (
	fileTable map[handle]transform.File
	FileTable interface {
		Add(handle, transform.File) error
		Exists(handle) bool
		Get(handle) (transform.File, error)
		Remove(handle) error
		Length() int
		List() []string
	}
)

/* TODO
type (
	directoryTable map[handle]transform.Directory
	DirectoryTable interface {
		Add(handle, transform.File) error
		Exists(handle) bool
		Get(handle) (transform.File, error)
		Remove(handle) error
		Length() int
		List() []string
	}
)
*/
