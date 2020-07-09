package fscmds

const (
	defaultAPIOption     = "fuse"
	platformMountRoot    = `\\localhost\`
	defaultSystemsOption = "pinfs,keyfs,file"
)

var defaultTargets = []string{
	platformMountRoot + "ipfs",
	platformMountRoot + "ipns",
	platformMountRoot + "file",
}
