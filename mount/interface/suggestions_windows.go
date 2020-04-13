package mountinter

// TODO: use a UNC path if the API allows
func init() {
	platformTargetRoot = func() string { return `I:\` } // I:\ipfs, I:\ipns, I:\file...
	suggestedNamespace = func() Namespace { return NamespaceAllInOne }
	suggestedProvider = func() ProviderType { return ProviderFuse }
}
