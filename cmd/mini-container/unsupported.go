//go:build !linux

package main

import (
	"fmt"
	"os"
	"runtime"
)

// The runtime depends on Linux-only kernel primitives (CLONE_NEW* namespaces,
// cgroups v2, chroot + mount of procfs). There is no meaningful equivalent on
// other kernels, so this stub keeps `go build ./...` honest instead of
// pretending the feature exists.
func main() {
	fmt.Fprintf(os.Stderr,
		"mini-container runtime requires Linux (this build is %s/%s).\n"+
			"Use mini-container-launcher instead: it runs the Linux binary inside WSL2, Lima or Docker.\n",
		runtime.GOOS, runtime.GOARCH)
	os.Exit(1)
}
