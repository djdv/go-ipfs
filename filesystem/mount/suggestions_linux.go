package mountinter

func init()                            { suggestedProvider = linuxProviderCheck }
func linuxProviderCheck() ProviderType { return ProviderFuse }
