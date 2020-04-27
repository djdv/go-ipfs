package mountinter

func init() {
	suggestedProvider = func() ProviderType { return ProviderFuse }
	suggestedNamespaces = func() []Namespace {
		return []Namespace{
			NamespaceIPFS,
			NamespaceIPNS,
			// NamespaceFiles, not implemented yet
		}
	}
	platformMountRoot = func() string { return `\\localhost\` }
	// \\localhost\ipfs, \\localhost\ipns, \\localhost\file
	suggestedTargets = func() []string {
		return []string{
			platformMountRoot() + "ipfs",
			platformMountRoot() + "ipns",
			// platformTargetRoot() + "file", not implemented yet
		}
	}

	allInOnePath = func() string { return platformMountRoot() + "ipfs" }
}
