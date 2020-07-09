//+build !solaris,!freebsd,!dragonfly,!openbsd,!netbsd,!darwin,!linux,!plan9

package p9fsp

func PlatformAttach(string, string, string) error { return errNotImplemented }
