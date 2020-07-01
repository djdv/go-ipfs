//+build !solaris,!freebsd,!dragonfly,!openbsd,!netbsd,!darwin,!linux,!plan9

package p9fsp

func PlatformDetach(string) error { return errNotImplemented }
