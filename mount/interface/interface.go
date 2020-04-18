package mountinter

/*
Conductor is responsible for managing multiple Providers
delegating requests to them while also managing grafted instances
*/
type Conductor interface {
	// Graft uses the selected provider to map groups of namespaces to their targets
	Graft(ProviderType, []TargetCollection) error
	// Detach removes a previously grafted target
	Detach(target string) error
	// Where provides the mapping of providers and their targets
	Where() map[ProviderType][]string
}

// Provider interacts with a namespace and the file system
// grafting a file system implementation to a target
type Provider interface {
	// grafts the target to the file system, returning the interface to detach it
	Graft(target string) (Instance, error)
	// returns true if the target has been grafted but not detached
	Grafted(target string) bool
	// returns a list of grafted targets
	Where() []string
}

// Instance is an active provider target that may be detached from the file system
type Instance interface {
	Detach() error
	Where() (string, error)
}
