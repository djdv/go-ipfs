package mountfuse

import (
	"errors"
	"fmt"
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

/* TODO: this should be more dynamic and flexible
 we should comprehend various targets and use them accordingly
 e.g. if target starts with drivespec `I:\ipfs`, mount as folder
 if target IS drivespec `I:` mount as a drive
 if target is `\\`, map to `\\localhost\$Namespace`
 if target starts with UNC `\\cool`, map to UNC space `\\localhost\$Namespace\cool`
 cmds pkg will have to handle platform specific target transfomarion
e.g. default `/ipfs` on Windows should probably map to `\\` as our input and thus result in `\\localhost\ipfs` output
*/
func fuseArgs(target string, namespace mountinter.Namespace) (string, []string) {
	args := []string{fmt.Sprintf("-o uid=-1,gid=-1,FileSystemName=%s", namespace.String())}

	if runtime.GOOS == "windows" {
		// TODO: this shouldn't be handled here; but where?
		// we also need a way to figure out which drive letters are free and use one, preffering `I:` but not requiring it
		if namespace == mountinter.NamespaceAllInOne {
			return "I:", []string{"-o uid=-1,gid=-1,FileSystemName=IPFS"}
		}

		args = append(args, fmt.Sprintf(`--VolumePrefix=\localhost\%s`, strings.ToLower(namespace.String())))
		return "", args
	}

	return target, args
}
