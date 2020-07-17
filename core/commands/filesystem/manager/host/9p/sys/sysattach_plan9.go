package sys

import (
	"golang.org/x/sys/plan9"
)

func Attach(source, target, _ string) error { return plan9.Bind(source, target, 0) }
func Detach(target string) error            { return plan9.Unmount("", target) }
