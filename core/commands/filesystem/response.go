package fscmds

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager"
	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host"
	p9fsp "github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host/9p"
	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host/fuse"
)

// Response is the RPC message used between local and remote `cmds.Commands`.
// It gets (un)marshaled into/from Go, json, text("encode" only)
type Response struct {
	manager.Request
	Info  string
	Error error
}

// a successful result is encoded and sent without additional info from the Response
type responseEnc struct {
	Target    string
	Arguments []string
}

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

	return json.Marshal(responseEnc{
		Target:    resp.String(),
		Arguments: resp.HostRequest.Arguments(),
	})
}

/* TODO: [lint]
func (resp Response) MarshalText() ([]byte, error) {
	var b bytes.Buffer
	if resp.Error != nil {
		b.WriteString("Error: ")
		b.WriteString(resp.Error.Error())
		return b.Bytes(), nil
	}

	if resp.Info != "" {
		b.WriteString(resp.Info)
		return b.Bytes(), nil
	}

	b.WriteString(resp.String())
	return b.Bytes(), nil
}
*/

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

	var hostReq responseEnc
	if err = json.Unmarshal(b, &hostReq); err != nil {
		return
	}

	resp.Request, err = decodeTarget(hostReq.Target, hostReq.Arguments)
	return
}

func decodeTarget(targetLine string, arguments []string) (manager.Request, error) {
	var mReq manager.Request

	const malformedFmt = "malformed response: `%s`"

	// input should look like:
	// `/api/fsid/socket/ip4/.../host/n/ipfs`
	if len(targetLine) == 0 || targetLine[0] != '/' {
		return mReq, fmt.Errorf(malformedFmt, targetLine) // TODO: real error
	}
	// skip initial slash
	targetLine = targetLine[1:] // `api/fsid/socket/ip4/.../host/n/ipfs`

	slashBound := strings.IndexRune(targetLine, '/') // find the next boundary
	if slashBound == 0 {
		return mReq, fmt.Errorf(malformedFmt, targetLine) // TODO: real error
	}

	api, err := typeCastAPIArg(targetLine[:slashBound]) // evaluate
	if err != nil {
		return mReq, err
	}
	// move ahead
	targetLine = targetLine[slashBound+1:] // `fsid/socket/ip4/.../host/n/ipfs`

	slashBound = strings.IndexRune(targetLine, '/') // find the next boundary
	if slashBound == 0 {
		return mReq, fmt.Errorf(malformedFmt, targetLine) // TODO: real error
	}

	sysID, err := typeCastSystemArg(targetLine[:slashBound]) // evaluate
	if err != nil {
		return mReq, err
	}
	// move ahead
	targetLine = targetLine[slashBound+1:] // `socket/ip4/.../host/n/ipfs`

	// assign header
	mReq.Header = manager.Header{API: api, ID: sysID}

	// copy the host value from (beyond) the host namespace (if it exists)
	var hostValue string
	if slashBound = strings.Index(targetLine, host.PathNamespace); slashBound != -1 {
		hostValue = targetLine[slashBound+len(host.PathNamespace):]

		// remove the host porting from the line if it contains a prefix
		if slashBound != 0 {
			targetLine = targetLine[:slashBound-1] // `socket/ip4/...`
		}
	}

	switch api {
	default:
		return mReq, fmt.Errorf("unexpected host API: %v", api)
	case manager.Fuse:
		mReq.HostRequest = fuse.Request{HostPath: hostValue, FuseArgs: arguments}
	case manager.Plan9Protocol:
		// copy socket component from the socket namespace (if it exists)
		var socketValue string
		if slashBound = strings.Index(targetLine, host.SocketNamespace); slashBound != -1 {
			socketValue = targetLine[slashBound+len(host.SocketNamespace):]
		}

		mReq.HostRequest = p9fsp.Request{
			ListenAddr: socketValue,
			HostPath:   hostValue,
		}
	}

	return mReq, nil
}

func encodeText(req *cmds.Request, w io.Writer, v interface{}) error {
	val, ok := v.(Response)
	if !ok {
		val = *(v.(*Response))
	}

	var res string
	switch {
	case val.Error != nil:
		res = val.Error.Error()
	case val.Info != "":
		res = val.Info
	case val.HostRequest != nil:
		res = val.String()
	default: // empty response
		return nil
	}
	res += "\n"

	_, err := w.Write([]byte(res))
	return err
}

func encodeJson(req *cmds.Request, w io.Writer, v interface{}) error {
	return json.NewEncoder(w).Encode(v)
}

// TODO: put on dispatcher unexported
// export Close which calls this, merged
func CloseFileSystem(dispatcher manager.Dispatcher) <-chan Response {
	responses := make(chan Response, 1)
	responses <- Response{Info: "detaching all host bindings..."}

	go func() {
		for index := range dispatcher.List() {
			for hostResp := range index.FromHost {
				binding := hostResp.Binding
				resp := Response{
					Request: manager.Request{
						Header:      index.Header,
						HostRequest: binding.Request,
					},
				}

				responses <- Response{Info: fmt.Sprintf(`closing: %s`, resp.String())}
				if err := binding.Close(); err != nil {
					resp.Error = err
				}
				responses <- resp
			}
		}
		close(responses)
	}()

	return responses
}
