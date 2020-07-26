package fscmds

const (
	defaultAPIOption      = "fuse"
	platformMountRoot     = `\\localhost\`
	defaultSystemsOption  = "pinfs,keyfs,file"
	mountDescWhatAndWhere = `
By default, mounts IPFS, IPNS, and the Files API,
under ` + platformMountRoot + ` to \ipfs, \ipns, and \file, respectively
All IPFS objects will be accessible under those directories.
`

	mountDescExample = `
# Import local FS object into IPFS
> mkdir foo
> echo "baz" > foo/bar
> ipfs add -r foo
added Qmc1sQCRR4y4k7MQCHvdYapSe5vu5qnLZfpvMPb7kD7msd foo/bar
added QmaiTPLXFCZADxfpvs6saE44QbM8fiREfTdqJDrVXUJgSb foo
> ipfs ls QmaiTPLXFCZADxfpvs6saE44QbM8fiREfTdqJDrVXUJgSb
Qmc1sQCRR4y4k7MQCHvdYapSe5vu5qnLZfpvMPb7kD7msd 5 bar
> ipfs cat Qmc1sQCRR4y4k7MQCHvdYapSe5vu5qnLZfpvMPb7kD7msd
baz

# Bind IPFS to host
> ipfs daemon &
> ipfs mount
binding file systems to host:
/fuse/file/host/\\localhost\file
/fuse/pinfs/host/\\localhost\ipfs
/fuse/keyfs/host/\\localhost\ipns
> cd \\localhost\ipfs\QmaiTPLXFCZADxfpvs6saE44QbM8fiREfTdqJDrVXUJgSb
> ls
bar
> cat .\bar
baz
> cat \\localhost\ipfs\QmaiTPLXFCZADxfpvs6saE44QbM8fiREfTdqJDrVXUJgSb\bar
baz
> cat \\localhost\ipfs\Qmc1sQCRR4y4k7MQCHvdYapSe5vu5qnLZfpvMPb7kD7msd
baz
`
)

var defaultTargets = []string{
	platformMountRoot + "ipfs",
	platformMountRoot + "ipns",
	platformMountRoot + "file",
}
