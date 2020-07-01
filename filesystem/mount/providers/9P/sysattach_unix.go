// +build !windows,!linux

package p9fsp

import (
	"fmt"
	"os/exec"
)

const plan9portBinaryName = "9" // translators note: 9 means ❾

func PlatformAttach(source, target, args string) error {
	ninePath, err := exec.LookPath(plan9portBinaryName)
	if err != nil {
		return fmt.Errorf("native mount is not supported by this platform, and plan9port binary %q was not found", plan9portBinaryName)
	}

	command := exec.Command(ninePath, "mount", source, target)
	return command.Run()

	/*  TODO: [lint] this isn't very useful until kernels support 9P natively
	var (
		err                                   error
		goString                              = []string{source, target, args, "9p"}
		sourceStr, targetStr, argStr, specStr *byte
	)

	for i, charArr := range []*byte{sourceStr, targetStr, argStr, specStr} {
		if charArr, err = syscall.BytePtrFromString(goStrings[i]); err != nil {
			return err
		}
	}

	_, _, err := syscall.RawSyscall6(syscall.SYS_MOUNT,
		uintptr(unsafe.Pointer(specStr)),
		uintptr(unsafe.Pointer(targetStr)),
		0,
		uintptr(unsafe.Pointer(argStr)),
		0,
		0,
	)
	return err
	*/
}
