//+build !solaris,!freebsd,!dragonfly,!openbsd,!netbsd,!darwin,!linux,!plan9

package sys

func Detach(string) error { return errNotImplemented }
