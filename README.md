# mini-container 📦

<p align="left">
  <img src="https://img.shields.io/badge/Language-Go-00ADD8?style=flat-square&logo=go" alt="Go" />
  <img src="https://img.shields.io/badge/Runtime-Linux-FCC624?style=flat-square&logo=linux&logoColor=black" alt="Linux" />
  <img src="https://img.shields.io/badge/Launcher-Windows%20%7C%20macOS%20%7C%20Linux-informational?style=flat-square" alt="Launcher platforms" />
  <img src="https://img.shields.io/badge/Tech-Docker%20Internals-blue?style=flat-square" alt="Docker Internals" />
  <img src="https://img.shields.io/badge/License-MIT-green?style=flat-square" alt="License" />
</p>

A lightweight container runtime built from scratch in Go to understand how **Docker** works under the hood.

Docker is not magic; it is an abstraction over native Linux kernel isolation primitives. Unlike traditional virtual machines that run an entire guest operating system, containers are just isolated processes running directly on the host kernel. This project demystifies container runtimes by implementing process jailing, virtual filesystem scoping, and resource throttling using raw Linux system calls (`namespaces`, `cgroups v2`, and `chroot`).

---

## Download and run

Grab the file for your machine from the [latest release](../../releases/latest) and run it. No Go toolchain required.

| Your machine | Download |
| --- | --- |
| Windows (Intel/AMD) | `mini-container_windows_amd64.exe` |
| Windows (ARM) | `mini-container_windows_arm64.exe` |
| macOS (Apple Silicon) | `mini-container_darwin_arm64` |
| macOS (Intel) | `mini-container_darwin_amd64` |
| Linux (Intel/AMD) | `mini-container_linux_amd64` |
| Linux (ARM) | `mini-container_linux_arm64` |

```console
$ mini-container doctor          # check this machine first
$ mini-container run /bin/sh     # start a shell inside a container
```

Verify any download against `checksums.txt` from the same release.

### Why Windows and macOS get a launcher, not the runtime

The runtime is Linux-only and cannot be otherwise. It calls `CLONE_NEWUTS`/`CLONE_NEWPID`/`CLONE_NEWNS`, writes cgroups v2 files under `/sys/fs/cgroup`, and calls `chroot` and `mount("proc")`. None of these exist on the Windows or macOS kernel, so a native build is not merely unimplemented — there is nothing to implement it with. This is exactly why Docker Desktop ships a Linux VM.

So the Windows and macOS downloads are a **launcher**. It:

1. **Analyses the host** — CPU model and core count, total RAM, free disk on the cache volume, and whether hardware virtualization is available.
2. **Finds a Linux backend** — WSL2 on Windows, Lima/Colima on macOS, or Docker on either. Docker Desktop's internal `docker-desktop` WSL distro is skipped: it is a management image, not a usable Linux userspace.
3. **Picks the right binary** — by asking the *backend* for its architecture, not the host. An Apple Silicon Mac running an amd64 Docker VM needs `mini-container_linux_amd64`, and the host arch would be the wrong answer.
4. **Downloads and verifies it** from this repository's GitHub Releases, caching it under the user cache directory and checking its SHA-256 against `checksums.txt`.
5. **Provisions and launches** — pushes the runtime into the Linux environment, extracts an Alpine minirootfs beside it, and starts the container.

On Linux the same launcher skips all of that and runs the runtime directly.

```console
$ mini-container.exe run /bin/sh
mini-container launcher v0.1.0

[1/4] host analysis
  OS         : windows/amd64
  CPU        : Intel64 Family 6 Model 186 Stepping 2, GenuineIntel (12 cores)
  RAM        : 15.7 GiB
  Free disk  : 15.3 GiB
  Virtualiz. : yes (VT-x/AMD-V enabled in firmware)

[2/4] linux backend
  backend    : wsl (Ubuntu)
  runs as    : root
  kernel arch: x86_64 -> amd64
  selected   : mini-container_linux_amd64

[3/4] runtime binary
  release    : v0.1.0
  downloaded : C:\Users\you\AppData\Local\mini-container\v0.1.0\mini-container_linux_amd64 (sha256 verified)

[4/4] provisioning wsl (Ubuntu)
  runtime    : /root/.mini-container/mini-container
  rootfs     : alpine-minirootfs-3.20.0-x86_64 (downloaded and extracted)

--> launching: /bin/sh

/ # hostname
box
/ # cat /etc/os-release | head -1
NAME="Alpine Linux"
/ # ps -o pid,comm
PID   COMMAND
    1 exe
```

