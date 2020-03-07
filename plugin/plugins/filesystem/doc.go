/*
Package filesystem implements the go-ipfs daemon plugin interface
and defines the plugin's config structure. The plugin itself exposes file system services over a multiaddr listener.

By default, it tries to expose the IPFS namespace using the 9P2000.L protocol, over a unix domain socket
(located at $IPFS_PATH/filesystem.9P.sock using config template $IPFS_HOME/filesystem.9P.sock)

To change the multiaddr listen address, you may set the option in the node's config file
via `ipfs config "Plugins.Plugins.filesystem.Config.Service.9p" "/ip4/127.0.0.1/tcp/564"`
To disable this plugin entirely, use: `ipfs config --bool "Plugins.Plugins.filesystem.Disabled" "true"``
*/
package filesystem
