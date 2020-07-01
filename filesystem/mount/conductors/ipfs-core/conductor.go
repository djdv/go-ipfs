package ipfsconductor

import (
	"context"
	"fmt"
	"sync"

	"github.com/ipfs/go-ipfs/filesystem/mount"
	provcom "github.com/ipfs/go-ipfs/filesystem/mount/providers"
	p9fsp "github.com/ipfs/go-ipfs/filesystem/mount/providers/9P"
	"github.com/ipfs/go-ipfs/filesystem/mount/providers/fuse"
	logging "github.com/ipfs/go-log"
	gomfs "github.com/ipfs/go-mfs"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

type (
	namespaceMap       map[mount.Namespace]mount.Provider
	namespaceProviders struct {
		p9p, fuse namespaceMap
	}
)

type conductor struct {
	sync.Mutex
	log logging.EventLogger

	ctx       context.Context
	providers namespaceProviders

	// constructor relevant
	core         coreiface.CoreAPI
	filesAPIRoot *gomfs.Root
	resLock      provcom.ResourceLock
}

func NewConductor(ctx context.Context, core coreiface.CoreAPI, opts ...Option) mount.Interface {
	// TODO: reconsider default; we probably want to bind in the foreground by default, not in the background
	settings := parseConductorOptions(opts...)

	if settings.foreground {
		panic("foreground mounting not implemented yet")
	}

	return &conductor{
		ctx:          ctx,
		core:         core,
		resLock:      provcom.NewResourceLocker(),
		log:          settings.log,
		filesAPIRoot: settings.filesAPIRoot,
	}
}

func (con *conductor) getNamespaces(provider mount.ProviderType) (namespaceMap, error) {
	if provider != mount.ProviderPlan9Protocol && provider != mount.ProviderFuse {
		return nil, fmt.Errorf("unknown provider %q", provider)
	}

	var namespaces namespaceMap
	switch provider {
	case mount.ProviderPlan9Protocol:
		if con.providers.p9p == nil {
			con.providers.p9p = make(namespaceMap)
		}
		namespaces = con.providers.p9p
	case mount.ProviderFuse:
		if con.providers.fuse == nil {
			con.providers.fuse = make(namespaceMap)
		}
		namespaces = con.providers.fuse
	}

	return namespaces, nil
}

func (con *conductor) getProvider(providerType mount.ProviderType, namespace mount.Namespace) (mount.Provider, error) {
	namespaces, err := con.getNamespaces(providerType)
	if err != nil {
		return nil, err
	}

	instance, ok := namespaces[namespace]
	if !ok {
		newInst, err := con.newProvider(providerType, namespace)
		if err != nil {
			return nil, err
		}
		instance, namespaces[namespace] = newInst, newInst
	}

	return instance, nil
}

func (con *conductor) newProvider(provider mount.ProviderType, namespace mount.Namespace) (mount.Provider, error) {
	pOps := []provcom.Option{
		provcom.WithResourceLock(con.resLock),
	}

	if namespace == mount.NamespaceFiles {
		pOps = append(pOps, provcom.WithFilesAPIRoot(con.filesAPIRoot))
	}

	if provider == mount.ProviderFuse {
		return fuse.NewProvider(con.ctx, namespace, con.core, pOps...)
	}
	return p9fsp.NewProvider(con.ctx, namespace, con.core, pOps...)
}

func (con *conductor) Bind(provider mount.ProviderType, requests ...mount.Request) error {
	if len(requests) == 0 {
		return nil
	}

	con.Lock()
	defer con.Unlock()

	// FIXME: quick and lazy port
	// separate requests based on their namespace and pass them all at once to prov.Bind()
	// for now we relay individually and don't unwind properly

	var err error
	for _, req := range requests {
		prov, pErr := con.getProvider(provider, req.Namespace)
		if pErr != nil {
			err = pErr
			break
		}

		if err = prov.Bind(req); err != nil {
			break
		}
	}

	return err
}

func (con *conductor) Detach(provider mount.ProviderType, requests ...mount.Request) error {
	// FIXME: quick and lazy port
	// separate requests based on their namespace and pass them all at once to prov.Detach()
	// for now we relay individually

	var err error
	for _, req := range requests {
		prov, pErr := con.getProvider(provider, req.Namespace)
		if pErr != nil {
			err = pErr
			break
		}

		if err = prov.Detach(req); err != nil {
			break
		}
	}

	return err

}

func (con *conductor) List() map[mount.ProviderType][]mount.Request {
	m := make(map[mount.ProviderType][]mount.Request)

	for _, instance := range con.providers.p9p {
		s := m[mount.ProviderPlan9Protocol]
		s = append(s, instance.List()...)
		if len(s) != 0 {
			m[mount.ProviderPlan9Protocol] = s
		}
	}
	for _, instance := range con.providers.fuse {
		s := m[mount.ProviderFuse]
		s = append(s, instance.List()...)
		if len(s) != 0 {
			m[mount.ProviderFuse] = s
		}
	}

	return m
}

func errWrap(target string, base, appended error) error {
	if base == nil {
		base = fmt.Errorf("%q:%s", target, appended)
	} else {
		base = fmt.Errorf("%w; %q:%s", base, target, appended)
	}
	return base
}

func (con *conductor) Close() error {
	var err error
	for ns, instance := range con.providers.p9p {
		if iErr := instance.Close(); iErr != nil {
			err = errWrap(ns.String(), err, iErr)
		}
	}
	for ns, instance := range con.providers.fuse {
		if iErr := instance.Close(); iErr != nil {
			err = errWrap(ns.String(), err, iErr)
		}
	}
	return err
}
