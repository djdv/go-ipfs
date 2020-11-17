package fscmds

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	cmds "github.com/ipfs/go-ipfs-cmds"
)

// TODO: names and documentation need a pass

// Request contains all the information needed for the file system manager
// to perform or print the requested operation.
type (
	Request string
	//fmt.Stringer // this request's target, printed as a path
	// e.g. a file system path `/host/mnt/x`
	// a system socket `/socket/ip4/127.0.0.1/tcp/564`
	// etc.
	//Arguments() []string // additional arguments (if any)
	// e.g. libfuse args, 9P server multiaddr
	// etc.

	// Binding is a requests that has been bound to a target on the host,
	// its Closer undoes the binding.
	Binding struct {
		Request   // the request that initiated this binding
		io.Closer // decouples the target from the host
	}

	// Response is the RPC message used between local and remote `cmds.Commands` requests.
	// It gets (un)marshaled into/from Go-binary, json, and text.
	Response struct {
		Binding
		Info  string
		Error error
	}
)

func (resp Response) MarshalJSON() ([]byte, error) {
	if resp.Error != nil {
		return json.Marshal(struct {
			Error string
		}{
			Error: resp.Error.Error(),
		})
	}

	if resp.Info != "" {
		return json.Marshal(struct {
			Info string
		}{
			Info: resp.Info,
		})
	}

	//TODO: cleanup V

	if resp.Binding.Request == "" {
		return nil, errors.New("no fields populated")
	}

	return json.Marshal(struct {
		Binding string
	}{
		Binding: string(resp.Binding.Request),
	})
}

func (resp *Response) UnmarshalJSON(b []byte) (err error) {
	if bytes.Equal(b, []byte("{}")) {
		return
	}

	var bigError = struct{ Error string }{}
	if err = json.Unmarshal(b, &bigError); err == nil && bigError.Error != "" {
		resp.Error = errors.New(bigError.Error)
		return
	}

	var bigInfo = struct{ Info string }{}
	if err = json.Unmarshal(b, &bigInfo); err == nil && bigInfo.Info != "" {
		resp.Info = bigInfo.Info
		return
	}

	var request Request
	if err = json.Unmarshal(b, &request); err != nil {
		return
	}

	resp.Request = request
	return
}

func encodeText(_ *cmds.Request, w io.Writer, v interface{}) (err error) {
	val, ok := v.(Response)
	if !ok {
		val = *(v.(*Response))
	}

	switch {
	case val.Error != nil:
		_, err = w.Write([]byte(val.Error.Error()))
	case val.Info != "":
		_, err = w.Write([]byte(val.Info))
	case val.Request != "":
		_, err = w.Write([]byte(val.Request))
	default: // empty response
		return
	}

	if err != nil {
		return
	}
	_, err = w.Write([]byte("\n"))

	return
}

func encodeJSON(_ *cmds.Request, w io.Writer, v interface{}) error {
	return json.NewEncoder(w).Encode(v)
}
