package linuxenv

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// nativeEnv is the host itself, used when the launcher runs on Linux.
type nativeEnv struct{}

func (nativeEnv) Name() string   { return "native" }
func (nativeEnv) Detail() string { return "host kernel" }
func (nativeEnv) Base() string   { return "$HOME/.mini-container" }

func (nativeEnv) Output(script string) (string, error) {
	return trimmedOutput(exec.Command("/bin/sh", "-c", script))
}

func (nativeEnv) Run(script string) error {
	cmd := exec.Command("/bin/sh", "-c", script)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func (nativeEnv) Push(hostFile, envPath string) error {
	// envPath is a shell expression ($HOME/...), so let the shell resolve it.
	resolved, err := nativeEnv{}.Output("printf %s " + envPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return err
	}
	src, err := os.Open(hostFile)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.OpenFile(resolved, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

func probeNative() Env {
	if runtime.GOOS != "linux" {
		return nil
	}
	return nativeEnv{}
}
