package mountinter

import (
	"errors"
)

var (
	PlatformMount  = func(source, target, args string) error { return errors.New("Not implemented") }
	PlatformDetach = func(target string) error { return errors.New("Not implemented") }
)
