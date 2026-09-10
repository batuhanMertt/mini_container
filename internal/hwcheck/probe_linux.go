//go:build linux

package hwcheck

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

func cpuModel() string {
	b, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "model name", "Model", "Hardware":
			return strings.TrimSpace(val)
		}
	}
	return ""
}

func totalRAM() uint64 {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0
		}
		kb, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0
		}
		return kb * 1024
	}
	return 0
}

func freeDisk(dir string) uint64 {
	if dir == "" {
		return 0
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0
	}
	if st.Bsize < 0 {
		return 0
	}
	return st.Bavail * uint64(st.Bsize)
}

func virtualization() (Tri, string) {
	// Only meaningful on x86; the launcher runs the runtime natively on Linux
	// anyway, so this is informational.
	b, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return Unknown, ""
	}
	s := string(b)
	if strings.Contains(s, " vmx") || strings.Contains(s, " svm") {
		return Yes, "VT-x/AMD-V exposed to the OS"
	}
	return Unknown, "no vmx/svm flag (normal on ARM)"
}
