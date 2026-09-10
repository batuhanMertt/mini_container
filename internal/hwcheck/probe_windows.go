//go:build windows

package hwcheck

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32                      = syscall.NewLazyDLL("kernel32.dll")
	procGlobalMemoryStatusEx      = kernel32.NewProc("GlobalMemoryStatusEx")
	procGetDiskFreeSpaceExW       = kernel32.NewProc("GetDiskFreeSpaceExW")
	procIsProcessorFeaturePresent = kernel32.NewProc("IsProcessorFeaturePresent")
)

// PF_VIRT_FIRMWARE_ENABLED: the firmware has enabled virtualization extensions.
const pfVirtFirmwareEnabled = 21

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

func cpuModel() string {
	// PROCESSOR_IDENTIFIER is set by the OS on every Windows session and avoids
	// a registry round-trip just to print a label.
	return os.Getenv("PROCESSOR_IDENTIFIER")
}

func totalRAM() uint64 {
	var m memoryStatusEx
	m.Length = uint32(unsafe.Sizeof(m))
	ret, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m)))
	if ret == 0 {
		return 0
	}
	return m.TotalPhys
}

func freeDisk(dir string) uint64 {
	if dir == "" {
		return 0
	}
	p, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return 0
	}
	var freeToCaller, total, free uint64
	ret, _, _ := procGetDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(&freeToCaller)),
		uintptr(unsafe.Pointer(&total)),
		uintptr(unsafe.Pointer(&free)),
	)
	if ret == 0 {
		return 0
	}
	return freeToCaller
}

func virtualization() (Tri, string) {
	ret, _, _ := procIsProcessorFeaturePresent.Call(uintptr(pfVirtFirmwareEnabled))
	if ret != 0 {
		return Yes, "VT-x/AMD-V enabled in firmware"
	}
	// The flag is also reported as absent while a hypervisor (Hyper-V, WSL2)
	// already owns the extensions, so absence is not proof of absence.
	return Unknown, "firmware flag not set; a running hypervisor can mask it"
}
