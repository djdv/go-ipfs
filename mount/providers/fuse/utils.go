package mountfuse

import (
	"errors"
	"fmt"
	"runtime"

	mountinter "github.com/ipfs/go-ipfs/mount/interface"
)

func cgofuseRecover(errPtr *error) {
	if r := recover(); r != nil {
		if typedR, ok := r.(string); ok {
			if runtime.GOOS == "windows" {
				if typedR == "cgofuse: cannot find winfsp" {
					*errPtr = errors.New("WinFSP(http://www.secfs.net/winfsp/) is required for mount on this platform, but it was not found")
					return
				}
			}

			*errPtr = fmt.Errorf("mount panicked %v", r)
			return
		}
	}
}

func fuseArgs(target string, namespace mountinter.Namespace) (string, []string) {
	var (
		retTarget, opts string
		args            []string
	)

	switch runtime.GOOS {
	case "windows": // expected target is WinFSP; use its options
		// basic info
		if namespace == mountinter.NamespaceAllInOne {
			opts = `-o FileSystemName="IPFS",volname="IPFS"`
		} else {
			opts = fmt.Sprintf("-o FileSystemName=%q,volname=%q", namespace.String(), namespace.String())
		}
		// set the owner to be the same as the process (`daemon`'s or `mount`'s depending on background/foreground)
		opts += ",uid=-1,gid=-1"

		// convert UNC targets to WinFSP format
		if len(target) > 2 && target[:2] == `\\` {
			// NOTE: cgo-fuse/WinFSP UNC parameter uses single slash prefix
			args = append(args, opts, fmt.Sprintf(`--VolumePrefix=%q`, target[1:]))
			break // don't set target value; UNC is handled by `VolumePrefix`
		}

		// target is local reference; use it
		retTarget = target
	case "linux":
		// [2020.04.18] cgofuse currently backed by hanwen/go-fuse on linux; their optset doesn't support our desire
		// libfuse: opts = fmt.Sprintf(`-o fsname="ipfs",subtype="fuse.%s"`, namespace.String())
		fallthrough
	default:
		retTarget = target
	}

	return retTarget, args
}
