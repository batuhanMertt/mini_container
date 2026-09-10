package linuxenv

import (
	"os"
	"os/exec"
	"strings"
)

// wslEnv runs commands in a WSL2 distribution. WSL1 is rejected: it emulates
// syscalls rather than running a Linux kernel, so namespaces and cgroups v2 are
// not available there.
type wslEnv struct{ distro string }

func (w wslEnv) Name() string   { return "wsl" }
func (w wslEnv) Detail() string { return w.distro }
func (w wslEnv) Base() string   { return "$HOME/.mini-container" }

func (w wslEnv) args(script string) []string {
	// -u root: the runtime needs root for cgroups, chroot and mount, and WSL
	// grants root on the Windows host's authority. Going through sudo instead
	// would block on a password prompt on any distro that has one.
	// -e runs the command directly instead of through the login shell, which
	// keeps wsl.exe from re-parsing the script on the Windows side.
	return []string{"-d", w.distro, "-u", "root", "-e", "/bin/sh", "-c", script}
}

func (w wslEnv) Output(script string) (string, error) {
	return trimmedOutput(exec.Command("wsl.exe", w.args(script)...))
}

func (w wslEnv) Run(script string) error {
	cmd := exec.Command("wsl.exe", w.args(script)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func (w wslEnv) Push(hostFile, envPath string) error {
	f, err := os.Open(hostFile)
	if err != nil {
		return err
	}
	defer f.Close()

	cmd := exec.Command("wsl.exe", w.args("mkdir -p $(dirname "+envPath+") && cat > "+envPath)...)
	cmd.Stdin = f
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// internalDistros are management distributions owned by container tooling.
// They are not general-purpose Linux userspaces - no sudo, no user home, no
// package manager - so they are never valid launch targets even though WSL
// lists them, and Docker Desktop registers its own as the default distro.
var internalDistros = map[string]bool{
	"docker-desktop":         true,
	"docker-desktop-data":    true,
	"rancher-desktop":        true,
	"rancher-desktop-data":   true,
	"podman-machine-default": true,
}

func probeWSL() Env {
	if !lookPath("wsl.exe") {
		return nil
	}
	out, err := exec.Command("wsl.exe", "-l", "-v").Output()
	if err != nil {
		return nil
	}
	// WSL's own management output is UTF-16LE; distro names are ASCII, so
	// dropping the NUL padding is enough to read the table.
	text := strings.ReplaceAll(string(out), "\x00", "")

	var def, running, first string
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		isDefault := strings.HasPrefix(trimmed, "*")
		fields := strings.Fields(strings.TrimPrefix(trimmed, "*"))

		// Columns are NAME STATE VERSION; WSL1 lacks the kernel we need.
		if len(fields) < 3 || fields[2] != "2" {
			continue
		}
		name, state := fields[0], fields[1]
		if internalDistros[strings.ToLower(name)] {
			continue
		}

		if isDefault && def == "" {
			def = name
		}
		if strings.EqualFold(state, "Running") && running == "" {
			running = name
		}
		if first == "" {
			first = name
		}
	}

	// Prefer the user's default distro, then one that is already running.
	for _, name := range []string{def, running, first} {
		if name != "" {
			return wslEnv{distro: name}
		}
	}
	return nil
}
