package mountinter

import (
	"errors"
	"fmt"
	"strings"
)

type (
	ProviderType int
	Namespace    int
)

var (
	ErrInvalidNamespace = errors.New("invalid namespace")
	ErrInvalidProvider  = errors.New("invalid provider")
)

//go:generate stringer -type=ProviderType -trimprefix=Provider
const (
	_ ProviderType = iota
	ProviderPlan9Protocol
	ProviderFuse
	// WindowsProjectedFileSystem
	// PowerShellFileSystemProvider
	// AndroidFileSystemProvider
)

//go:generate stringer -type=Namespace -trimprefix=Namespace
const (
	// NOTE:
	// NamespaceAll should be implemented by conductors; targeting all available namespaces
	// NamespaceCombined should be implemented by providers

	_ Namespace = iota
	NamespaceIPFS
	NamespaceIPNS
	NamespaceFiles
	NamespacePinFS
	NamespaceKeyFS
	// mounts all namespaces to their config or default targets
	NamespaceAll
	// mounts all namespaces within a single directory to a single target
	NamespaceCombined
)

var (
	// replace any number of these in init() for platform specific suggestions
	suggestedProvider   = func() ProviderType { return ProviderFuse }
	suggestedNamespaces = func() []Namespace {
		return []Namespace{
			NamespacePinFS,
			NamespaceKeyFS,
			NamespaceFiles,
		}
	}
	platformMountRoot = func() string { return "/" }
	suggestedTargets  = func() []string {
		return []string{
			platformMountRoot() + "ipfs",
			platformMountRoot() + "ipns",
			platformMountRoot() + "file",
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

func ParseNamespace(in string) (Namespace, error) {
	ns, ok := map[string]Namespace{
		strings.ToLower(NamespaceIPFS.String()):     NamespaceIPFS,
		strings.ToLower(NamespaceIPNS.String()):     NamespaceIPNS,
		strings.ToLower(NamespaceFiles.String()):    NamespaceFiles,
		strings.ToLower(NamespacePinFS.String()):    NamespacePinFS,
		strings.ToLower(NamespaceKeyFS.String()):    NamespaceKeyFS,
		strings.ToLower(NamespaceAll.String()):      NamespaceAll,
		strings.ToLower(NamespaceCombined.String()): NamespaceCombined,
	}[strings.ToLower(in)]
	if !ok {
		return Namespace(0), fmt.Errorf("%w:%s", ErrInvalidNamespace, in)
	}
	return ns, nil
}

func ParseProvider(in string) (ProviderType, error) {
	pt, ok := map[string]ProviderType{
		strings.ToLower(ProviderPlan9Protocol.String()): ProviderPlan9Protocol,
		strings.ToLower(ProviderFuse.String()):          ProviderFuse,
	}[strings.ToLower(in)]
	if !ok {
		return ProviderType(0), fmt.Errorf("%w:%s", ErrInvalidProvider, in)
	}
	return pt, nil
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

func SuggestedCombinedPath() string {
	return allInOnePath()
}

func MountRoot() string {
	return platformMountRoot()
}
