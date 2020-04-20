package transform

import "runtime"

// NOTE: [b7952c54-1614-45ea-a042-7cfae90c5361] cgofuse only supports ReaddirPlus on Windows
// if this ever changes (bumps libfuse from 2.8 -> 3.X+), add platform support here (and to any other tags with this UUID)
// TODO: this would be best in the fuselib itself; make a patch upstream
var CanReaddirPlus bool = runtime.GOOS == "windows"
