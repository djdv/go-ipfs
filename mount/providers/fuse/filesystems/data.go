package fusecommon

import (
	"runtime"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
)

const (
	OperationSuccess = 0
	ErrorHandle      = ^uint64(0)

	S_IRWXO = fuselib.S_IROTH | fuselib.S_IWOTH | fuselib.S_IXOTH
	S_IRWXG = fuselib.S_IRGRP | fuselib.S_IWGRP | fuselib.S_IXGRP
	S_IRWXU = fuselib.S_IRUSR | fuselib.S_IWUSR | fuselib.S_IXUSR

	IRWXA = S_IRWXU | S_IRWXG | S_IRWXO                                    // 0777
	IRXA  = IRWXA &^ (fuselib.S_IWUSR | fuselib.S_IWGRP | fuselib.S_IWOTH) // 0555}
)

// TODO: this is mostly here as a placeholder/marker until we figure out how best to standardize permissions
// not everything should have the execute bit set but this isn't stored anywhere for us to fetch either
func ApplyPermissions(fsWritable bool, mode *uint32) {

	// TODO: [investigate]
	// apparently cgofuse expects the other_execute bit set for `cd` to work on Windows
	// if this turns out to be the case; split these into different constants via build constraints; don't do this at runtime
	// this is just a quick hack
	var writablePermissions, defaultPermissions uint32
	if runtime.GOOS == "windows" {
		writablePermissions = IRWXA
		defaultPermissions = IRXA
	} else {
		writablePermissions = IRWXA &^ (fuselib.S_IWOTH | fuselib.S_IXOTH) // 0774
		defaultPermissions = IRXA &^ (fuselib.S_IXOTH)                     // 0554
	}

	// TODO: proper write mask when creation is implemented
	// for now we never want write set on directories
	if *mode&fuselib.S_IFMT != fuselib.S_IFDIR {
		if fsWritable {
			*mode |= writablePermissions
			return
		}
	}
	*mode |= defaultPermissions
}
