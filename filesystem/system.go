package transform

//TODO: reconsider the name of this interface
// Interface? FileSystem? Index? GoodiesAndTreatsBag?
// TODO: considerations around non-prefixed methods
// it should be implied that the default action of the filing system is to operate on files, and anything else needs specification. Right?
// arity of these may change; creation specifically will likely need more context
type Interface interface {
	// index
	Open(path string, flags IOFlags) (File, error)
	OpenDirectory(path string) (Directory, error)
	Info(path string, req IPFSStatRequest) (*IPFSStat, IPFSStatRequest, error)
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

	// system
	Close() error // TODO: I don't know if it's a good idea to have this; an even though it's Go convention the name is kind of bad for this
	// TODO: consider
	// Subsystem(path string) (Root, error)
	// e.g. keyfs.Subsystem("/Qm.../an/ipns/path") => (ipns.Root, nil)
}
