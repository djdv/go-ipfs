package mount

func init()                            { suggestedProvider = linuxProviderCheck }
func linuxProviderCheck() ProviderType { return ProviderFuse }
