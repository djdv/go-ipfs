package manager

import (
	"strings"

	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host"
	"github.com/ipfs/go-ipfs/filesystem"
)

type (
	Header struct {
		API           // the API this request is directed at
		filesystem.ID // the ID of the file instance the request is directed at
	}

	Request struct {
		Header                   // routing information
		HostRequest host.Request // the request itself
	}

	Response struct {
		Header                        // all requests returned on the channel below
		FromHost <-chan host.Response // are associated with the pair of API/system IDs above
	}
)

// Dispatcher parses requests and couples them with their intended APIs.
type Dispatcher interface {
	Attach(...Request) <-chan Response
	Detach(...Request) <-chan Response
	List() <-chan Response // List provides streams of prior (processed) requests
	//io.Closer              // closes all active `Attacher`s
}

func (mreq *Request) String() string {
	var sb strings.Builder

	// set up the API and FS prefixes
	sb.WriteRune('/') // fmt: `/9p`, `/fuse`, et al.
	sb.WriteString(strings.ToLower(mreq.API.String()))

	sb.WriteRune('/') // fmt: `/ipfs`, `/pinfs`, et al.
	sb.WriteString(strings.ToLower(mreq.ID.String()))

	valueName := mreq.HostRequest.String()

	// Like multiaddrs, the component value needs to be delimited by a slash,
	// but need not contain one itself.
	// e.g. `/fuse/pinfs/host\\localhost\ipfs` is considered invalid,
	// while `/fuse/pinfs/host/\\localhost\ipfs` is considered valid.
	if valueName[0] != '/' {
		sb.WriteRune('/')
	}
	sb.WriteString(valueName) // fmt: `/host/n/my-ipfs-node`, `/socket/ip4/...`, etc.

	return sb.String()
}
