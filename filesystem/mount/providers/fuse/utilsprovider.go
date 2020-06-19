package fuse

import (
	"fmt"
	"os"
	"os/user"
	"runtime"
	"strings"

	mountinter "github.com/ipfs/go-ipfs/filesystem/mount"
)

func fuseArgs(target string, namespace mountinter.Namespace) (string, []string) {
	var (
		retTarget, opts string
		args            []string
	)

	switch runtime.GOOS {
	default:
		retTarget = target

	case "windows": // expected target is WinFSP; use its options
		// cgofuse expects an argument format broken up by components
		// e.g. `mount.exe -o "uid=-1,volname=a valid name,gid=-1" --VolumePrefix=\localhost\UNC`
		// is equivalent to this in Go:
		//`[]string{"-o", "uid=-1,volname=a valid name,gid=-1", "--VolumePrefix=\\localhost\\UNC"}`
		// refer to the WinFSP documentation for expected parameters and their literal format

		// basic info
		if namespace == mountinter.NamespaceCombined {
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
	case "freebsd":
		if namespace == mountinter.NamespaceCombined {
			opts = "fsname=IPFS,subtype=IPFS"
		} else {
			opts = fmt.Sprintf("fsname=%s,subtype=%s", namespace.String(), namespace.String())
		}

		// TODO: [general] we should allow the user to pass in raw options
		// that we will then relay to the underlying fuse implementation, unaltered
		// options like `allow_other` depend on opinions of the sysop, not us
		// so we shouldn't just assume this is what they want
		if os.Geteuid() == 0 { // if root, allow other users to access the mount
			opts += ",allow_other" // allow users besides root to see and access the mount

			//opts += ",default_permissions"
			// TODO: [cli, constructors]
			// for now, `default_permissions` won't prevent anything
			// since we tell whoever is calling that they own the file, regardless of who it is
			// we need a way for the user to set `uid` and `gid` values
			// both for our internal context (getattr)
			// as well as allowing them to pass the uid= and gid= FUSE options (not specifically, pass anything)
			// (^system ignores our values and substitutes its own)
		}

		args = append(args, "-o", opts)
		retTarget = target

	case "openbsd":
		args = append(args, "-o", "allow_other")
		retTarget = target

	case "darwin":
		if namespace == mountinter.NamespaceCombined {
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

	case "linux":
		// [2020.04.18] cgofuse currently backed by hanwen/go-fuse on linux; their optset doesn't support our desire
		// libfuse: opts = fmt.Sprintf(`-o fsname=ipfs,subtype=fuse.%s`, namespace.String())
		retTarget = target
	}

	return retTarget, args
}
