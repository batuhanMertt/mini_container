package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

const defaultMemoryLimit = 100 * 1024 * 1024 // 100MB

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: mini-container run <command> [args...]")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		parent()
	case "child":
		child()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func parent() {
	cmd := exec.Command("/proc/self/exe", append([]string{"child"}, os.Args[2:]...)...)
		os.Exit(1)
	}
}

func parent() {
	cmd := exec.Command("/proc/self/exe", append([]string{"child"}, os.Args[2:]...)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS,
	}

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start child process: %v\n", err)
		os.Exit(1)
	}

	if err := applyCgroup(cmd.Process.Pid); err != nil {
		fmt.Fprintf(os.Stderr, "cgroup error: %v\n", err)
	}

	if err := cmd.Wait(); err != nil {
		os.Exit(1)
	}
}

func applyCgroup(pid int) error {
	cgPath := "/sys/fs/cgroup/mini-container"
	if err := os.MkdirAll(cgPath, 0755); err != nil {
		return err
	}

	maxMemFile := filepath.Join(cgPath, "memory.max")
	procsFile := filepath.Join(cgPath, "cgroup.procs")

	if err := os.WriteFile(maxMemFile, []byte(strconv.Itoa(defaultMemoryLimit)), 0644); err != nil {
		return err
	}
	return os.WriteFile(procsFile, []byte(strconv.Itoa(pid)), 0644)
}

func child() {
	if err := syscall.Sethostname([]byte("box")); err != nil {
		fmt.Fprintf(os.Stderr, "sethostname failed: %v\n", err)
		os.Exit(1)
	}

	rootfs := "./assets/rootfs"
	if err := syscall.Chroot(rootfs); err != nil {
		fmt.Fprintf(os.Stderr, "chroot error: %v (run 'make setup' first)\n", err)
		os.Exit(1)
	}

	if err := syscall.Chdir("/"); err != nil {
		os.Exit(1)
	}

	_ = os.MkdirAll("/proc", 0755)
	if err := syscall.Mount("proc", "/proc", "proc", 0, ""); err != nil {
		fmt.Fprintf(os.Stderr, "failed to mount proc: %v\n", err)
		os.Exit(1)
	}
	defer syscall.Unmount("/proc", 0)

	cmd := exec.Command(os.Args[2], os.Args[3:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		os.Exit(1)
	}
}
