package fscmds

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager"
	fsm "github.com/ipfs/go-ipfs/core/commands/filesystem/manager"
)

// TODO: rework this into some proper response structure
// I misunderstood how the emitter worked lol
// it takes in an interface{} but is still bound to a single type
// (the concrete type of the value set on `cmd.Command.Type`)
// responders always receive a pointer to concreteTypeOf(`.Type`)
type Response struct {
	fsm.Request
	Info  string
	Error string
}

/* TODO: [lint]
type ResponseError struct{ Err error }
func (re *ResponseError) MarshalJSON() ([]byte, error) { return json.Marshal(re.Err.Error()) }
func (re *ResponseError) UnMarshalJSON(v []byte) error {
	if len(v) > 0 {
		if bytes.Compare(v, []byte("null")) == 0 {
			return nil
		}

		re.Err = errors.New(string(v))
	}

	return nil
}
*/

func processResponse(res cmds.Response, re cmds.ResponseEmitter) error {
	//fmt.Fprintln(os.Stdout, "postrun", res.Length(), res.requests().Options["encoding"])
	//defer fmt.Fprintln(os.Stdout, "postrun exit")
	//os.Stdout.Sync()

	for {
		v, err := res.Next()
		if err != nil {
			if err == io.EOF {
				//fmt.Fprintln(os.Stdout, "eof")
				return nil
			}
			fmt.Fprintln(os.Stdout, "other:", err)
			return err
		}

		switch v.(type) {
		case Response:
			fmt.Println("procResp 0")
		case *Response:
			fmt.Println("procResp 1")
			// parse here
			fmt.Fprintf(os.Stdout, "procResp: emission value:%#v\n", v)
		// do something
		case string:
			fmt.Println("procResp 2")
			outType := res.Request().Options[cmds.EncLong]
			if outType == cmds.Text {
			}
		default:
			fmt.Fprintf(os.Stdout, "procResp 3: value:%#v\n", v)
			return cmds.ErrIncorrectType
		}
	}
}

func encodeText(req *cmds.Request, w io.Writer, v interface{}) error {
	val, ok := v.(Response)
	if !ok {
		val = *(v.(*Response))
	}

	var res string
	switch {
	case val.Error != "":
		res = val.Error
	case val.Info != "":
		res = val.Info
	default:
		res = val.String()
	}
	res += "\n"

	_, err := w.Write([]byte(res))
	return err
}

func encodeJson(req *cmds.Request, w io.Writer, v interface{}) error {
	return json.NewEncoder(w).Encode(v)
}

func CloseFileSystem(dispatcher fsm.Dispatcher) <-chan Response {
	responses := make(chan Response, 1)
	responses <- Response{Info: "detaching all file systems from the host..."}

	// FIXME: closes instances but doesn't remove them from the index

	go func() {
		for index := range dispatcher.List() {
			for hostResp := range index.FromHost {
				binding := hostResp.Binding
				resp := Response{
					Request: manager.Request{
						Header:  index.Header,
						Request: binding.Request,
					},
				}

				responses <- Response{Info: fmt.Sprintf(`closing: %s`, resp.String())}
				if err := binding.Close(); err != nil {
					resp.Error = err.Error()
				}
				responses <- resp
			}
		}
		close(responses)
	}()

	return responses
}
