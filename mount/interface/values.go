package mountinter

import "strings"

type (
	ProviderType int
	Namespace    int
)

//go:generate stringer -type=ProviderType -trimprefix=Provider
const (
	ProviderNone ProviderType = iota
	ProviderPlan9Protocol
	ProviderFuse
	// WindowsProjectedFileSystem
	// PowerShellFileSystemProvider
	// AndroidFileSystemProvider
)

//go:generate stringer -type=Namespace -trimprefix=Namespace
const (
	NamespaceNone Namespace = iota
	NamespaceIPFS
	NamespaceIPNS
	NamespaceFiles
	// NOTE: All must be implemented by the conductor
	// AllInOne must be implemented by providers
	NamespaceAll      // mounts all namespaces to their config or default targets
	NamespaceAllInOne // mounts all namespaces within a single directory to a single target
)

var (
	suggestedNamespace = func() Namespace { return NamespaceAll }
	suggestedProvider  = func() ProviderType { return ProviderPlan9Protocol }
	platformTargetRoot = func() string { return "/" }
)

// TODO: I still don't like this name;
// History: NamePair -> NameTriplet -> TargetCollection
type TargetCollection struct {
	Namespace
	Target, Parameter string
}

type TargetCollections []TargetCollection

func (pairs TargetCollections) String() string {
	var prettyPaths strings.Builder
	tEnd := len(pairs) - 1
	for i, pair := range pairs {
		prettyPaths.WriteRune('"')
		prettyPaths.WriteString(pair.Target)
		prettyPaths.WriteRune('"')
		if i != tEnd {
			prettyPaths.WriteString(", ")
		}
	}
	return prettyPaths.String()
}

func ParseNamespace(in string) Namespace {
	return map[string]Namespace{
		NamespaceIPFS.String():     NamespaceIPFS,
		NamespaceIPNS.String():     NamespaceIPNS,
		NamespaceFiles.String():    NamespaceFiles,
		NamespaceAll.String():      NamespaceAll,
		NamespaceAllInOne.String(): NamespaceAllInOne,
	}[in]
}

func ParseProvider(in string) ProviderType {
	return map[string]ProviderType{
		ProviderNone.String():          ProviderNone,
		ProviderPlan9Protocol.String(): ProviderPlan9Protocol,
		ProviderFuse.String():          ProviderFuse,
	}[in]
}

func SuggestedProvider() ProviderType {
	return suggestedProvider()
}

func SuggestedNamespace() Namespace {
	return suggestedNamespace()
}
