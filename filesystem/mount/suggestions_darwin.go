package mountinter

func init() {
	suggestedProvider = func() ProviderType { return ProviderFuse }

	platformMountRoot = func() string { return "~/" }
	allInOnePath = func() string { return platformMountRoot() + "ipfs" }
}
