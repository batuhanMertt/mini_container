// Package linuxenv abstracts over the ways a Linux userspace can be reached
// from the host: natively, through WSL2, through a Lima VM, or through Docker.
//
// Every backend reduces to three operations - run a shell script, capture its
// output, and push a file in - so provisioning and launching are written once.
package linuxenv

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// Env is a reachable Linux userspace.
type Env interface {
	// Name is the stable backend identifier, e.g. "wsl".
	Name() string
	// Detail describes the concrete instance, e.g. "Ubuntu-22.04".
	Detail() string
	// Base is the directory (as a shell expression) mini-container lives in.
	Base() string
	// Output runs script under /bin/sh and returns its stdout.
	Output(script string) (string, error)
	// Run runs script under /bin/sh with the host's stdio attached.
	Run(script string) error
	// Push copies a host file to envPath inside the environment.
	Push(hostFile, envPath string) error
}

// ErrNoBackend is returned when no Linux environment is reachable.
var ErrNoBackend = errors.New("no Linux environment available")

// Detect returns every reachable backend, best first.
func Detect() []Env {
	var envs []Env
	for _, probe := range probeOrder() {
		if e := probe(); e != nil {
			envs = append(envs, e)
		}
	}
	return envs
}

// Select returns the named backend, or the best available one when name is "".
func Select(name string) (Env, error) {
	envs := Detect()
	if name == "" {
		if len(envs) == 0 {
			return nil, ErrNoBackend
		}
		return envs[0], nil
	}
	for _, e := range envs {
		if e.Name() == name {
			return e, nil
		}
	}
	return nil, fmt.Errorf("backend %q is not available on this host", name)
}

func probeOrder() []func() Env {
	switch runtime.GOOS {
	case "linux":
		return []func() Env{probeNative, probeDocker}
	case "windows":
		return []func() Env{probeWSL, probeDocker}
	case "darwin":
		return []func() Env{probeLima, probeDocker}
	}
	return []func() Env{probeDocker}
}

func lookPath(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}

func trimmedOutput(cmd *exec.Cmd) (string, error) {
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("%s: %s", err, strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
