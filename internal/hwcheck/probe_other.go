//go:build !windows && !darwin && !linux

package hwcheck

func cpuModel() string              { return "" }
func totalRAM() uint64              { return 0 }
func freeDisk(dir string) uint64    { return 0 }
func virtualization() (Tri, string) { return Unknown, "unsupported platform" }
