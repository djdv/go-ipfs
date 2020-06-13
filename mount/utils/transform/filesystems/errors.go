package transformcommon

import "github.com/ipfs/go-ipfs/mount/utils/transform"

type Error struct {
	Cause error
	Type  transform.Kind
}

func (e *Error) Error() string        { return e.Cause.Error() }
func (e *Error) Kind() transform.Kind { return e.Type }
