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
	// Request is a Multiaddr that has been formatted for use in this file system API.
	// It is intended to be used directly as a `multiaddr.Multiaddr`, and is defined concretely as `[]byte` for type encoding reasons only.
	// TODO: if we can use `Multiaddr` directly, we should. But I think it might complicate cmds-rpc and pb-rpc.
	//
	// Specific Multiaddr value's are expected to be agreed upon by the `multiaddr` package itself.
	// But an example standard one could imagine is seeing
	// `/fuse/ipfs/path/ipfs` effectively translating to  some abstract meaning `manager.fuse.mount(ipfs, "/ipfs")`.
	Request []byte
	// Requests is simply a series of requests
	Requests = <-chan Request

	// Response contains the request that initiated it,
	// along with an error (if encountered).
	Response struct {
		Request   `json:"request"`
		Error     error      `json:"error,omitempty"`
		io.Closer `json:"-"` // local only field, do not try to send/receive from an encoder
	}
	// Responses is simply a series of responses
	Responses = <-chan Response
)

func (r Request) String() string {
	if len(r) == 0 {
		return "FIXME-VALUE: multiaddr was empty" // TODO: change when done testing
	}
	return multiaddr.Cast(r).String()
}

type (
	// Interface is the top level file system management interface,
	// containing methods which fulfil and respond to client requests.
	Interface interface {
		Binder
		Index
	}

	//
	// Binder takes in a series of API requests,
	// and returns a series of responses for this handler.
	// e.g. consider `Bind` requests:
	//
	// 	fuseIPFSBinder.Bind("/path/mnt/target") - binds IPFS (via FUSE), to mount path `/mnt/target`
	// 	plan9MFSBinder.Bind("/ip4/127.0.0.1/tcp/564") - binds MFS, to a 9P listener socket `tcp://127.0.0.1:564`
	Binder interface {
		Bind(context.Context, <-chan Request) <-chan Response
	}

	// Index maintains a `List` of instances.
	Index interface {
		List(context.Context) <-chan Response
		// Close should close all instances in the index.
		//io.Closer
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
// even better to obviate it and abstract marshalling on the error type so they can cast themselves
var errUnwound = fmt.Errorf("binding undone")