The host here is Ubuntu under WSL2, yet the container reports Alpine, the hostname `box`, and PID 1 — the chroot, UTS namespace and PID namespace are all real.

### If no backend is found

`mini-container doctor` prints what to install:

| Host | Install one of |
| --- | --- |
| Windows | `wsl --install` (recommended), or Docker Desktop set to Linux containers |
| macOS | `brew install lima && limactl start default`, Colima, Docker Desktop, or OrbStack |

**Requirements:** a 64-bit x86 or ARM machine with hardware virtualization enabled in firmware, ~1 GiB RAM and ~512 MiB free disk. WSL1 is rejected — it emulates syscalls instead of running a Linux kernel, so it has no namespaces or cgroups v2.

### Launcher flags

```
mini-container run <command> [args...]   analyse the host, then run the container
mini-container doctor                    report hardware and backend readiness

--backend <name>   force wsl, lima, docker or native
--release <tag>    pin a release tag instead of latest
--binary <path>    use a local Linux binary instead of downloading one
--version          print the launcher version
```

Set `GITHUB_TOKEN` if you hit the anonymous GitHub API rate limit.

---

## System Architecture & Pipeline

In Linux, an active process cannot dynamically re-assign its own running thread to PID 1. To work around this, container engines rely on the **Fork-Exec / Self Re-Execution** pattern:

```text
[Host Environment]
 └── mini-container run /bin/sh (PID: 14205)
      │
      ├── 1. Namespace Configuration:
      │       CLONE_NEWUTS | CLONE_NEWPID | CLONE_NEWNS
      │
      ├── 2. Fork/Exec -> /proc/self/exe child /bin/sh
      │       (Re-executes own binary within isolated boundaries)
      │
      ├── 3. Cgroups v2 Resource Accounting:
      │       Creates /sys/fs/cgroup/mini-container
      │       Sets hard memory limit: 100MB (memory.max)
      │       Attaches Child PID to cgroup.procs
      │
      └── [Isolated Container Boundary] (Child Process)
           │
           ├── sethostname("box")           -> Independent hostname (UTS)
           ├── chroot("./assets/rootfs")    -> Jailed to minimal Alpine VFS root
           ├── chdir("/")                   -> Working directory set to new root
           ├── mount("proc", "/proc")       -> Process table scoped to container only
           │
           └── execvp("/bin/sh")            -> Container shell runs as PID 1
```

On Windows and macOS the launcher wraps this pipeline rather than changing it:

```text
[Windows / macOS host]
 └── mini-container.exe run /bin/sh
      │
      ├── hwcheck   -> CPU, RAM, disk, virtualization support
      ├── linuxenv  -> WSL2 | Lima | Docker   (backend reports its own arch)
      ├── release   -> download + SHA-256 verify mini-container_linux_<arch>
      │
      └── [Linux environment]
           └── the pipeline above, unchanged
```

---

## Layout

```
cmd/mini-container/             the Linux runtime (namespaces, cgroups, chroot)
cmd/mini-container-launcher/    the host-side launcher for Windows/macOS/Linux
internal/hwcheck/               CPU, RAM, disk and virtualization probes per OS
internal/linuxenv/              WSL2 / Lima / Docker / native backends
internal/release/               GitHub release resolution, download, checksums
.github/workflows/              CI (cross-compile all targets) and release
```

`cmd/mini-container` carries a `//go:build linux` tag; on other platforms a stub replaces it that explains the requirement and exits, so `go build ./...` stays honest everywhere.

## Building from source

```bash
make check      # gofmt, go vet, cross-compile every release target
make build      # Linux runtime for this machine
make launcher   # host launcher for this machine
make dist       # every published release asset, into dist/ with checksums
```

Running the runtime directly on Linux:

```bash
make setup      # download the Alpine rootfs into assets/
make run        # sudo ./mini-container run /bin/sh
```

The rootfs location defaults to `./assets/rootfs` and can be overridden with `MINI_CONTAINER_ROOTFS`, which is how the launcher points the runtime at its provisioned copy.

## Releasing

Pushing a version tag builds and publishes every asset:

```bash
git tag v0.1.0 && git push origin v0.1.0
```

The workflow cross-compiles the runtime for `linux/{amd64,arm64}` and the launcher for `windows`, `darwin` and `linux` on both architectures, writes `checksums.txt`, and creates the GitHub release the launcher downloads from.
