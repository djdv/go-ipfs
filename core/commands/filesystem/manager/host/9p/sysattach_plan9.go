package p9fsp

import (
	"golang.org/x/sys/plan9"
)

func PlatformAttach(source, target, _ string) error { return plan9.Bind(source, target, 0) }
func PlatformDetach(target string) error            { return plan9.Unmount("", target) }
