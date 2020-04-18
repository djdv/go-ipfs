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

//go:generate stringer -type=Namespace -trimprefix=Namespace -linecomment
const (
	// TODO: consider renaming AllInOne to Overlay or simillar
	// NOTE:
	// NamespaceAll should be implemented by conductors; targeting all available namespaces
	// NamespaceAllInOne should be implemented by providers

	NamespaceNone Namespace = iota
	NamespaceIPFS
	NamespaceIPNS
	NamespaceFiles
	// mounts all namespaces to their config or default targets
	NamespaceAll
	// mounts all namespaces within a single directory to a single target
	NamespaceAllInOne // Overlay
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
		strings.ToLower(NamespaceIPFS.String()):     NamespaceIPFS,
		strings.ToLower(NamespaceIPNS.String()):     NamespaceIPNS,
		strings.ToLower(NamespaceFiles.String()):    NamespaceFiles,
		strings.ToLower(NamespaceAll.String()):      NamespaceAll,
		strings.ToLower(NamespaceAllInOne.String()): NamespaceAllInOne,
	}[strings.ToLower(in)]
}

func ParseProvider(in string) ProviderType {
	return map[string]ProviderType{
		strings.ToLower(ProviderPlan9Protocol.String()): ProviderPlan9Protocol,
		strings.ToLower(ProviderFuse.String()):          ProviderFuse,
	}[strings.ToLower(in)]
}

func SuggestedProvider() ProviderType {
	return suggestedProvider()
}

func SuggestedNamespace() Namespace {
	return suggestedNamespace()
}
