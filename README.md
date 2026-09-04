# mini-container 📦

<p align="left">
  <img src="https://img.shields.io/badge/Language-Go-00ADD8?style=flat-square&logo=go" alt="Go" />
  <img src="https://img.shields.io/badge/Platform-Linux-FCC624?style=flat-square&logo=linux&logoColor=black" alt="Linux" />
  <img src="https://img.shields.io/badge/Tech-Docker%20Internals-blue?style=flat-square" alt="Docker Internals" />
  <img src="https://img.shields.io/badge/License-MIT-green?style=flat-square" alt="License" />
</p>

A lightweight container runtime built from scratch in Go to understand how **Docker** works under the hood.

Docker is not magic; it is an abstraction over native Linux kernel isolation primitives. Unlike traditional virtual machines that run an entire guest operating system, containers are just isolated processes running directly on the host kernel. This project demystifies container runtimes by implementing process jailing, virtual filesystem scoping, and resource throttling using raw Linux system calls (`namespaces`, `cgroups v2`, and `chroot`).

---

### System Architecture & Pipeline

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