package sys

import (
	"golang.org/x/sys/unix"
)

func Attach(source, target, args string) error {
	return unix.Mount(source, target, "9p", 0, args)
}
