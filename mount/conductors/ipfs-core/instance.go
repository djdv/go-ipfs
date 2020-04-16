package mount

import (
	"context"
	"errors"
	"fmt"
	"sync"

	cond "github.com/ipfs/go-ipfs/mount/conductors"
	mountinter "github.com/ipfs/go-ipfs/mount/interface"
	provider "github.com/ipfs/go-ipfs/mount/providers"
	mount9p "github.com/ipfs/go-ipfs/mount/providers/9P"
	mountfuse "github.com/ipfs/go-ipfs/mount/providers/fuse"
	mountcom "github.com/ipfs/go-ipfs/mount/utils/common"
	logging "github.com/ipfs/go-log"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

var log = logging.Logger("mount/conductor")

type (
	namespaceMap       map[mountinter.Namespace]mountinter.Provider
	namespaceProviders struct {
		p9p, fuse namespaceMap
	}
)

type conductor struct {
	sync.Mutex
	ctx context.Context

	// IPFS API
	core coreiface.CoreAPI

	// object implementation details
	lock      mountcom.ResourceLock
	instances mountcom.InstanceCollection
	providers namespaceProviders
}

func (con *conductor) Graft(provider mountinter.ProviderType, targets []mountinter.TargetCollection, ops ...cond.Option) error {
	con.Lock()
	defer con.Unlock()
	opts := cond.ParseOptions(ops...)
	if opts.Foreground {
		return errors.New("Foreground mounting not implemented yet")
	}

	// NOTE: we're going to have to rewrite the unwind logic for foreground support
	// ^ for-each; go{try to graft, with success signal}; if success; add to unwind stack

	// construct pairs of {provider instances:mount target}
	type instancePair struct {
		mountinter.Provider
		target string
	}
	instancePairs := make([]instancePair, 0, len(targets))

	for _, triple := range targets {
		if con.instances.Exists(triple.Target) {
			return fmt.Errorf("%q already grafted", triple.Target)
		}

		instance, err := con.getNamespaceProvider(provider, triple.Parameter, triple.Namespace, ops...)
		if err != nil {
			return err
		}
		instancePairs = append(instancePairs, instancePair{Provider: instance, target: triple.Target})
	}

	// prepare unwind function; detaches any successful mounts if all did not succeed
	var (
		failedTarget string
		instances    []mountinter.Instance
	)
	unwind := func() {
		if len(instances) == 0 {
			return
		}
		log.Errorf("failed to attach %q, detaching previous targets", failedTarget)
		for _, instance := range instances {
			whence, err := instance.Where()
			if err != nil {
				log.Errorf("failed to detach instance: %s", err)
				continue
			}
			switch instance.Detach() {
			case nil:
				log.Warnf("detached %s", whence)
			default:
				log.Errorf("failed to detach %q: %s", whence, err)
			}
			// NOTE: regardless of error, we still don't want to keep track of zombies
			// it falls into the hands of the operator and the OS at this point
			// it should be rare that we have the ability to mount an instance, but not unmount it
			if err := con.instances.Remove(whence); err != nil {
				panic(err) // this is fatal however
			}
		}
	}

	// attempt to actually mount the targets
	for _, pair := range instancePairs {
		instance, err := pair.Provider.Graft(pair.target)
		if err != nil {
			unwind()
			return err
		}
		if err := con.instances.Add(pair.target, instance); err != nil {
			unwind()
			return err
		}
		instances = append(instances, instance)
	}

	return nil
}

func (con *conductor) Detach(target string) error {
	instance, err := con.instances.Get(target)
	if err != nil {
		return err
	}

	retErr := instance.Detach() // stop tracking regardless of detatch status; host's cleanup responsability now
	if err := con.instances.Remove(target); err != nil {
		log.Error(err)
	}
	return retErr
}

func (con *conductor) Where() map[mountinter.ProviderType][]string {
	m := make(map[mountinter.ProviderType][]string)

	for _, instance := range con.providers.p9p {
		s := m[mountinter.ProviderPlan9Protocol]
		s = append(s, instance.Where()...)
		m[mountinter.ProviderFuse] = s
	}
	for _, instance := range con.providers.fuse {
		s := m[mountinter.ProviderFuse]
		s = append(s, instance.Where()...)
		m[mountinter.ProviderFuse] = s
	}

	return m
}

func (con *conductor) newProvider(prov mountinter.ProviderType, provParam string, namespace mountinter.Namespace, ops ...cond.Option) (mountinter.Provider, error) {
	opts := cond.ParseOptions(ops...)
	provOps := []provider.Option{provider.ProviderFilesRoot(opts.FilesRoot)}

	switch prov {
	case mountinter.ProviderPlan9Protocol:
		return mount9p.NewProvider(con.ctx, namespace, provParam, con.core, provOps...)

	case mountinter.ProviderFuse:
		return mountfuse.NewProvider(con.ctx, namespace, provParam, con.core, provOps...)
	}

	return nil, fmt.Errorf("unknown provider %q", prov)
}

func (con *conductor) getNamespaceProvider(prov mountinter.ProviderType, providerParameter string, namespace mountinter.Namespace, ops ...cond.Option) (mountinter.Provider, error) {
	var namespaces namespaceMap
	switch prov {
	case mountinter.ProviderPlan9Protocol:
		if con.providers.p9p == nil {
			con.providers.p9p = make(namespaceMap)
		}
		namespaces = con.providers.p9p
	case mountinter.ProviderFuse:
		if con.providers.fuse == nil {
			con.providers.fuse = make(namespaceMap)
		}
		namespaces = con.providers.fuse
	}

	instance, ok := namespaces[namespace]
	if !ok {
		newInst, err := con.newProvider(prov, providerParameter, namespace, ops...)
		if err != nil {
			return nil, err
		}
		instance, namespaces[namespace] = newInst, newInst
	}

	return instance, nil
}

type resLocker struct {
	// TODO: replace this with something efficient; maps for now to get things working
	lockMap map[mountinter.Namespace]map[string]resLock
}

type resLock struct {
	read, write uint64
}

func (l *resLocker) Request(namespace, resource string) error {
	// FIXME: not implemented yet
	return nil
}
func (l *resLocker) Release(namespace, resource string) {
	// FIXME: not implemented yet
}

func NewConductor(ctx context.Context, core coreiface.CoreAPI) *conductor {
	return &conductor{
		ctx:  ctx,
		core: core,
		//TODO: lock
		//providers: make(providerMap),
		instances: mountcom.NewInstanceCollection(),
	}
}
