package linuxenv

import (
	"os"
	"os/exec"
	"strings"
)

// Provisioned state lives in a named volume rather than a host bind mount: an
// extracted Alpine rootfs needs Linux filesystem semantics (symlinks, device
// nodes, exec bits) that an NTFS or APFS bind mount does not preserve.
const (
	dockerVolume = "mini-container-data"
	dockerImage  = "alpine:3.20"
)

// dockerEnv runs each command in a throwaway container sharing one volume.
type dockerEnv struct{ detail string }

func (d dockerEnv) Name() string   { return "docker" }
func (d dockerEnv) Detail() string { return d.detail }
func (d dockerEnv) Base() string   { return "/mc" }

func (d dockerEnv) args(extra []string, script string) []string {
	args := []string{"run", "--rm", "-i", "-v", dockerVolume + ":/mc", "-w", "/mc"}
	args = append(args, extra...)
	return append(args, dockerImage, "/bin/sh", "-c", script)
}

func (d dockerEnv) Output(script string) (string, error) {
	return trimmedOutput(exec.Command("docker", d.args(nil, script)...))
}

func (d dockerEnv) Run(script string) error {
	// The runtime writes to /sys/fs/cgroup and calls chroot/mount, so it needs
	// full capabilities and the host cgroup namespace.
	extra := []string{"--privileged", "--cgroupns=host"}
	if isTerminal(os.Stdin) {
		extra = append(extra, "-t")
	}
	cmd := exec.Command("docker", d.args(extra, script)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func (d dockerEnv) Push(hostFile, envPath string) error {
	f, err := os.Open(hostFile)
	if err != nil {
		return err
	}
	defer f.Close()

	cmd := exec.Command("docker", d.args(nil, "mkdir -p $(dirname "+envPath+") && cat > "+envPath)...)
	cmd.Stdin = f
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func probeDocker() Env {
	if !lookPath("docker") {
		return nil
	}
	out, err := exec.Command("docker", "info", "--format", "{{.OSType}}/{{.Architecture}}/{{.Name}}").Output()
	if err != nil {
		return nil // daemon not running, or unreachable
	}
	parts := strings.SplitN(strings.TrimSpace(string(out)), "/", 3)
	if len(parts) < 2 || parts[0] != "linux" {
		return nil // Windows containers cannot host a Linux runtime
	}
	detail := strings.Join(parts[1:], " ")
	return dockerEnv{detail: strings.TrimSpace(detail)}
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
