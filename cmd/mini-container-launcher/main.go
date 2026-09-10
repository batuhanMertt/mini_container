// Command mini-container-launcher makes the Linux-only mini-container runtime
// usable from Windows and macOS.
//
// It inspects the host hardware, finds a reachable Linux environment (WSL2,
// Lima or Docker), works out which published Linux binary that environment
// actually needs, downloads it from GitHub Releases, provisions an Alpine
// rootfs next to it, and launches the container.
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/batuhanMertt/mini_container/internal/hwcheck"
	"github.com/batuhanMertt/mini_container/internal/linuxenv"
	"github.com/batuhanMertt/mini_container/internal/release"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

// The rootfs the runtime chroots into. Kept in step with the Makefile.
const (
	alpineBranch  = "v3.20"
	alpineVersion = "3.20.0"
)

// sudoPrefix defines $SUDO so assembled scripts elevate only when they are not
// already root. Written without double quotes so it survives Windows argument
// escaping on its way through wsl.exe.
const sudoPrefix = "if [ $(id -u) -eq 0 ]; then SUDO=; else SUDO=sudo; fi; "

func main() {
	fs := flag.NewFlagSet("mini-container", flag.ExitOnError)
	backend := fs.String("backend", "", "force a Linux backend: wsl, lima, docker or native")
	relTag := fs.String("release", os.Getenv("MINI_CONTAINER_RELEASE"), "release tag to use (default: latest)")
	localBin := fs.String("binary", "", "use this local Linux binary instead of downloading one")
	showVersion := fs.Bool("version", false, "print the launcher version and exit")
	fs.Usage = usage
	_ = fs.Parse(os.Args[1:])

	if *showVersion {
		fmt.Printf("mini-container launcher %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
		return
	}

	args := fs.Args()
	if len(args) == 0 {
		usage()
		os.Exit(1)
	}

	var err error
	switch args[0] {
	case "doctor":
		err = doctor(*backend)
	case "run":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "run: no command given, e.g. mini-container run /bin/sh")
			os.Exit(1)
		}
		err = run(*backend, *relTag, *localBin, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
		usage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "\nerror: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `mini-container launcher %s

usage:
  mini-container run <command> [args...]   analyse the host, then run the container
  mini-container doctor                    report hardware and backend readiness

flags:
  --backend <name>   force wsl, lima, docker or native
  --release <tag>    pin a release tag instead of latest
  --binary <path>    use a local Linux binary instead of downloading one
  --version          print the launcher version

`, version)
}

// analyse performs steps 1 and 2: host inspection and backend selection. It
// returns a nil Env together with an error when no backend is reachable.
func analyse(backend string) (hwcheck.Report, linuxenv.Env, string, error) {
	report := hwcheck.Detect(release.CacheDir())

	fmt.Println("[1/4] host analysis")
	fmt.Print(report)
	for _, w := range report.Warnings {
		fmt.Printf("  warning    : %s\n", w)
	}

	fmt.Println("\n[2/4] linux backend")
	env, err := linuxenv.Select(backend)
	if err != nil {
		fmt.Println("  backend    : none found")
		return report, nil, "", err
	}
	fmt.Printf("  backend    : %s (%s)\n", env.Name(), env.Detail())
	if user, uerr := env.Output("id -un"); uerr == nil && user != "" {
		fmt.Printf("  runs as    : %s\n", user)
	}

	// The backend, not the host, decides which binary is needed: an arm64 Mac
	// can run an amd64 Docker VM, and an amd64 host an emulated arm64 one.
	machine, err := env.Output("uname -m")
	if err != nil {
		return report, env, "", fmt.Errorf("querying %s architecture: %w", env.Name(), err)
	}
	arch, ok := hwcheck.NormalizeUname(machine)
	if !ok {
		return report, env, "", fmt.Errorf("%s reports unsupported architecture %q; only x86_64 and aarch64 are published",
			env.Name(), strings.TrimSpace(machine))
	}
	fmt.Printf("  kernel arch: %s -> %s\n", strings.TrimSpace(machine), arch)
	fmt.Printf("  selected   : %s\n", release.AssetName(arch))
	return report, env, arch, nil
}

func doctor(backend string) error {
	fmt.Printf("mini-container launcher %s\n\n", version)

	_, env, _, err := analyse(backend)
	if env == nil {
		fmt.Print(backendHelp())
		return nil // doctor reports readiness, it does not fail on the answer
	}
	if err != nil {
		return err
	}

	fmt.Println("\nready. run:  mini-container run /bin/sh")
	return nil
}

func run(backend, relTag, localBin string, cmdArgs []string) error {
	fmt.Printf("mini-container launcher %s\n\n", version)

	_, env, arch, err := analyse(backend)
	if err != nil {
		if env == nil {
			return fmt.Errorf("%w\n%s", err, backendHelp())
		}
		return err
	}

	fmt.Println("\n[3/4] runtime binary")
	binPath := localBin
	if binPath == "" {
		res, ferr := release.Fetch(arch, relTag, os.Stdout)
		if ferr != nil {
			return ferr
		}
		state := "downloaded"
		if res.Cached {
			state = "cached"
		}
		verified := "unverified"
		if res.Verified {
			verified = "sha256 verified"
		}
		fmt.Printf("  release    : %s\n", res.Tag)
		fmt.Printf("  %-11s: %s (%s)\n", state, res.Path, verified)
		binPath = res.Path
	} else {
		fmt.Printf("  local      : %s\n", binPath)
	}

	fmt.Printf("\n[4/4] provisioning %s (%s)\n", env.Name(), env.Detail())
	if err := provision(env, binPath, arch); err != nil {
		return err
	}

	fmt.Printf("\n--> launching: %s\n\n", strings.Join(cmdArgs, " "))
	return launch(env, cmdArgs)
}

func provision(env linuxenv.Env, hostBinary, arch string) error {
	base := env.Base()

	if _, err := env.Output("mkdir -p " + base + "/assets/rootfs"); err != nil {
		return fmt.Errorf("preparing %s: %w", base, err)
	}
	if err := env.Push(hostBinary, base+"/mini-container"); err != nil {
		return fmt.Errorf("copying the runtime into %s: %w", env.Name(), err)
	}
	if _, err := env.Output("chmod +x " + base + "/mini-container"); err != nil {
		return err
	}
	fmt.Printf("  runtime    : %s/mini-container\n", resolve(env, base))

	alpineArch, ok := hwcheck.AlpineArch(arch)
	if !ok {
		return fmt.Errorf("no Alpine rootfs is published for architecture %q", arch)
	}
	url := fmt.Sprintf("https://dl-cdn.alpinelinux.org/alpine/%s/releases/%s/alpine-minirootfs-%s-%s.tar.gz",
		alpineBranch, alpineArch, alpineVersion, alpineArch)

	// Extraction runs as root: the tarball carries ownership and device entries.
	script := sudoPrefix +
		"if [ -x " + base + "/assets/rootfs/bin/busybox ]; then echo present; exit 0; fi; " +
		"if command -v curl >/dev/null 2>&1; then " +
		"curl -fsSL " + url + " | $SUDO tar -xz -C " + base + "/assets/rootfs; " +
		"elif command -v wget >/dev/null 2>&1; then " +
		"wget -qO- " + url + " | $SUDO tar -xz -C " + base + "/assets/rootfs; " +
		"else echo 'neither curl nor wget is installed in this Linux environment' >&2; exit 1; fi; " +
		"echo extracted"

	out, err := env.Output(script)
	if err != nil {
		return fmt.Errorf("provisioning the Alpine rootfs: %w", err)
	}
	state := "downloaded and extracted"
	if strings.Contains(out, "present") {
		state = "already present"
	}
	fmt.Printf("  rootfs     : alpine-minirootfs-%s-%s (%s)\n", alpineVersion, alpineArch, state)
	return nil
}

func launch(env linuxenv.Env, cmdArgs []string) error {
	base := env.Base()

	quoted := make([]string, len(cmdArgs))
	for i, a := range cmdArgs {
		quoted[i] = shQuote(a)
	}

	script := sudoPrefix + "cd " + base + " && " +
		"$SUDO env MINI_CONTAINER_ROOTFS=" + base + "/assets/rootfs " +
		"./mini-container run " + strings.Join(quoted, " ")

	return env.Run(script)
}

// resolve expands a shell expression such as $HOME/.mini-container so the
// report can show a real path. The scripts keep using the unexpanded form,
// which needs no quoting even when the path contains spaces.
func resolve(env linuxenv.Env, expr string) string {
	if out, err := env.Output("printf %s " + expr); err == nil && out != "" {
		return out
	}
	return expr
}

// shQuote wraps a value for /bin/sh. Single quotes also survive Windows
// argument escaping on the way through wsl.exe.
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func backendHelp() string {
	switch runtime.GOOS {
	case "windows":
		return `
mini-container needs a Linux kernel: it builds on namespaces, cgroups v2 and
chroot, none of which exist on Windows. Install one of these, then re-run:

  WSL2 (recommended)   wsl --install
                       reboot, then finish the distro's first-run setup
  Docker Desktop       https://docs.docker.com/desktop/install/windows-install/
                       make sure it is switched to Linux containers

Check readiness with:  mini-container doctor
`
	case "darwin":
		return `
mini-container needs a Linux kernel: it builds on namespaces, cgroups v2 and
chroot, which macOS does not provide. Install one of these, then re-run:

  Lima (recommended)   brew install lima && limactl start default
  Colima               brew install colima && colima start
  Docker Desktop       https://docs.docker.com/desktop/install/mac-install/
  OrbStack             https://orbstack.dev

Check readiness with:  mini-container doctor
`
	default:
		return `
No Linux environment was reachable. On Linux the launcher runs the runtime
directly, so this usually means /bin/sh is missing or this is not a Linux host.

Check readiness with:  mini-container doctor
`
	}
}
