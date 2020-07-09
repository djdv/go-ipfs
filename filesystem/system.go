package filesystem

const LogGroup = "filesystem"

// TODO: try to use human English to define this
type ID uint // namespace identifier values that should match with a file node implementation

//go:generate stringer -type=ID --linecomment
const (
	_ ID = iota
	IPFS
	IPNS
	Files // file
	PinFS
	KeyFS
)

// Interface contains the methods to interact with a file node.
// TODO: [bb846ad6-69aa-4f5c-991c-626a7ce92b38] name considerations
// right now this is `filesystem.Interface` rather than `filesystem.FileSystem`
// avoids stuttering and makes sense in long form, but may be too generic in short form `Interface`
type Interface interface {
	// TODO: reconsider if this is a good idea
	ID() ID // returns the ID for this system

	// index
	Open(path string, flags IOFlags) (File, error)
	OpenDirectory(path string) (Directory, error)
	Info(path string, req StatRequest) (*Stat, StatRequest, error)
	ExtractLink(path string) (string, error)

	// creation
	Make(path string) error
	MakeDirectory(path string) error
	MakeLink(path, target string) error

	// removal
	Remove(path string) error
	RemoveDirectory(path string) error
	RemoveLink(path string) error

	// modification
	Rename(oldName, newName string) error

	// node
	Close() error // TODO: I don't know if it's a good idea to have this; an even though it's Go convention the name is kind of bad for this
	// TODO: consider
	// Subsystem(path string) (Root, error)
	// e.g. keyfs.Subsystem("/Qm.../an/ipns/path") => (ipns.Root, nil)
}
