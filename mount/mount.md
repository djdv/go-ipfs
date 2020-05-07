# Editors note
The purpose of this document is to get feedback from a handful of people.  
The sources in this branch are a work in progress and shouldn't be read at all yet.  
(Nothing is final, nothing is documented, and nothing is tested)  
The commit log is just terrible, it might be split up later when things are closer to final, broken up into sections like "Add conductor", "Add provider FUSE", etc.  

In short, it's all gross and not fully usable yet. But a lot of it works when built.  

* [Overview](#overview)
* [Important interfaces](#important-interfaces)
	* [Command line](#command-line)
	* [Conductors](#conductors)
	* [Providers](#providers)
	* [Instances](#provider-instances)
* [Implementation details](#implementation-details-incomplete)
	* [Cross boundary locking](#cross-boundary-locking)
	* [File system implementations](#file-system-implementations-themselves)



# Overview
The mount directory contains various packages to facilitate mounting various file systems to various hosts using various APIs.  
(Note: "/mount" may change to "/filesystem" as we provide multiple filesystem interfaces. In addition to mounting them, they're useful on their own within Go, and in the case of 9P will be allowed to spawn a socket without calling the system's `mount`)
```
./mount
├── conductors (implementation(s) of the `mount.Conductor` interface)
│   └── ipfs-core
├── interface (various interface definitions used by the mount system)
├── providers (implementations of the `mount.Provider` interface)
│   ├── 9P (implements file systems via the Plan 9 protocol)
│   │   └── filesystems (common base for filesystems to be built on)
│   │       ├── ipfs (9P operation semantics and mappings from intermediate formats to 9P)
│   │       ├── ipns (etc...)
│   │       ├── keyfs
│   │       ├── mfs
│   │       ├── overlay
│   │       └── pinfs
│   └───fuse
│       └───filesystems (common base for filesystems to be built on)
│           ├───core (FUSE operation semantics and mappings from intermediate formats to FUSE)
│           ├───internal
│           │   └───testutils
│           ├───keyfs (etc...)
│           ├───mfs
│           ├───overlay
│           └───pinfs
└── utils (some of the things in here will likely move)
    ├── cmds (hosts the parameters and sub-commands for `daemon`, `mount`, `unmount`)
    ├── common
    ├── sys (interactions with the host OS such as mounting, target defaults, etc.)
    └───transform (wrap IPFS API constructs into intermediate formats)
        └───filesystems
            ├───empty
            ├───ipfscore (translates core paths into `transform.File` and `transform.Directory`)
            ├───keyfs (etc...)
            ├───mfs
            └───pinfs
```
Primarily, the packages are used to construct a "conductor" (it's like a volume manager) and bind it to the IPFS daemon/node instance.  

The conductor will facilitate management of file system "providers" (they're like volume constructors).  

"Providers" provide implementations of file systems, and facilities to graft them to some target.  
Typically this will be mounting file systems to a path in the host system.  
(e.g. mounting the abstract namespace "IPFS" to the local path `/ipfs`  via the FUSE API)

## Important interfaces
### Command line
The command line sub-commands `ipfs daemon` and `ipfs mount` and parsers for their parameters live in `mount/utils/cmd`.  
Values are populated (in priority order) from the parameters of the sub-command, the node's config file, or fall back to a platform suggested dynamic default. Feeding them into the underlying go interfaces.  

Issuing `ipfs mount` will mount a set of targets based on the above, but may be customized using combinations of parameters. A complex example would be `ipfs mount --provider=Plan9Protocol --namespace="IPFS,IPNS,FilesAPI" --target="/ipfs,/ipns,/file"` which mimic's the current defaults on Linux (when 9P is loaded in the kernel), more explicitly.


Anything that can be determined by the implementation may be omitted.  Such as the provider, or the targets if they're within your config file.  
e.g. `ipfs mount --namespace="IPFS"` is valid and would expand to `ipfs mount --provider=Plan9Protocol --namespace="IPFS" --target="$(ipfs config Mounts.IPFS)"`  
Assume you unload 9P support from the kernel and make the same call, `ipfs mount --namespace="IPFS"` would now expand to `ipfs mount --provider=FUSE --namespace="IPFS" --target="$(ipfs config Mounts.IPFS)"` 
automatically.

It is also possible to specify any combination of namespaces and targets so long as the argument count matches.  
For example, this is a valid way to map IPFS to 2 different mountpoints `ipfs mount --namespace="IPFS,IPFS" -target="/ipfs,/mnt/ipfs"`  

At any time, you may list the currently active mounts via `ipfs mount --list` or shorthand `ipfs mount -l`
(NOTE: this works but it's not pretty printed yet)

`ipfs unmount` shares the same parameters as `ipfs mount` with the addition of a `-a` to unmount all previously mounted targets

`ipfs daemon` shares the same parameters as `ipfs mount` simply prefixed with `--mount-`.  
e.g. `ipfs daemon --mount --mount-provider="FUSE" --mount-namespace="IPFS,IPNS" --mount-target="/ipfs,/ipns"`  
It carries the same auto expansion rules, picking up missing parameters through the same deduction methods. (checks arguments, then config, then environment)

### Conductors
(Note: I don't like this name but couldn't think of anything better; something like volume manager wouldn't be bad)  
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
Simply, provider instances are instances generated by the provider that should be tracked by the caller that generated them. In our case this is the conductor which maps a series of targets to instances, allowing callers to detach these instances by name/path. Following the traditional model of volumes, you can think of these almost as active partitions of a volume.
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
### Cross boundary locking
In order to allow the daemon to perform normal operations without locking the user out of certain features, such as publishing to IPNS keys or using the FilesAPI via the `ipfs` command, or other API instances. We'll want to incorporate a shared resource lock on the daemon for these namespaces to use.
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

### File system implementations themselves
~~Currently there are 2 separate file system APIs that themselves implement mappings for various IPFS api's.
1 for FUSE and 1 for 9P. They're fairly distinct but I'm going to put effort into trying to generalize and overlap as much as possible via a transform package.  
An example of this is not implementing 2 different forms of `Getattr` 1 for each API, instead we map from IPFS semantics to some intermediate representation.  
`(mount/utils/transform).CoreGetAttr(ctx, corepath, core, request)`, returns some intermediate object that itself implements transforms `object.ToFuse() *fuselib.Stat_t`, `object.To9P() *p9plib.Attr`.  
There will likely be other ways we can find overlap to provide generalized code over specific code. Allowing for uniformity, as well as more code coverage with less tests.  
Intermediate wrappers for file I/O that wrap the Core+MFS apis to make the layers for fuse.Read and p9.Read smaller. e.g. via something like `intermediate.CoreOpenFile(...) io.ReadWriter` is being tested.~~  
^ This happened but is a work in progress  
(there's a lot of lint that needs to be removed, and everything needs testing, but in general it's nicer)  

Mappings from the core api's get wrapped in an intermediate layer that then gets further transformed in external API specific ways. For example this is the `Gettattr` for IPFS under fuse
```go
		...

		fullPath := corepath.New(gopath.Join("/", strings.ToLower(fs.namespace.String()), path))

		iStat, _, err := transform.GetAttr(fs.Ctx(), fullPath, fs.Core(), transform.IPFSStatRequestAll)
		if err != nil {
			fs.log.Error(err)
			return -fuselib.ENOENT
		}

		*stat = *iStat.ToFuse()
		fusecom.ApplyPermissions(readOnly, &stat.Mode)
		stat.Uid, stat.Gid, _ = fuselib.Getcontext()
		return fusecom.OperationSuccess
```
and under 9P
```go
	...

	iStat, iFilled, err := transform.GetAttr(callCtx, id.CorePath(), id.Core, transform.RequestFrom9P(req))
	if err != nil {
		id.Logger.Error(err)
		return qid, iFilled.To9P(), iStat.To9P(), err
	}
	nineAttr, nineFilled := iStat.To9P(), iFilled.To9P()

	if req.Mode { // UFS provides type bits, we provide permission bits
		nineAttr.Mode |= common.IRXA
	}

	return qid, nineFilled, nineAttr, err

```


A version of the `pinfs` (a directory which lists the node's pins as files and directories) has been implemented using this method. ~~Its use within FUSE looks like this:~~  
This is how it was, but it's in the process of being changed for standards compliance.
## Old
___
```go 
// OpenDir(){
dir, err := transform.OpenDirPinfs(fs.Ctx(), fs.Core())
// Readdir{
entChan, err := fs.pinDir.Readdir(offset, requestedEntryCount).ToFuse()
for ent := range entChan {
	fill(ent.Name, ent.Stat, ent.Offset)
}
return OperationSuccess
```
Used within 9P, it's very similar

```go 
// Open(){
dir, err := transform.OpenDirPinfs(fs.Ctx(), fs.Core())
// Readdir(offset, count) (p9.Dirents, error) {
return fs.pinDir.Readdir(offset, count).To9P()
```
The interface is still in progress, but currently looks like this
```go
type Directory interface {
	// Readdir returns /at most/ count entries; or attempts to return all entires when count is 0
	Readdir(offset, count uint64) DirectoryState
	io.Closer
}

// TODO: better name
type DirectoryState interface {
	// TODO: for Go and 9P, allow the user to pass in a pre-allocated slice (or nil)
	// same for Fuse but with a channel, in case they want it buffered
	// NOTE: pre-allocated/defined inputs are optional and should be allocated internally if nil
	// channels must be closed by the method
	To9P() (p9.Dirents, error)
	ToGo() ([]os.FileInfo, error)
	ToGoC(predefined chan os.FileInfo) (<-chan os.FileInfo, error)
	ToFuse() (<-chan FuseStatGroup, error)
}
```

## WIP (may change)
___

```go 
// OpenDir(){
dir, err := transform.OpenDirPinfs(fs.Ctx(), fs.Core())
// Readdir{
entChan, err := fs.pinDir.Readdir(readDirCtx, offset).ToFuse()
for ent := range entChan {
	fill(ent.Name, ent.Stat, ent.Offset)
}
return OperationSuccess
```

```go
type Directory interface {
	// Readdir returns attempts to return all entires starting from offset until it reaches the end
	// or the context is canceled
	Readdir(ctx context.Context, offset uint64) DirectoryState
	io.Closer
}

type DirectoryState interface {
	// one of these most likely
	To9P(count) (p9.Dirents, error)
	To9P() (<-p9.Dirent, error)
	...
	// not likely
	ToFuse(fillerFunc) error
}
```

Misc Notes
___
### NetBSD
is only allowing 1 mountpoint to be active at a time, if a second mountpoint is requested, it will be mapped, but the previous mountpoint will be overtaken by the new one.  
e.g. consider the sequence:  
`ipfs mount --namespace=pinfs --target=/ipfs` will mount the pinfs to `/ipfs`  
`ipfs mount --namespace=keyfs --target=/ipns` will mount the keyfs to `/ipns`  
at this moment, listing either `/ipfs` or `/ipns` will return results from the keyfs.  
This is likely a cgofuse bug, needs looking into.  
Otherwise, things seem to work as expected.  
(Env: NetBSD 9.0, Go 1.13.9)


### OpenBSD
is allowing traversal and `cat`ing of files, but `getdents` is failing in `ls`.
```
 57206 ls       CALL  fstat(4,0x7f7ffffd9fe8)
 57206 ls       STRU  struct stat { dev=9733, ino=1, mode=dr-xr-xr-- , nlink=0, uid=0<"root">, gid=0<"wheel">, rdev=0, atime=0, mtime=0, ctime=0, size=0, blocks=4, blksize=512, flags=0x0, gen=0x0 }
 57206 ls       RET   fstat 0
 57206 ls       CALL  fchdir(4)
 57206 ls       RET   fchdir 0
 57206 ls       CALL  getdents(4,0x95a668ff000,0x1000)
 57206 ls       RET   getdents -1 errno 2 No such file or directory
 57206 ls       CALL  close(4)
 57206 ls       RET   close 0
```
The daemon is receiving a very large offset/`seekdir` value for some reason.
```
2020-05-07T05:32:11.856-0400    DEBUG   fuse/pinfs      pinfs/pinfs.go:90       Getattr - {FFFFFFFFFFFFFFFF}"/"
2020-05-07T05:32:11.856-0400    DEBUG   fuse/pinfs      pinfs/pinfs.go:90       Getattr - {FFFFFFFFFFFFFFFF}"/"
2020-05-07T05:32:11.856-0400    DEBUG   fuse/pinfs      pinfs/pinfs.go:113      Opendir - "/"
2020-05-07T05:32:11.856-0400    DEBUG   fuse/pinfs      pinfs/pinfs.go:90       Getattr - {FFFFFFFFFFFFFFFF}"/"
2020-05-07T05:32:11.856-0400    DEBUG   fuse/pinfs      pinfs/pinfs.go:157      Readdir - {1|0}"/"
2020-05-07T05:32:11.856-0400    DEBUG   fuse/pinfs      pinfs/pinfs.go:157      Readdir - {1|208}"/"
2020-05-07T05:32:11.856-0400    ERROR   fuse/pinfs      pinfs/pinfs.go:171      offset 206 is not/no-longer valid
2020-05-07T05:32:11.857-0400    DEBUG   fuse/pinfs      pinfs/pinfs.go:139      Releasedir - {1}"/"
2020-05-07T05:32:11.857-0400    DEBUG   fuse/pinfs      pinfs/pinfs.go:90       Getattr - {FFFFFFFFFFFFFFFF}"/"
2020-05-07T05:32:11.857-0400    DEBUG   fuse/pinfs      pinfs/pinfs.go:113      Opendir - "/"
2020-05-07T05:32:11.857-0400    DEBUG   fuse/pinfs      pinfs/pinfs.go:90       Getattr - {FFFFFFFFFFFFFFFF}"/"
2020-05-07T05:32:11.857-0400    DEBUG   fuse/pinfs      pinfs/pinfs.go:90       Getattr - {FFFFFFFFFFFFFFFF}"/"
2020-05-07T05:32:11.857-0400    DEBUG   fuse/pinfs      pinfs/pinfs.go:90       Getattr - {FFFFFFFFFFFFFFFF}"/"
2020-05-07T05:32:11.857-0400    DEBUG   fuse/pinfs      pinfs/pinfs.go:157      Readdir - {2|0}"/"
2020-05-07T05:32:11.857-0400    DEBUG   fuse/pinfs      pinfs/pinfs.go:157      Readdir - {2|208}"/"
2020-05-07T05:32:11.857-0400    ERROR   fuse/pinfs      pinfs/pinfs.go:171      offset 206 is not/no-longer valid
2020-05-07T05:32:11.857-0400    DEBUG   fuse/pinfs      pinfs/pinfs.go:139      Releasedir - {2}"/"
```
Readdir tests are passing within Go on the platform, so this is likely a cgofuse issue.  
This is also the only platform currently where `ls` doesn't work.  
Needs investigating.  
(Env: OpenBSD 6.6, Go 1.13.1)