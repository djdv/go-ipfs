package ipfscore

func (*coreInterface) Make(_ string) error                        { return errNotImplemented }
func (*coreInterface) MakeDirectory(_ string) error               { return errNotImplemented }
func (*coreInterface) MakeLink(_ string, linkTarget string) error { return errNotImplemented }
