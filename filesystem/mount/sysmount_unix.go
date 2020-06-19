//+build !windows

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
	return unix.Mount(source, target, "9p", flags, args)
}

func unixUnmount(target string) error {
	return unix.Unmount(target, flags)
}
