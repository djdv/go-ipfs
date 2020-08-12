// +build solaris freebsd dragonfly openbsd netbsd linux

package fscmds

var defaultTargets = []string{
	platformMountRoot + "ipfs",
	platformMountRoot + "ipns",
	platformMountRoot + "file",
}

const (
	platformMountRoot     = `/`
	defaultHostAPISetting = "fuse"
	defaultNodeAPISetting = "pinfs,keyfs,file"
	mountDescWhatAndWhere = `
By default, mounts IPFS, IPNS, and the Files API,
under ` + platformMountRoot + ` to /ipfs, /ipns, and /file, respectively
All IPFS objects will be accessible under those directories.

You may have to create the directories before using 'ipfs mount':

> sudo mkdir /ipfs /ipns /file
> sudo chown $(whoami) /ipfs /ipns /file
> ipfs daemon &
> ipfs mount
`
	mountDescExample = `
# setup
> mkdir foo
> echo "baz" > foo/bar
> ipfs add -r foo
added QmWLdkp93sNxGRjnFHPaYg8tCQ35NBY3XPn6KiETd3Z4WR foo/bar
added QmSh5e7S6fdcu75LAbXNZAFY2nGyZUJXyLCJDvn2zRkWyC foo
> ipfs ls QmSh5e7S6fdcu75LAbXNZAFY2nGyZUJXyLCJDvn2zRkWyC
QmWLdkp93sNxGRjnFHPaYg8tCQ35NBY3XPn6KiETd3Z4WR 4 bar
> ipfs cat QmWLdkp93sNxGRjnFHPaYg8tCQ35NBY3XPn6KiETd3Z4WR
baz

# mount
> ipfs daemon &
> ipfs mount
binding file systems to host:
/fuse/file/host/file
/fuse/pinfs/host/ipfs
/fuse/keyfs/host/ipns
> cd /ipfs/QmSh5e7S6fdcu75LAbXNZAFY2nGyZUJXyLCJDvn2zRkWyC
> ls
bar
> cat bar
baz
> cat /ipfs/QmSh5e7S6fdcu75LAbXNZAFY2nGyZUJXyLCJDvn2zRkWyC/bar
baz
> cat /ipfs/QmWLdkp93sNxGRjnFHPaYg8tCQ35NBY3XPn6KiETd3Z4WR
baz
`
)
