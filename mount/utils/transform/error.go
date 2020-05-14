package transform

type FuseErrNo = int

type Error interface {
	error
	ToFuse() FuseErrNo
	To9P() error
}

type ErrorActual struct {
	GoErr  error
	ErrNo  FuseErrNo
	P9pErr error
}

func (e *ErrorActual) Error() string     { return e.GoErr.Error() }
func (e *ErrorActual) ToFuse() FuseErrNo { return e.ErrNo }
func (e *ErrorActual) To9P() error       { return e.P9pErr }
