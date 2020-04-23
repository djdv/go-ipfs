package fusecommon

import (
	"path/filepath"
	"unsafe"

	"github.com/billziss-gh/cgofuse/fuse"
	fuselib "github.com/billziss-gh/cgofuse/fuse"
	"golang.org/x/sys/windows"
)

func init() { Statfs = statfsWin }

const LOAD_LIBRARY_SEARCH_SYSTEM32 = 0x00000800

// TODO review

func loadSystemDLL(name string) (*windows.DLL, error) {
	modHandle, err := windows.LoadLibraryEx(name, 0, LOAD_LIBRARY_SEARCH_SYSTEM32)
	if err != nil {
		return nil, err
	}
	return &windows.DLL{Name: name, Handle: modHandle}, nil
}

func statfsWin(path string, fStatfs *fuselib.Statfs_t) (error, int) {
	mod, err := loadSystemDLL("kernel32.dll")
	if err != nil {
		return err, -fuselib.ENOMEM // kind of true, probably better than EIO
	}
	defer mod.Release()

	proc, err := mod.FindProc("GetDiskFreeSpaceExW")
	if err != nil {
		return err, -fuselib.ENOMEM // kind of true, probably better than EIO
	}

	var (
		FreeBytesAvailableToCaller,
		TotalNumberOfBytes,
		TotalNumberOfFreeBytes uint64

		SectorsPerCluster,
		BytesPerSector uint16
		//NumberOfFreeClusters,
		//TotalNumberOfClusters uint16
	)
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err, -fuse.EFAULT // caller should check for syscall.EINVAL; NUL byte was in string
	}

	r1, _, wErr := proc.Call(uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&FreeBytesAvailableToCaller)),
		uintptr(unsafe.Pointer(&TotalNumberOfBytes)),
		uintptr(unsafe.Pointer(&TotalNumberOfFreeBytes)),
	)
	if r1 == 0 {
		return wErr, -fuselib.ENOMEM
	}

	proc, _ = mod.FindProc("GetDiskFreeSpaceW")
	r1, _, wErr = proc.Call(uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&SectorsPerCluster)),
		uintptr(unsafe.Pointer(&BytesPerSector)),
		//uintptr(unsafe.Pointer(&NumberOfFreeClusters)),
		0,
		//uintptr(unsafe.Pointer(&TotalNumberOfClusters)),
		0,
	)
	if r1 == 0 {
		return wErr, -fuselib.EIO
	}

	var (
		componentLimit = new(uint32)
		volumeFlags    = new(uint32)
		volumeSerial   = new(uint32)
	)

	volumeRoot := filepath.VolumeName(path) + string(filepath.Separator)
	pathPtr, err = windows.UTF16PtrFromString(volumeRoot)
	if err != nil {
		return err, -fuse.EFAULT // caller should check for syscall.EINVAL; NUL byte was in string
	}

	if err = windows.GetVolumeInformation(pathPtr, nil, 0, volumeSerial, componentLimit, volumeFlags, nil, 0); err != nil {
		return err, -fuselib.EIO
	}

	fStatfs.Bsize = uint64(SectorsPerCluster * BytesPerSector)
	fStatfs.Frsize = uint64(BytesPerSector)
	fStatfs.Blocks = TotalNumberOfBytes / uint64(BytesPerSector)
	fStatfs.Bfree = TotalNumberOfFreeBytes / (uint64(BytesPerSector))
	fStatfs.Bavail = FreeBytesAvailableToCaller / (uint64(BytesPerSector))
	fStatfs.Files = ^uint64(0)

	// TODO: these have to come from our own file table
	// fStatfs.Ffree = fs.AvailableHandles()
	// fStatfs.Favail = fStatfs.Ffree

	fStatfs.Namemax = uint64(*componentLimit)

	// cgofuse ignores these but we have them
	fStatfs.Flag = uint64(*volumeFlags)
	fStatfs.Fsid = uint64(*volumeSerial)

	return nil, OperationSuccess
}

// TODO: [anyone] Replace with `windows.GetVersion()` when this is resolved: https://github.com/golang/go/issues/17835
func rawWinver() (major, minor, build uint32) {
	type rtlOSVersionInfo struct {
		dwOSVersionInfoSize uint32
		dwMajorVersion      uint32
		dwMinorVersion      uint32
		dwBuildNumber       uint32
		dwPlatformId        uint32
		szCSDVersion        [128]byte
	}

	ntoskrnl := windows.MustLoadDLL("ntoskrnl.exe")
	defer ntoskrnl.Release()

	proc := ntoskrnl.MustFindProc("RtlGetVersion")

	var verStruct rtlOSVersionInfo
	verStruct.dwOSVersionInfoSize = uint32(unsafe.Sizeof(verStruct))
	proc.Call(uintptr(unsafe.Pointer(&verStruct)))

	return verStruct.dwMajorVersion, verStruct.dwMinorVersion, verStruct.dwBuildNumber
}
