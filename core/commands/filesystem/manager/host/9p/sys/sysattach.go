package sys

import "errors"

// used on platforms that do not support `Attach` and/or `Detach`
var errNotImplemented = errors.New("9P attach wrapper not implemented for this platform")
