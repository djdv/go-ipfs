package meta

import "github.com/hugelgupf/p9/p9"

const (
	// TODO [2019.09.12; anyone]
	// Start a discussion around block sizes
	// should we use the de-facto standard of 4KiB or use our own of 256KiB?
	// context: https://github.com/ipfs/go-ipfs/pull/6612/files#r322989041
	UFS1BlockSize = 256 << 10

	// device - attempts to comply with the [standard multicodec table](https://github.com/multiformats/multicodec/blob/master/table.csv)
	DevMemory = 0x2f // generic "path"
	DevIPFS   = 0xe4

	// pedantic POSIX stuff
	S_IROTH p9.FileMode = p9.Read
	S_IWOTH             = p9.Write
	S_IXOTH             = p9.Exec

	S_IRGRP = S_IROTH << 3
	S_IWGRP = S_IWOTH << 3
	S_IXGRP = S_IXOTH << 3

	S_IRUSR = S_IRGRP << 3
	S_IWUSR = S_IWGRP << 3
	S_IXUSR = S_IXGRP << 3

	S_IRWXO = S_IROTH | S_IWOTH | S_IXOTH
	S_IRWXG = S_IRGRP | S_IWGRP | S_IXGRP
	S_IRWXU = S_IRUSR | S_IWUSR | S_IXUSR

	IRWXA = S_IRWXU | S_IRWXG | S_IRWXO            // 0777
	IRXA  = IRWXA &^ (S_IWUSR | S_IWGRP | S_IWOTH) // 0555
)
