package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/djdv/p9/p9"
	plugin "github.com/ipfs/go-ipfs/plugin"
	fsnodes "github.com/ipfs/go-ipfs/plugin/plugins/filesystem/nodes"
	logging "github.com/ipfs/go-log"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr-net"
)

var (
	_ plugin.PluginDaemon = (*FileSystemPlugin)(nil) // impl check

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
	errorChan chan error
}

func (*FileSystemPlugin) Name() string {
	return PluginName
}

func (*FileSystemPlugin) Version() string {
	return PluginVersion
}

func (fs *FileSystemPlugin) Init(env *plugin.Environment) error {
	logger.Info("Initializing 9P resource server...")
	if !filepath.IsAbs(env.Repo) {
		absRepo, err := filepath.Abs(env.Repo)
		if err != nil {
			return err
		}
		env.Repo = absRepo
	}

	cfg := &Config{}
	// config not being set is okay and will load defalts
	// config being set with malformed data is not okay and will instruct the daemon to halt-and-catch-fire
	rawConf, ok := (env.Config).(json.RawMessage)
	if ok {
		if err := json.Unmarshal(rawConf, cfg); err != nil {
			return err
		}
	} else {
		if env.Config != nil {
			return fmt.Errorf("plugin config does not appear to be correctly formatted: %#v", env.Config)
		}
		cfg = defaultConfig(env.Repo)
	}

	var err error
	if envAddr := os.ExpandEnv(EnvAddr); envAddr == "" {
		fs.addr, err = multiaddr.NewMultiaddr(cfg.Service[defaultService])
	} else {
		fs.addr, err = multiaddr.NewMultiaddr(envAddr)
	}
	if err != nil {
		return err
	}

	logger.Info("9P resource server okay for launch")
	return nil
}

func (fs *FileSystemPlugin) Start(core coreiface.CoreAPI) error {
	logger.Info("Starting 9P resource server...")

	//TODO [manet]: unix sockets are not removed on process death (on Windows)
	// so for now we just try to remove it before listening on it
	if runtime.GOOS == "windows" {
		removeUnixSockets(fs.addr)
	}

	fs.ctx, fs.cancel = context.WithCancel(context.Background())
	fs.errorChan = make(chan error, 1)

	var err error
	if fs.listener, err = manet.Listen(fs.addr); err != nil {
		logger.Errorf("9P listen error: %s\n", err)
		return err
	}

	// construct and run the 9P resource server
	s := p9.NewServer(fsnodes.RootAttacher(fs.ctx, core))
	go func() {
		fs.errorChan <- s.Serve(manet.NetListener(fs.listener))
	}()

	logger.Infof("9P service is listening on %s\n", fs.listener.Addr())
	return nil
}

func (fs *FileSystemPlugin) Close() error {
	logger.Info("9P server requested to close")
	fs.cancel()
	fs.listener.Close()
	return <-fs.errorChan
}
