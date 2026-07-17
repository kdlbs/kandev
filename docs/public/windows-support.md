---
title: "Windows Support"
description: "Run Kandev on Windows through native and WSL workflows."
---

# Windows Support

Kandev has native Windows x64 desktop and npm/npx releases. WSL 2 remains useful when repositories, agents, or setup scripts assume a Linux environment. Choose one execution environment per Kandev installation; a native process and a WSL process use different filesystems, tools, Docker endpoints, and credential stores even when they can reach each other over localhost.

| Path | Best for | Main limitations |
|---|---|---|
| [Desktop app](./desktop-app.md) | Normal interactive use, native menus/notifications/updates | Windows x64 and WebView2; no service/background mode |
| [npm/npx CLI](./cli.md) | Browser UI, terminal operation, headless use | Node.js with npm 7+ is needed for the package shim |
| WSL 2 CLI | Linux-only agent tools and shell scripts | Browser launch and Windows/WSL path interop are dependency-bound |
| Native source checkout | Contributors testing Windows-specific code | Manual toolchain and curated Windows test subset; not the simplest product install |

## Native desktop installation

The recommended Windows product path is the x64 NSIS installer from [GitHub Releases](https://github.com/kdlbs/kandev/releases):

1. Download the file whose name begins `kandev-desktop-windows-x64-` and ends in `.exe`, plus its `.sha256` file.
2. Verify its SHA-256 value as shown in [Desktop App](./desktop-app.md).
3. Run the installer and launch Kandev from the Start menu.

The desktop app bundles Kandev and `agentctl`; Node.js is not required. It uses Microsoft WebView2 and binds its owned backend to `127.0.0.1`. Current Windows normally includes WebView2; repair or install the runtime if the app cannot create its window.

Windows ARM64 does not have a native Kandev artifact. Windows may emulate the x64 build on some ARM devices, but that combination is not a release/test target.

## Native CLI installation

Prerequisites:

- Windows x64;
- Node.js with npm 7 or later; and
- Git for repository and worktree operations.

From PowerShell:

```powershell
npm --version
npm install -g kandev@latest
kandev --version
kandev
```

Or use a non-global npm execution:

```powershell
npx -y kandev@latest
```

The npm shim selects the exact `@kdlbs/runtime-win32-x64` optional dependency and starts `kandev.exe`. Do not install with `--omit=optional`, copy only the executable, or mix shim/runtime versions.

The CLI prints a localhost URL and opens it through `cmd.exe`. It otherwise behaves like the macOS/Linux CLI, including automatic port fallback, foreground supervision, and `Ctrl+C` shutdown. To keep it local-only, override the backend's cross-platform default listen host:

```powershell
$env:KANDEV_SERVER_HOST = '127.0.0.1'
kandev
```

Without that override, the backend default is `0.0.0.0`; Windows Firewall may prompt for network access. Do not allow public/private-network exposure unless you deliberately built an authenticated network boundary. See [Configuration](./configuration.md).

## Windows paths and command discovery

The default Kandev home is `.kandev` below the Windows user profile, normally:

```text
%USERPROFILE%\.kandev
```

Set a dedicated native Windows path before launch to relocate or isolate it:

```powershell
$env:KANDEV_HOME_DIR = 'D:\KandevData'
kandev
```

Do not point native Kandev and WSL Kandev at the same SQLite database or task/worktree directory. Path formats, file locking, permissions, executable formats, and symlink semantics differ.

The native CLI inherits the terminal's `PATH`. Desktop also appends the user's roaming npm directory and Scoop shims. If an agent command cannot be found:

```powershell
Get-Command claude -ErrorAction SilentlyContinue
Get-Command codex -ErrorAction SilentlyContinue
Get-Command opencode -ErrorAction SilentlyContinue
```

Set the agent profile command to the full executable path when necessary. The Windows process layer resolves `PATHEXT`, runs `.cmd`/`.bat` tools through `cmd.exe /c`, and uses ConPTY for interactive terminal sessions. Shell aliases and PowerShell functions are not standalone executables and cannot be used as profile commands unless wrapped in a script file.

## Git, worktrees, and scripts

Install Git for Windows and confirm it is available to the same account/environment that starts Kandev:

```powershell
git --version
```

Kandev supports native Git repositories and worktrees. Windows still imposes filesystem-specific constraints:

- repository setup, cleanup, dev, and executor prepare scripts must use syntax available to their selected Windows shell; POSIX shell snippets do not become PowerShell automatically;
- creating symbolic links often requires Windows Developer Mode or an elevated account;
- a repository `copyFiles` entry using `:symlink` can therefore warn/fail on Windows—use a normal copy unless live linkage is required and symlinks are enabled; and
- path-length errors remain dependent on Windows and Git configuration. Prefer short repository/Kandev-home roots; only enable Git's long-path support if allowed by local policy.

Kandev uses Windows Job Objects and process-tree termination for managed child cleanup. An abruptly killed terminal or externally launched child can still outlive a session; inspect Task Manager and executor resources before deleting a worktree.

See [Git operations](./git-operations.md) for branch/worktree lifecycle and [Executors](./executors.md) for prepare scripts and copied files.

## Docker Desktop

Native Windows configuration defaults to Docker's named pipe:

```text
npipe:////./pipe/docker_engine
```

Install Docker Desktop, select Linux containers when using Kandev's Linux agent images, start the daemon, and test it from the same account:

```powershell
docker version
docker info
```

The local Docker executor creates its client lazily, so Kandev can start even when Docker Desktop is stopped; the executor fails when first used and retries on a later attempt. Windows drive sharing, bind mounts, corporate Docker policies, and WSL-backed Docker Desktop are external dependencies. Do not expose an unauthenticated Docker TCP endpoint. See [Docker](./docker.md).

## Run in WSL 2

WSL is the better choice when the repository's commands and agent CLI are Linux-native.

### 1. Install WSL

Run from an elevated PowerShell window:

```powershell
wsl --install
```

Restart when prompted, complete the distribution's first-run setup, and confirm WSL 2:

```powershell
wsl --status
wsl --list --verbose
```

### 2. Install product prerequisites inside WSL

Install Git plus a Node.js/npm distribution that provides npm 7 or later. For an Ubuntu distribution, Git itself is:

```bash
sudo apt update
sudo apt install git
npm --version
```

Node installation is distribution/version-manager specific; use the method your organization supports. The published Kandev runtime is prebuilt, so Go, pnpm, a C compiler, and `make` are **not** prerequisites for the normal npm/npx product path.

### 3. Start Kandev inside WSL

```bash
npx -y kandev@latest --headless
```

Open the printed `http://localhost:<port>` URL in a Windows browser. Windows-to-WSL localhost forwarding normally makes this work, but VPNs, mirrored-network settings, firewall policy, or an older WSL configuration can change that behavior. The native Linux launcher uses `xdg-open`; automatic Windows browser launch from WSL is not guaranteed, so `--headless` plus manual opening is the predictable path.

Keep Linux worktrees in the WSL filesystem (for example under `~/src` or the default `~/.kandev`) when Linux permissions, symlinks, and filesystem performance matter. Repositories under `/mnt/c` inherit Windows filesystem behavior.

### WSL and Docker

When Docker Desktop WSL integration is enabled for the distribution, verify `docker info` inside WSL. Otherwise install/configure a Linux Docker daemon inside WSL according to your platform policy. Native Windows named-pipe settings do not apply inside WSL; its default endpoint is the Unix socket.

## Remote executor limitation

The release bundle carries remote `agentctl` helpers for Linux `amd64`/`arm64` and macOS `amd64`/`arm64`. It does not carry a Windows remote helper. Consequently, a Windows desktop/CLI host can control supported Linux/macOS SSH targets, but a Windows machine is not a supported destination for the SSH executor. Remote Docker behavior depends on the target daemon and configured Linux container image; it does not add a Windows `agentctl` remote target.

## Services and background operation

`kandev service` supports systemd on Linux and launchd on macOS only. Native Windows Service Control Manager integration is not implemented. For interactive Windows use, choose Desktop or keep the CLI process in a terminal. Do not wrap it in an arbitrary service manager without designing user profile, `PATH`, network, process-tree, update, and data permissions explicitly.

WSL systemd availability depends on the distribution and WSL configuration. Even when systemd works inside WSL, its service lifecycle is tied to the WSL VM and is not the same as a native Windows service. Treat this as deployment-specific and follow [Run as a service](./run-as-a-service.md) only after verifying systemd in that distribution.

## Build from source on Windows

Native source development is supported as a contributor path but requires more than the product release. The checkout currently pins Node.js 24, pnpm 9.15.9, and Go 1.26.0 in `mise.toml`; the desktop build additionally requires Rust/Tauri dependencies, and SQLite builds require a Windows C toolchain. GNU Make can be installed with:

```powershell
winget install ezwinports.make
```

The repository bootstrap script is Bash/package-manager oriented and does not provision a native Windows toolchain automatically. Use Git Bash or another compatible shell for Unix-oriented root recipes, install the pinned tools manually, and follow the contributor guide. `make dev` builds a `winjob.exe` helper so `Ctrl+C` can close a native development process tree.

Backend Windows CI runs `go build ./...`, `go vet ./...`, and focused race tests for Windows-sensitive process and agent-launcher packages. It does not run every backend package or the full product E2E suite. The repository's broader Windows-clean target adds web and CLI unit tests:

```powershell
make test-windows
```

for the repository's Windows-clean subset. Linux-only release/build recipes are not evidence of native Windows support.

## Troubleshooting

### `No Kandev runtime package found for win32-x64`

Check npm and reinstall with optional dependencies:

```powershell
npm --version
npm install -g kandev@latest
kandev --version
```

npm must be 7 or later, and configuration must not omit optional dependencies. A native ARM64 Node process resolves as `win32-arm64`, for which no package exists; use an x64 Node/runtime environment only as an explicitly accepted emulation path.

### PowerShell blocks an npm-generated script

npm may create both PowerShell and `.cmd` shims. Use the organization's approved execution policy, or invoke the generated command shim explicitly:

```powershell
kandev.cmd --version
npx.cmd -y kandev@latest
```

Do not weaken machine-wide execution policy solely for Kandev without administrator approval.

### Browser or backend cannot be reached

Run headless and use the exact printed port:

```powershell
kandev --headless --verbose
```

Check Windows Firewall, `KANDEV_SERVER_HOST`, port conflicts, VPN/proxy policy, and whether the process is native Windows or WSL. An explicit occupied `--port` fails; omit it to allow fallback.

### Agent command is not found or exits immediately

Use `Get-Command`, then configure the full `.exe`, `.cmd`, or `.bat` path. Confirm credentials are available to the Desktop/CLI process rather than only an unrelated shell session. Rewrite POSIX prepare/setup commands for Windows or run the task under WSL/a Linux executor.

### Symlink or permission failure

Use normal copy-file entries instead of `:symlink`, or enable Developer Mode according to organizational policy and recreate the task/worktree. Also confirm antivirus/EDR has not quarantined the runtime or denied writes below the Kandev home.

### WSL URL does not open in Windows

Keep Kandev running with `--headless`, copy the printed URL, and open it manually. If Windows cannot connect, verify `wsl --status`, localhost forwarding/networking mode, firewall/VPN policy, and the backend listen address. Restarting the WSL VM with `wsl --shutdown` is safe only after stopping Kandev and active tasks inside WSL.
