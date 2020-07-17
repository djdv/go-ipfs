//+build !solaris,!freebsd,!dragonfly,!openbsd,!netbsd,!darwin,!linux,!plan9

package sys

func Attach(string, string, string) error { return errNotImplemented }
