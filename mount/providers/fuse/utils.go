package mountfuse

import (
	"errors"
	"fmt"
	"os/user"
	"runtime"
	"strings"

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
		// cgofuse expects an argument format broken up by components
		// e.g. `mount.exe -o "uid=-1,volname=a valid name,gid=-1" --VolumePrefix=\localhost\UNC`
		// is equivalent to this in Go:
		//`[]string{"-o", "uid=-1,volname=a valid name,gid=-1", "--VolumePrefix=\\localhost\\UNC"}`
		// refer to the WinFSP documentation for expected parameters and their literal format

		// basic info
		if namespace == mountinter.NamespaceAllInOne {
			opts = "FileSystemName=IPFS,volname=IPFS"
		} else {
			opts = fmt.Sprintf("FileSystemName=%s,volname=%s", namespace.String(), namespace.String())
		}
		// set the owner to be the same as the process (`daemon`'s or `mount`'s depending on background/foreground)
		opts += ",uid=-1,gid=-1"
		args = append(args, "-o", opts)

		// convert UNC targets to WinFSP format
		if len(target) > 2 && target[:2] == `\\` {
			// NOTE: cgo-fuse/WinFSP UNC parameter uses single slash prefix
			args = append(args, fmt.Sprintf(`--VolumePrefix=%s`, target[1:]))
			break // don't set target value; UNC is handled by `VolumePrefix`
		}

		// target is local reference; use it
		retTarget = target
	case "linux":
		// [2020.04.18] cgofuse currently backed by hanwen/go-fuse on linux; their optset doesn't support our desire
		// libfuse: opts = fmt.Sprintf(`-o fsname=ipfs,subtype=fuse.%s`, namespace.String())
		fallthrough
	case "darwin":
		if namespace == mountinter.NamespaceAllInOne {
			// TODO: see if we can provide `volicon` via an IPFS path; or make the overlay provide one via `/.VolumeIcon.icns` on darwin
			opts = "fsname=IPFS,volname=IPFS"
		} else {
			opts = fmt.Sprintf("fsname=%s,volname=%s", namespace.String(), namespace.String())
		}

		args = append(args, "-o", opts)

		// TODO reconsider if we should leave this hack in
		// macfuse takes this literally and will make a mountpoint as `./~/target` not `/home/user/target`
		if strings.HasPrefix(target, "~") {
			usr, err := user.Current()
			if err != nil {
				panic(err)
			}
			retTarget = usr.HomeDir + target[1:]
			break
		}

		retTarget = target

	default:
		retTarget = target
	}

	return retTarget, args
}
