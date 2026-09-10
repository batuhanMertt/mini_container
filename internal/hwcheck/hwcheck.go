// Package hwcheck inspects the host hardware and reports whether it can run
// the mini-container runtime, and which Linux binary flavour it needs.
package hwcheck

import (
	"fmt"
	"runtime"
	"strings"
)

// Tri is a three-valued answer: some capabilities cannot be probed reliably
// from userspace, and "unknown" is a more honest answer than a guess.
type Tri int

const (
	Unknown Tri = iota
	No
	Yes
)

func (t Tri) String() string {
	switch t {
	case Yes:
		return "yes"
	case No:
		return "no"
	default:
		return "unknown"
	}
}

// Minimum resources for the runtime plus an Alpine minirootfs. The container
// itself is capped at 100MB, so these are deliberately modest.
const (
	MinCores    = 1
	MinTotalRAM = 1 << 30 // 1 GiB
	MinFreeDisk = 1 << 29 // 512 MiB
)

// Report is the outcome of a host inspection.
type Report struct {
	OS       string // runtime.GOOS
	HostArch string // runtime.GOARCH
	CPUModel string
	Cores    int
	TotalRAM uint64 // bytes; 0 when it could not be read
	FreeDisk uint64 // bytes on the cache volume; 0 when it could not be read
	Virt     Tri    // hardware virtualization available to a hypervisor
	VirtNote string
	Warnings []string
}

// Detect inspects the host. It never fails: fields it cannot determine stay
// zero-valued and are reported as "unknown".
func Detect(cacheDir string) Report {
	r := Report{
		OS:       runtime.GOOS,
		HostArch: runtime.GOARCH,
		Cores:    runtime.NumCPU(),
	}
	r.CPUModel = cpuModel()
	r.TotalRAM = totalRAM()
	r.FreeDisk = freeDisk(cacheDir)
	r.Virt, r.VirtNote = virtualization()

	if r.Cores < MinCores {
		r.Warnings = append(r.Warnings, fmt.Sprintf("only %d CPU core(s) detected", r.Cores))
	}
	if r.TotalRAM != 0 && r.TotalRAM < MinTotalRAM {
		r.Warnings = append(r.Warnings, fmt.Sprintf("only %s RAM detected; %s recommended",
			HumanBytes(r.TotalRAM), HumanBytes(MinTotalRAM)))
	}
	if r.FreeDisk != 0 && r.FreeDisk < MinFreeDisk {
		r.Warnings = append(r.Warnings, fmt.Sprintf("only %s free disk on the cache volume; %s recommended",
			HumanBytes(r.FreeDisk), HumanBytes(MinFreeDisk)))
	}
	if _, ok := LinuxArch(r.HostArch); !ok && r.OS != "linux" {
		r.Warnings = append(r.Warnings, fmt.Sprintf("host architecture %q has no published Linux build", r.HostArch))
	}
	return r
}

// LinuxArch maps a GOARCH to the architecture suffix used by the published
// Linux release assets.
func LinuxArch(goarch string) (string, bool) {
	switch goarch {
	case "amd64", "arm64":
		return goarch, true
	case "386":
		// 32-bit hosts run a 64-bit kernel often enough that amd64 is the
		// better guess, but the backend probe is authoritative.
		return "amd64", true
	}
	return "", false
}

// NormalizeUname maps `uname -m` output from inside a Linux environment onto a
// release asset architecture. The backend is authoritative: an arm64 Mac can
// run an amd64 Docker VM, and vice versa.
func NormalizeUname(machine string) (string, bool) {
	switch strings.TrimSpace(machine) {
	case "x86_64", "amd64":
		return "amd64", true
	case "aarch64", "arm64":
		return "arm64", true
	}
	return "", false
}

// AlpineArch maps a release architecture onto Alpine's rootfs naming.
func AlpineArch(arch string) (string, bool) {
	switch arch {
	case "amd64":
		return "x86_64", true
	case "arm64":
		return "aarch64", true
	}
	return "", false
}

// HumanBytes renders a byte count with binary units.
func HumanBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

func bytesOrUnknown(b uint64) string {
	if b == 0 {
		return "unknown"
	}
	return HumanBytes(b)
}

// String renders the report as the aligned block printed before a run.
func (r Report) String() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "  OS         : %s/%s\n", r.OS, r.HostArch)
	fmt.Fprintf(&sb, "  CPU        : %s (%d cores)\n", orUnknown(r.CPUModel), r.Cores)
	fmt.Fprintf(&sb, "  RAM        : %s\n", bytesOrUnknown(r.TotalRAM))
	fmt.Fprintf(&sb, "  Free disk  : %s\n", bytesOrUnknown(r.FreeDisk))
	virt := r.Virt.String()
	if r.VirtNote != "" {
		virt += " (" + r.VirtNote + ")"
	}
	fmt.Fprintf(&sb, "  Virtualiz. : %s\n", virt)
	return sb.String()
}
