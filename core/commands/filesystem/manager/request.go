package manager

import (
	"strings"

	p9fsp "github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host/9p"
	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host/fuse"

	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host"
	"github.com/ipfs/go-ipfs/filesystem"
)

type (
	Header struct {
		API           // the API this request is directed at
		filesystem.ID // the ID of the file instance the request is directed at
	}

	Request struct {
		Header
		host.Request // the request itself
	}

	Response struct {
		Header                        // all requests returned on the channel
		FromHost <-chan host.Response // are associated with this pair of API/system IDs
	}
)

// Dispatcher parses requests and couples them with their intended APIs.
type Dispatcher interface {
	Attach(...Request) <-chan Response
	Detach(...Request) <-chan Response
	List() <-chan Response // List provides streams of prior (processed) requests
	//io.Closer              // closes all active `Attacher`s
}

const (
	hostPathNamespace   = "/host"
	socketPathNamespace = "/socket"
)

func (cr Request) String() string {
	var sb strings.Builder

	sb.WriteRune('/')
	sb.WriteString(strings.ToLower(cr.API.String())) // fmt: `/9p`

	sb.WriteRune('/')
	sb.WriteString(strings.ToLower(cr.ID.String())) // fmt: `/ipfs`

	switch cr.API {
	default:
		panic("unexpected file instance API requested")
	case Fuse:
		// fmt: `/host/n/ipfs`,
		sb.WriteString(hostPathNamespace)
		index := RequestIndex(cr)
		if index[0] != '/' {
			// `/fuse/pinfs/host/\\localhost\ipfs` is correct
			sb.WriteRune('/')
		}
		sb.WriteString(index)
	case Plan9Protocol:
		// fmt: `/host/n/ipfs`,
		// fmt: `/socket/unix/.../.ipfs/9p.ipfs.sock`, etc.
		sb.WriteString(get9Type(cr.Request))
	}

	return sb.String()
}

func get9Type(request host.Request) string {
	// each 9p instance has a listener address
	// if the listeners address is not provided as a request argument
	// we consider the request invalid
	if len(request.Arguments) != 1 {
	}

	index, addr, socketOnly, err := p9fsp.ParseRequest(request)
	if err != nil {
		panic(err)
	}

	var sb strings.Builder
	sb.WriteString(socketPathNamespace) // fmt: `socket`
	sb.WriteString(addr)                // fmt: `/maddr`

	// if the request was bound to the host fs via a client connection to the listener
	// append the host path as the socket's value
	if !socketOnly {
		sb.WriteString(hostPathNamespace)
		if index[0] != '/' {
			sb.WriteRune('/')
		}
		//fmt: /host/n/ipfs
		sb.WriteString(index)
	}

	return sb.String()
}

func RequestIndex(request Request) string {
	switch request.API {
	case Plan9Protocol:
		indexName, _, _, err := p9fsp.ParseRequest(request.Request)
		if err != nil {
			panic(err) // the request dispatcher pushed a bad response
		}
		return indexName
	case Fuse:
		_, indexName := fuse.ParseRequest(request.Request)
		return indexName
	default:
		return request.Target
	}
}
