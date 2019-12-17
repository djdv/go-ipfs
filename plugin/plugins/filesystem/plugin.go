package filesystem

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/hugelgupf/p9/p9"
	"github.com/ipfs/go-ipfs/core"
	"github.com/ipfs/go-ipfs/core/coreapi"
	plugin "github.com/ipfs/go-ipfs/plugin"
	"github.com/ipfs/go-ipfs/plugin/plugins/filesystem/filesystems/overlay"
	"github.com/ipfs/go-ipfs/plugin/plugins/filesystem/meta"
	logging "github.com/ipfs/go-log"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr-net"
)

const (
	PluginName    = "filesystem"
	PluginVersion = "0.0.1"
)

var (
	_ plugin.PluginDaemonInternal = (*FileSystemPlugin)(nil) // impl check

	// Plugins is an exported list of plugins that will be loaded by go-ipfs.
	Plugins = []plugin.Plugin{
		&FileSystemPlugin{}, //TODO: individually name implementations: &P9{}
	}

	logger logging.EventLogger
)

func init() {
	logger = logging.Logger("plugin/filesystem")
}

type FileSystemPlugin struct {
	ctx    context.Context
	cancel context.CancelFunc

	addr      multiaddr.Multiaddr
	listener  manet.Listener
	closed    chan struct{}
	serverErr error
}

func (*FileSystemPlugin) Name() string {
	return PluginName
}

func (*FileSystemPlugin) Version() string {
	return PluginVersion
}

func (fs *FileSystemPlugin) Init(env *plugin.Environment) error {
	logger.Info("Initializing 9P resource server...")
	if fs.addr != nil {
		return fmt.Errorf("already initialized with %s", fs.addr.String())
	}

	// stabilise repo path; our template depends on this
	if !filepath.IsAbs(env.Repo) {
		absRepo, err := filepath.Abs(env.Repo)
		if err != nil {
			return err
		}
		env.Repo = absRepo
	}

	cfg, err := loadPluginConfig(env)
	if err != nil {
		return err
	}

	addrString := cfg.Service[defaultService]

	// expand config templates
	templateRepoPath := env.Repo
	if strings.HasPrefix(addrString, "/unix") {
		// prevent template from expanding to double slashed paths like `/unix//home/...`
		// but allow it to expand to `/unix/C:\Users...`
		templateRepoPath = strings.TrimPrefix(templateRepoPath, "/")
	}

	// only expand documented value(s)
	addrString = os.Expand(addrString, func(key string) string {
		return (map[string]string{tmplHome: templateRepoPath})[key]
	})

	// initialize listening addr from config string
	ma, err := multiaddr.NewMultiaddr(addrString)
	if err != nil {
		return err
	}
	fs.addr = ma

	logger.Info("9P resource server is okay to start")
	return nil
}

func (fs *FileSystemPlugin) Start(node *core.IpfsNode) error {
	logger.Info("Starting 9P resource server...")
	if fs.addr == nil {
		return fmt.Errorf("Start called before plugin Init")
	}

	// make sure we're not in use already
	if fs.listener != nil {
		return fmt.Errorf("already started and listening on %s", fs.listener.Addr())
	}

	// make sure the api is valid
	coreAPI, err := coreapi.NewCoreAPI(node)
	if err != nil {
		return err
	}

	// if we're using unix sockets, make sure the file doesn't exist
	{
		var failed bool
		multiaddr.ForEach(fs.addr, func(comp multiaddr.Component) bool {
			if comp.Protocol().Code == multiaddr.P_UNIX {
				target := comp.Value()
				if len(target) >= 108 {
					// TODO [anyone] this type of check is platform dependant and checks+errors around it should exist in `mulitaddr` when forming the actual structure
					// e.g. on Windows 1909 and lower, this will always fail when binding
					// on Linux this can cause problems if applications are not aware of the true addr length and assume `sizeof addr <= 108`
					logger.Warning("Unix domain socket path is at or exceeds standard length `sun_path[108]` this is likely to cause problems")
				}
				if callErr := os.Remove(comp.Value()); err != nil && !os.IsNotExist(err) {
					logger.Error(err)
					failed = true
					err = callErr
				}
			}
			return false
		})
		if failed {
			return err
		}
	}

	// launch the listener
	listener, err := manet.Listen(fs.addr)
	if err != nil {
		logger.Errorf("9P listen error: %s\n", err)
		return err
	}
	fs.listener = listener

	// construct and launch the 9P resource server
	fs.ctx, fs.cancel = context.WithCancel(context.Background())
	fs.closed = make(chan struct{})

	// TODO: either: 1) pass the already constructed `mfs.root` through or 2) add a pubfunc getter to `mfs.Root` so we can reconstruct it
	// copy paste function body for now
	opts := []meta.AttachOption{
		meta.Logger(logging.Logger("9root")),
		meta.MFSRoot(node.FilesRoot),
	}

	server := p9.NewServer(overlay.Attacher(fs.ctx, coreAPI, opts...))
	go func() {
		// run the server until the listener closes
		// store error on the fs object then close our syncing channel (see use in `Close` below)

		err := server.Serve(manet.NetListener(fs.listener))

		// [async] we expect `net.Accept` to fail when the filesystem has been canceled
		if fs.ctx.Err() != nil {
			// non-'accept' ops are not expected to fail, so their error is preserved
			var opErr *net.OpError
			if errors.As(fs.serverErr, &opErr) && opErr.Op != "accept" {
				fs.serverErr = err
			}
		} else {
			// unexpected failure during operation
			fs.serverErr = err
		}

		close(fs.closed)
	}()

	logger.Infof("9P service is listening on %s\n", fs.listener.Addr())
	return nil
}

func (fs *FileSystemPlugin) Close() error {
	logger.Info("9P server requested to close")
	if fs.addr == nil { // forbidden
		return fmt.Errorf("Close called before plugin Init")
	}

	// synchronization between plugin interface <-> fs server
	if fs.closed != nil { // implies `Start` was called prior
		fs.cancel()         // stop and prevent all fs operations, signifies "closing" intent
		fs.listener.Close() // stop accepting new clients
		<-fs.closed         // wait for the server thread to set the error value
		fs.listener = nil   // reset `Start` conditions
		fs.closed = nil
	}
	// otherwise we were never started to begin with; default/initial value will be returned
	return fs.serverErr
}
