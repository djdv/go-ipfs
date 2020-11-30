package filesystem

import (
	"errors"
	"fmt"

	"github.com/multiformats/go-multiaddr"
)

type (
	protocolCode int32 // identification tokens

	API protocolCode // represents a particular host API (e.g. 9P, Fuse, et al.)
	ID  protocolCode // represents a particular file system implementation (e.g. IPFS, IPNS, et al.)
)

const (
	// For now, we use the max value supported, decrementing; acting as our private range of protocol values
	// (internally: go-multiaddr/d18c05e0e1635f8941c93f266beecaedd4245b9f/varint.go:10)
	_ API = API(^uint32(0)>>1) - iota
	// NOTE: these values are for development only and may change as the
	// multicodec table values and API is decided on.
	//
	//go:generate stringer -type=API,ID -linecomment -output standards_string.go
	//
	// Multiaddr protocol names (host-APIs):
	// The multiaddr paths used, create a binding between a host-API and a node-API,
	// typically paired with a socket, path, etc.
	// e.g. `/fuse/ipfs/path/mnt/ipfs`, `/9p/ipfs/ip4/127.0.0.1/tcp/564/path/n/ipfs`
	Fuse          // fuse
	Plan9Protocol // 9p

	// Existing Multicodec standards:
	// NOTE: this protocol may be defined in another package
	// we should use it or create one it if it doesn't exist.
	// (go-multiaddr itself should register `/path`?)
	// For now we're exporting our own constant for it.
	PathProtocol API = 0x2f // path

	_ ID = iota
	// Multiaddr protocol values (node-APIs):
	IPFS // ipfs
	IPNS // ipns
)

func init() {
	var err error
	if err = registerStandardProtocols(); err != nil {
		panic(err)
	}
	if err = registerAPIProtocols(Fuse, Plan9Protocol); err != nil {
		panic(err)
	}
	registerSystemIDs(IPFS, IPNS)
}

// this is just a context string, the caller should check for
// (wrapped) multiaddr error values if this is encountered
const errDecodeNodeAPI = "could not decode node-API varint"

var ErrInvalidNodeAPI = errors.New("unknown node-API value requested")

func registerStandardProtocols() error {
	return multiaddr.AddProtocol(multiaddr.Protocol{
		Name:  PathProtocol.String(),
		Code:  int(PathProtocol),
		VCode: multiaddr.CodeToVarint(int(PathProtocol)),
		Size:  multiaddr.LengthPrefixedVarSize,
		Path:  true,
		Transcoder: multiaddr.NewTranscoderFromFunctions(
			func(s string) ([]byte, error) { return []byte(s), nil },
			func(b []byte) (string, error) { return string(b), nil },
			nil),
	})
}

func registerAPIProtocols(apis ...API) (err error) {
	for _, api := range apis {
		err = multiaddr.AddProtocol(multiaddr.Protocol{
			Name:  api.String(),
			Code:  int(api),
			VCode: multiaddr.CodeToVarint(int(api)),
			Size:  32, // TODO: const; sizeof (API) or args should be a struct {API, Size, ErrorTemplate}...
			Transcoder: multiaddr.NewTranscoderFromFunctions( // TODO: generator T = g(ErrorTemplateValue)
				apiStringToBytes, nodeAPIBytesToString,
				nil),
		})
		if err != nil {
			return
		}
	}
	return
}

var ( // TODO: [immutable] we should literally inline these into a lookup function, not register in pkg scope
	stringToID = make(map[string]ID)
	IDToString = make(map[ID]string)
)

func registerSystemIDs(ids ...ID) {
	for _, id := range ids {
		stringToID[id.String()] = id
		IDToString[id] = id.String()
	}
}

func apiStringToBytes(systemName string) (buf []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New(fmt.Sprintf("%s", r))
		}
	}()
	id, ok := stringToID[systemName]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrInvalidNodeAPI, systemName)
	}

	return multiaddr.CodeToVarint(int(id)), nil
}

func nodeAPIBytesToString(buffer []byte) (value string, err error) {
	var id int
	id, _, err = multiaddr.ReadVarintCode(buffer)
	if err != nil {
		err = fmt.Errorf("%s: %w", errDecodeNodeAPI, err)
		return
	}

	var ok bool
	if value, ok = IDToString[ID(id)]; !ok {
		err = fmt.Errorf("%w: %#x", ErrInvalidNodeAPI, id)
	}

	return
}
