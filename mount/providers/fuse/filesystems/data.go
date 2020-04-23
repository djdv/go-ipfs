package fusecommon

import (
	"math"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
)

const (
	OperationSuccess = 0
	ErrorHandle      = math.MaxUint64
	handleMax        = ErrorHandle - 1

	S_IRWXO = fuselib.S_IROTH | fuselib.S_IWOTH | fuselib.S_IXOTH
	S_IRWXG = fuselib.S_IRGRP | fuselib.S_IWGRP | fuselib.S_IXGRP
	S_IRWXU = fuselib.S_IRUSR | fuselib.S_IWUSR | fuselib.S_IXUSR

	IRWXA = S_IRWXU | S_IRWXG | S_IRWXO                                    // 0777
	IRXA  = IRWXA &^ (fuselib.S_IWUSR | fuselib.S_IWGRP | fuselib.S_IWOTH) // 0555}
)

// TODO: this is mostly here as a placeholder/marker until we figure out how best to standardize permissions
// not everything should have the execute bit set but this isn't stored anywhere for us to fetch either
func ApplyPermissions(fsWritable bool, mode *uint32) {
	*mode |= IRXA &^ (fuselib.S_IXOTH) // 0554
	if fsWritable {
		*mode |= (fuselib.S_IWGRP | fuselib.S_IWUSR) // |0220
	}
}
