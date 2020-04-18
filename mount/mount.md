# Editors note
The purpose of this document is to get feedback from a handful of people.  
The sources in this branch are a work in progress and shouldn't be read except for curiosity's sake.  
Nothing has been discussed or approved, nothing is finalized or tested either.  
The FUSE code has yet to be ported from the other branch.  
The 9P code has yet to be ported to the intermediate layer and isn't stable.  
Nothing is async safe yet.  
The commit log has yet to be partitioned.  
And the documentation has yet to be written.  
In short, it's all gross and unusable.

# Overview
The mount directory contains various packages to facilitate mounting various file systems to various hosts using various APIs.  
(Note: "/mount" may change to "/filesystem" as we provide multiple filesystem interfaces. In addition to mounting them, they're useful on their own within Go)
```
./mount
├── conductors (implementation(s) of the `mount.Conductor` interface)
│   └── ipfs-core
├── interface (various interface definitions used by the mount system)
├── providers (implementations of the `mount.Provider` interface)
│   ├── 9P (implements file systems via the Plan 9 protocol)
│   │   └── filesystems (common base for filesystems to be built on)
│   │       ├── ipfs (IPFS API mappings to 9P, etc...)
│   │       ├── ipns
│   │       ├── keyfs
│   │       ├── mfs
│   │       ├── overlay
│   │       └── pinfs
│   └── fuse
│       └── filesystems (common base for filesystems to be built on)
│           ├── ipfs (IPFS API mappings to FUSE, etc...)
│           ├── ipns
│           └── mfs
└── utils (some of the things in here will likely move)
    ├── cmds (hosts the parameters and sub-commands for `daemon`, `mount`, `unmount`)
    ├── common
    ├── sys (interactions with the host OS such as mounting, target defaults, etc.)
    └── transform (wrap coreapi constructs, mapping results to FUSE|9P)
```
Primarily, the packages are used to construct a "conductor" and bind it to the IPFS daemon/node instance.  
The conductor will then facilitate management of file system "providers".  
"Providers" provide implementations of file systems, and facilities to graft them to some target.  
Typically this will be mounting file systems to a path in the host system.  
(e.g. mounting the abstract namespace "IPFS" to the local path `/ipfs`  via the FUSE API)

## Important interfaces
(TODO: either link directly to pkg.go.dev or write a go generate tool to modify this markdown from the source comments; for now we're dumping it here)
### Command line
The command line sub-commands `ipfs daemon` and `ipfs mount` and parsers for their parameters live in `mount/utils/cmd`. They pull arguments (in priority order) from the parameters of the sub-command, the config file, or fall back to a platform default. Feeding them into the underlying go interfaces.  

Issuing `ipfs mount` will mount each namespace by default, but may be customized using combinations of parameters. A complex example would be `ipfs mount --provider=Plan9Protocol --namespace="IPFS,IPNS,FilesAPI" --target="/ipfs,/ipns,/file"` which mimic's the current defaults on Linux, more explicitly.
It is possible to specify any combination of namespaces and targets so long as the argument count matches For example, this is a valid way to map IPFS to 2 different mountpoints `ipfs mount --namespace="IPFS,IPFS" -target="/ipfs,/mnt/ipfs"`  

`ipfs unmount` shares the same parameters as `ipfs mount` with the addition of a `-a` to unmount all previously mounted targets

`ipfs daemon` shares the same parameters as `ipfs mount` simply prefixed with `--mount-`.  
e.g. `ipfs daemon --mount --mount-provider="FUSE" --mount-namespace="IPFS,IPNS" --mount-target="/ipfs,/ipns"`

### Conductors
(Note: I don't like this name but couldn't think of anything better)  
The Conductor is responsible for managing multiple "system `Provider`s". Delegating requests to them, while also managing the instances they provide.
```go
type Conductor interface {
	// Graft uses the selected provider to map groups of namespaces to their targets
	Graft(ProviderType, []TargetCollection) error
	// Detach removes a previously grafted target
	Detach(target string) error
	// Where provides the mapping of providers and their targets
	Where() map[ProviderType][]string
}
```
An implementation of this exists in `mount/conductors/ipfs-core` which is constructed by the daemon on startup or upon calling the mount sub-command. It's stored on the node and shared across calls. It utilizes the IPFS core API for it's operations.
```go
node.Mount = mountcon.NewConductor(node.Context(), coreAPI, opts...)
```


### Providers
Providers provide instances of a namespace/file system and a means with which to bind it to some target (like a path in the operating system's own file system).
```go
// Provider interacts with a namespace and the file system
// grafting a file system implementation to a target
type Provider interface {
	// grafts the target to the file system, returning the interface to detach it
	Graft(target string) (Instance, error)
	// returns true if the target has been grafted but not detached
	Grafted(target string) bool
	// returns a list of grafted targets
	Where() []string
}
```
There are currently 2 providers, 1 for the Plan 9 protocol and 1 for FUSE. They live under `mount/providers`.  
The providers themselves implement the various namespaces and APIs of IPFS. Living under `mount/providers/${provider}/filesystems/${API}.  
An example of this would be mapping the node's Pins via the Pin API as a directory containing directories and files. `mount/providers/9P/filesystems/pinfs`  

Our conductor manages multiple providers on demand. Here is an example of instantiating a 9P related request
```go
mount9p.NewProvider(ctx, namespace, listenAddr, coreAPI, ops...)
mountfuse.NewProvider(ctx, namespace, fuseArgs, coreAPI, ops...)
```


### Provider instances
Simply, provider instances are instances generated by the provider that should be tracked by the caller that generated them. In our case this is the conductor which maps a series of targets to instances, allowing callers to detach these instances by name/path.
```go
// Instance is an active provider target that may be detached from the file system
type Instance interface {
	Detach() error
	Where() (string, error)
}
```
```go
instance, err := someProvider.Graft(target)
```

## Implementation details (incomplete)
### cross boundary locking
In order to allow the daemon to perform normal operations without locking the user out of certain features, such as publishing to IPNS keys or using the FilesAPI via the `ipfs` command, or other API instances. We'll want to incoperate a shared resource lock on the daemon for these namespaces to use.
For example, within the `ipfs name publish` command we would like to acquire a lock for the key we are about to publish to, which may or may not also be in use by an `ipfs mount` instance, or other instance of the CoreAPI.
Likewise with `ipfs files` in general.
As a result we'll need some kind of interface such as this
```go
type ResourceLock interface {
	Request(namespace mountinter.Namespace, resourceReference string, ltype LockType, timeout time.Duration) error
	Release(namespace mountinter.Namespace, resourceReference string, ltype LockType)
}
```
usable within the `name publish` cmd as 
```go
err := daemonNode.???.Request(mountinter.NamespaceIPNS, "/${key-hash}", mountinter.LockDataWrite, 0)
```
where the same instance is used by the rest of the services on the daemon, such as `files`, and `mount`.
Any may hold the lock at various points, preventing one another from colliding and creating inconsistency without entirely disabling functionality on the node / holding exclusive access of the entire node.  

NOTE: a quick hack was written to implement this but I don't trust myself to implement it correctly/efficiently.  
This will require research to see how other systems perform ancestry style path locking and which libraries already exist that could help with it.  

### file system implementations themselves
Currently there are 2 separate file system APIs that themselves implement mappings for various IPFS api's.
1 for FUSE and 1 for 9P. They're fairly distinct but I'm going to put effort into trying to generalize and overlap as much as possible via a transform package.  
An example of this is not implementing 2 different forms of `Getattr` 1 for each API, instead we map from IPFS semantics to some intermediate representation.  
`(mount/utils/transform).CoreGetAttr(ctx, corepath, core, request)`, returns some intermediate object that itself implements transforms `object.ToFuse() *fuselib.Stat_t`, `object.To9P() *p9plib.Attr`.  
There will likely be other ways we can find overlap to provide generalized code over specific code. Allowing for uniformity, as well as more code coverage with less tests.  
Intermediate wrappers for file I/O that wrap the Core+MFS apis to make the layers for fuse.Read and p9.Read smaller. e.g. via something like `intermediate.CoreOpenFile(...) io.ReadWriter` is being tested.  

It may be such that this transformation layer grows into a common file system interface in itself, which could be returned by one of the above interfaces.  
e.g. `Provider.Instance() (FileSystem, error)` which wraps an implementation of some file system API (like FUSE), with transforms that allow for `FileSystem.Open()`.
