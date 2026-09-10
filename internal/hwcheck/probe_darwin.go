//go:build darwin

package hwcheck

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func sysctl(key string) string {
	out, err := exec.Command("sysctl", "-n", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func cpuModel() string {
	if v := sysctl("machdep.cpu.brand_string"); v != "" {
		return v
	}
	return sysctl("hw.model")
}

func totalRAM() uint64 {
	n, err := strconv.ParseUint(sysctl("hw.memsize"), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func freeDisk(dir string) uint64 {
	if dir == "" {
		return 0
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0
	}
	return st.Bavail * uint64(st.Bsize)
}

func virtualization() (Tri, string) {
	switch sysctl("kern.hv_support") {
	case "1":
		return Yes, "Hypervisor.framework supported"
	case "0":
		return No, "Hypervisor.framework unsupported on this Mac"
	}
	return Unknown, ""
}
