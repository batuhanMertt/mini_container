package linuxenv

import (
	"os"
	"os/exec"
	"strings"
)

// limaEnv runs commands in a running Lima VM. Colima registers itself as a Lima
// instance, so it is picked up here too.
type limaEnv struct{ instance string }

func (l limaEnv) Name() string   { return "lima" }
func (l limaEnv) Detail() string { return l.instance }
func (l limaEnv) Base() string   { return "$HOME/.mini-container" }

func (l limaEnv) args(script string) []string {
	return []string{"shell", l.instance, "/bin/sh", "-c", script}
}

func (l limaEnv) Output(script string) (string, error) {
	return trimmedOutput(exec.Command("limactl", l.args(script)...))
}

func (l limaEnv) Run(script string) error {
	cmd := exec.Command("limactl", l.args(script)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func (l limaEnv) Push(hostFile, envPath string) error {
	f, err := os.Open(hostFile)
	if err != nil {
		return err
	}
	defer f.Close()

	cmd := exec.Command("limactl", l.args("mkdir -p $(dirname "+envPath+") && cat > "+envPath)...)
	cmd.Stdin = f
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func probeLima() Env {
	if !lookPath("limactl") {
		return nil
	}
	out, err := exec.Command("limactl", "list", "--format", "{{.Name}} {{.Status}}").Output()
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.EqualFold(fields[1], "Running") {
			return limaEnv{instance: fields[0]}
		}
	}
	return nil
}
