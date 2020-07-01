package p9fsp

import (
	"golang.org/x/sys/unix"
)

func PlatformAttach(source, target, args string) error {
	return unix.Mount(source, target, "9p", 0, args)
}
