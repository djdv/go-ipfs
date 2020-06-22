//+build !windows,!openbsd,!netbsd,!freebsd,!darwin

package mountinter

import (
	"golang.org/x/sys/unix"
)

func init() {
	PlatformMount = unixMount
	PlatformDetach = unixUnmount
}

const flags = 0

func unixMount(source, target, args string) error {
	// FIXME: the unix package isn't super useful on the BSDs
	// for whatever reason they have Unmount but not Mount
	// despite the constants existing in the same package
	// we need to branch these out on a per system level and use syscall directly
	return unix.Mount(source, target, "9p", flags, args)
}

func unixUnmount(target string) error {
	return unix.Unmount(target, flags)
}
