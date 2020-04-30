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
	// TODO: consider renaming AllInOne to Overlay or similar
	// NOTE:
	// NamespaceAll should be implemented by conductors; targeting all available namespaces
	// NamespaceAllInOne should be implemented by providers

	NamespaceNone Namespace = iota
	NamespaceCore
	NamespaceIPFS
	NamespaceIPNS
	NamespaceFiles
	NamespacePinFS
	NamespaceKeyFS
	// mounts all namespaces to their config or default targets
	NamespaceAll
	// mounts all namespaces within a single directory to a single target
	NamespaceAllInOne // Overlay
)

var (
	// replace any number of these in init() for platform specific suggestions
	suggestedProvider   = func() ProviderType { return ProviderFuse }
	suggestedNamespaces = func() []Namespace {
		return []Namespace{
			NamespacePinFS,
			NamespaceIPNS, // ipns -> keyfs
			// NamespaceFiles, // not implemented yet
		}
	}
	platformMountRoot = func() string { return "/" }
	suggestedTargets  = func() []string {
		return []string{
			platformMountRoot() + "ipfs",
			platformMountRoot() + "ipns",
			// platformTargetRoot() + "file", // not implemented yet
		}
	}
	allInOnePath = func() string { return "/mnt/ipfs" }
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
	// core/ipld namespace is omitted for now
	return map[string]Namespace{
		strings.ToLower(NamespaceIPFS.String()):     NamespaceIPFS,
		strings.ToLower(NamespaceIPNS.String()):     NamespaceIPNS,
		strings.ToLower(NamespaceFiles.String()):    NamespaceFiles,
		strings.ToLower(NamespacePinFS.String()):    NamespacePinFS,
		strings.ToLower(NamespaceKeyFS.String()):    NamespaceKeyFS,
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

func SuggestedNamespaces() []Namespace {
	return suggestedNamespaces()
}

func SuggestedTargets() []string {
	return suggestedTargets()
}

func SuggestedAllInOnePath() string {
	return allInOnePath()
}

func MountRoot() string {
	return platformMountRoot()
}
