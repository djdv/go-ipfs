package interfaceutils

import (
	"errors"
	"fmt"

	fserrors "github.com/ipfs/go-ipfs/filesystem/errors"
)

// Err implements the filesystem error interface.
// it is expected that all of our `filesystem.Interface` methods return these exclusively
// rather than plain Go errors.
type Error struct {
	Cause error
	Type  fserrors.Kind
}

func (e *Error) Error() string       { return e.Cause.Error() }
func (e *Error) Kind() fserrors.Kind { return e.Type }

var (
	errExist    = errors.New("already exists")
	errNotExist = errors.New("does not exist")
	errNotDir   = errors.New("is not a directory")
)

func ErrExist(name string) error {
	return &Error{
		Cause: fmt.Errorf("%w: %s", errExist, name),
		Type:  fserrors.Exist,
	}
}

func ErrNotExist(name string) error {
	return &Error{
		Cause: fmt.Errorf("%w: %s", errNotExist, name),
		Type:  fserrors.NotExist,
	}
}

func ErrNotDir(name string) error {
	return &Error{
		Cause: fmt.Errorf("%w: %s", errNotDir, name),
		Type:  fserrors.NotDir,
	}
}

func ErrIO(err error) error { return &Error{Cause: err, Type: fserrors.IO} }
