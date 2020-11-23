package filesystem

import (
	"encoding/binary"
	"errors"
	"fmt"
	"unsafe"

	"github.com/multiformats/go-multiaddr"
)

//go:generate stringer -type=API,ID -linecomment -output system_string.go
type (
	protocolCode int32 // identification tokens

	API protocolCode // represents a particular host API (e.g. 9P, Fuse, et al.)
	ID  protocolCode // represents a particular file system implementation (e.g. IPFS, IPNS, et al.)
)

// NOTE: values used are for development only
// official multicodec values should be used when the API is decided on
// for now, we use the max value supported, decrementing; as our private range of protocol values
// (internally: go-multiaddr/d18c05e0e1635f8941c93f266beecaedd4245b9f/varint.go:10)

const (
	// The multiaddr paths used, create a binding between
	// - The host API
	// - The node API
	// - Typically to be encapsulated by a socket, path, or sometimes both
	// e.g. `/fuse/ipfs/path/mnt/ipfs`, `/9p/ipfs/ip4/127.0.0.1/tcp/564/path/n/ipfs`

	// host api multiaddr protocols:
	_             API = API(^uint32(0)>>1) - iota
	Fuse              // fuse
	Plan9Protocol     // 9p

	// existing multicodec standards:
	// TODO: placeholder name on exported constant!
	PathProtocol API = 0x2f // path

	// protocol values (node APIs):
	_    ID = iota
	IPFS    // ipfs
	IPNS    // ipns
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

var (
	ErrInvalidNodeAPI = errors.New("unknown node API requested")
	ErrInvalidHostAPI = errors.New("unknown host API requested")
)

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
			Size:  32, // TODO: const; sizeof (API)
			Transcoder: multiaddr.NewTranscoderFromFunctions(
				nodeAPIStringToBytes, nodeAPIBytesToString,
				nil),
		})
		if err != nil {
			return
		}
	}
	return
}

var (
	stringToID = make(map[string]ID)
	IDToString = make(map[ID]string)
)

func registerSystemIDs(ids ...ID) {
	for _, id := range ids {
		stringToID[id.String()] = id
		IDToString[id] = id.String()
	}
}

func nodeAPIStringToBytes(systemName string) ([]byte, error) {
	id, ok := stringToID[systemName]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrInvalidNodeAPI, systemName)
	}

	buffer := make([]byte, unsafe.Sizeof(id))
	binary.PutVarint(buffer, int64(id))

	return buffer, nil
}

func nodeAPIBytesToString(buffer []byte) (value string, err error) {
	id, _ := binary.Varint(buffer)

	var ok bool
	if value, ok = IDToString[ID(id)]; !ok {
		return "", fmt.Errorf("%w: %#x", ErrInvalidNodeAPI, id)
	}

	return value, nil
}

// TODO: [micro-opt] can we do a byte cast with what we have into a multiaddr? []byte{protocol, value} instead of stringParse(protocol.String(), ...)
// TODO: import functional options for passing of `FuseAllowOther`
func NewFuse(nodeSystem ID, hostMountPoint string) (multiaddr.Multiaddr, error) {
	fuseComponent, err := multiaddr.NewComponent(Fuse.String(), nodeSystem.String())
	if err != nil {
		return nil, err
	}

	pathComponent, err := multiaddr.NewComponent(PathProtocol.String(), hostMountPoint)
	if err != nil {
		return nil, err
	}

	return fuseComponent.Encapsulate(pathComponent), nil
}
