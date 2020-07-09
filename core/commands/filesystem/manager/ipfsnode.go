package manager

type (
	ErrorChannel chan error

	// this structure is stored on the node
	// used to manage file node bindings between
	// the node and the host node
	Manager struct {
		Dispatcher
		ErrorChannel
	}
)
