package interfaceutils

import (
	"github.com/ipfs/go-ipfs/filesystem/errors"
)

// Err implements the filesystem error interface
// it is expected that all of our `filesystem.Interface` methods return these exclusively
// rather than plain Go errors
type Error struct {
	Cause error
	Type  errors.Kind
}

func (e *Error) Error() string     { return e.Cause.Error() }
func (e *Error) Kind() errors.Kind { return e.Type }
