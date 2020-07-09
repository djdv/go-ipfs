//+build !windows

package p9fsp

import "golang.org/x/sys/unix"

// you may be asking, why do the `syscall` and `unix` packages implement `Unmount` for every platform
// but only implement `Mount` for Linux?
// I don't know!
func PlatformDetach(target string) error { return unix.Unmount(target, 0) }
