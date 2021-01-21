package manager

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/ipfs/go-ipfs/filesystem/manager/errors"
	"github.com/multiformats/go-multiaddr"
)

type (
	// Request is a Multiaddr formatted message, containing file system relevant values
	// (such as a file system API, target, etc.)
	Request []byte // TODO: if we can use `Multiaddr` directly, we should. But I think we need a concrete implementation for RPC encoding.
	// Requests is simply a series of requests.
	Requests = <-chan Request

	// Response contains the request that initiated it,
	// along with an error (if encountered).
	Response struct {
		Request   `json:"request"`
		Error     error      `json:"error,omitempty"`
		io.Closer `json:"-"` // local only field, do not try to send/receive from an encoder
	}
	// Responses is simply a series of responses.
	Responses = <-chan Response
)

func (r Request) String() string {
	if len(r) == 0 {
		return "FIXME-VALUE: multiaddr was empty" // TODO: change when done testing
	}
	return multiaddr.Cast(r).String()
}

type (
	// Interface accepts bind `Request`s,
	// and typically stores relevant `Response`s within its `Index`.
	Interface interface {
		Binder
		Index
	}

	// Binder takes in a series of requests,
	// and returns a series of responses.
	// Responses should contain the request that initiated it,
	// along with either its closer, or an error.
	Binder interface {
		Bind(context.Context, Requests) Responses
	}

	// Index maintains a `List` of Responses.
	// Typically corresponding to a range of responses from `Bind`.
	Index interface {
		List(context.Context) Responses
	}
)

// ParseRequests inspects the input strings and transforms them into a series of typed `Request`s if possible.
// Closing the output streams on cancel or an encountered error.
func ParseRequests(ctx context.Context, arguments ...string) (Requests, errors.Stream) {
	requests, errors := make(chan Request), make(chan error)
	go func() {
		defer close(requests)
		defer close(errors)
		for _, maddrString := range arguments {
			ma, err := multiaddr.NewMultiaddr(maddrString)
			if err != nil {
				err = fmt.Errorf("failed to parse system arguments: %w", err)
				select {
				case errors <- err:
				case <-ctx.Done():
				}
				return
			}
			select {
			case requests <- Request(ma.Bytes()):
			case <-ctx.Done():
				return
			}
		}
	}()
	return requests, errors
}

func (resp Response) MarshalJSON() ([]byte, error) {
	var errStr string
	if resp.Error != nil {
		errStr = resp.Error.Error()
	}

	return json.Marshal(struct {
		Request `json:"request"`
		Error   string `json:"error,omitempty"`
	}{
		Request: resp.Request,
		Error:   errStr,
	})
}

func (resp *Response) UnmarshalJSON(b []byte) (err error) {
	if len(b) < 2 || bytes.Equal(b, []byte("{}")) {
		return
	}

	var mock struct {
		Request `json:"request"`
		Error   string `json:"error,omitempty"`
	}

	if err = json.Unmarshal(b, &mock); err != nil {
		return
	}
	resp.Request = mock.Request

	switch errStr := mock.Error; {
	case errStr == "":
	case strings.Contains(errStr, errUnwound.Error()):
		resp.Error = errUnwound
	default:
		resp.Error = fmt.Errorf(errStr)
	}

	return
}

// TODO: move this or export it; duplicated across pkgs currently
// we may need a string type that maps to concrete errors via Unwrap or something
var errUnwound = fmt.Errorf("binding undone")
