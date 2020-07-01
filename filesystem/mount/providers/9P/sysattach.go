package p9fsp

import "errors"

// used on platforms that do not support `PlatformAttach` and/or `PlatformDetach`
var errNotImplemented = errors.New("9P attach wrapper not implemented for this platform")
