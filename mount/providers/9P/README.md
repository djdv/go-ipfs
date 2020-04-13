## Filesystem API Plugin

This daemon plugin wraps the IPFS node and exposes file system services over a multiaddr listener.  
Currently using the 9P2000.L protocol, and offering read support for `/ipfs`, `/ipns`, and `/file`(`ipfs files` root) requests. With writable support for `/ipns` and `/file`.

You may connect to this service using the [`v9fs`](https://git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git/plain/Documentation/filesystems/9p.txt) client used in the Linux kernel.
By default we listen for requests on a Unix domain socket.
`mount -t 9p -o trans=unix $IPFS_PATH/filesystem.9p.sock ~/ipfs-mount`

To change the multiaddr listen address, you may set the option in the node's config file
via `ipfs config "Plugins.Plugins.filesystem.Config.Service.9p" "/ip4/127.0.0.1/tcp/564"`
To disable this plugin entirely, use: `ipfs config --bool "Plugins.Plugins.filesystem.Disabled" "true"``

See the [v9fs documentation](https://git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git/plain/Documentation/filesystems/9p.txt) for instructions on using transports, such as TCP.
i.e.
```
> ipfs config "Plugins.Plugins.filesystem.Config.Service.9p" "/ip4/127.0.0.1/tcp/564"
> ipfs daemon &
> mount -t 9p 127.0.0.1 ~/ipfs-mount
> ...
> umount ~/ipfs-mount
> ipfs shutdown
```

Note that the default TCP port for 9P is 564, which is a privileged port, and that most systems require special permissions for a user to be able to mount filesystems.  

To avoid permission issues on the server side, you can use a non-privileged port (anything higher than 1024).
Making sure client specify it during the `mount` command: `mount -t 9p -o port=1234 127.0.0.1 ~/ipfs-mount`  

On the client side, the server does not care what clients connect to it, as long as they speak the protocol.
Meaning that you may host and connect through any combination of kernel-space or userland code.
i.e. You may use a client library such as  https://github.com/hugelgupf/p9/ to create a client connection from user-space in a program of your own, irrelevant of the environment.

### How to enable

See the Plugin documentation [here](https://github.com/ipfs/go-ipfs/blob/master/docs/plugins.md#installing-plugins).
You will likely want to add the plugin to the `go-ipfs` plugin pre-load list
`filesystem github.com/ipfs/go-ipfs/plugin/plugins/filesystem *`