package fscmds

const (
	defaultAPIOption     = "fuse"
	platformMountRoot    = `~/`
	defaultSystemsOption = "pinfs,keyfs,file"
)

var defaultTargets = []string{
	platformMountRoot + "ipfs",
	platformMountRoot + "ipns",
	platformMountRoot + "file",
}
