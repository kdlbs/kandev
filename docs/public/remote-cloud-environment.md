---
title: "Remote Development Environment"
description: "Contributor setup for developing and testing Kandev in an ephemeral remote VM or cloud workspace."
---

# Remote Development Environment

This is a contributor guide for cloning, building, testing, and running the Kandev source tree inside an ephemeral remote VM or cloud workspace, such as a codespace or provider sandbox. It does **not** describe an end-user Kandev deployment. The setup is provider-agnostic: Kandev does not create, suspend, secure, or delete the development VM.

To operate the Kandev control plane on a remote host, use [Run as a service](run-as-a-service.md), [Docker](docker.md), or the experimental [Kubernetes guidance](k8s.md). To keep the control plane elsewhere and run agent tasks remotely, configure a Sprites or SSH [executor profile](executors.md). Those product paths do not use this guide's source bootstrap, development ports, `.kandev-dev` state, or test tooling.

## Before you start

You need:

- a Linux or macOS workspace with enough access to install build packages, or those packages preinstalled;
- Git credentials capable of cloning the repository and any private test repositories;
- outbound HTTPS access to the tool registries used by mise, Go, pnpm, and Playwright;
- a provider-private port-forwarding mechanism for the Kandev backend;
- optional Docker daemon access only if you intend to test Docker executors or container-based suites.

The shared bootstrap supports `apt-get`, `dnf`, `apk`, and Homebrew on a best-effort basis. Its native build prerequisites include a C/C++ compiler, `pkg-config`, and SQLite development headers because the backend uses CGO and SQLite FTS5. If the workspace has neither root nor `sudo`, arrange those OS packages in the base image first.

> **Trust boundary:** bootstrap installs mise from `https://mise.run` when missing, trusts this checkout's `mise.toml`, downloads its pinned tools, installs pnpm dependencies, and may install Git hooks. Review and trust the checkout before running it.

## Clone and bootstrap

Provider startup hooks should authenticate and clone first, then invoke the repository-owned script from the repository root:

```bash
git clone https://github.com/kdlbs/kandev.git
cd kandev
scripts/bootstrap-dev-env --provider my-cloud-vm
```

`--provider` changes log labels only; it does not configure a provider. The script:

1. attempts to install common OS build packages;
2. installs mise if needed and adds bash activation to `~/.bashrc`;
3. trusts `mise.toml` and runs `mise install`;
4. runs `pnpm install --frozen-lockfile` in `apps/`;
5. runs `make doctor`, which wires pre-commit and commit-message hooks when `pre-commit` is available.

The current `mise.toml` pins these direct tools:

| Tool | Version |
| --- | --- |
| Node.js | 24 |
| pnpm | 9.15.9 |
| Go | 1.26.0 |
| jq | 1.8.1 |
| ripgrep | 15.1.0 |
| uv | 0.11.21 |
| golangci-lint | 2.9.0 |
| git-cliff | 2.13.1 |
| go-licenses | 1.6.0 |
| pre-commit | 4.6.0 |

Use `mise.toml` as the source of truth; this table describes the current checkout and will change with it.

For a workspace that will run browser E2E tests, include Playwright setup:

```bash
scripts/bootstrap-dev-env --provider my-cloud-vm --with-e2e
```

That path calls GNU `timeout`. It is normally available from coreutils on Linux, but stock macOS does not provide it and the bootstrap's Homebrew package list does not install it. Prepare it before using `--with-e2e` on macOS:

```bash
brew install coreutils
export PATH="$(brew --prefix coreutils)/libexec/gnubin:$PATH"
```

Available opt-outs are:

```text
--without-hooks
--without-node-modules
--without-os-packages
--without-mise-install
```

`--without-mise-install` fails when mise is absent. The other switches assume you have provided the skipped layer separately.

The script activates mise only for its own bash process and persists bash setup. In another shell, activate the matching shell integration, for example:

```bash
eval "$(mise activate bash)"
mise current
```

Use `mise activate zsh`, `fish`, or the appropriate shell form instead when applicable.

## Run development mode

```bash
make dev
```

`make dev` runs the repository doctor, builds remote `agentctl` helper binaries, then starts the TypeScript development launcher. The launcher starts:

- the Go backend, preferring port `38429`;
- the Vite development server, preferring port `37429`;
- standalone `agentctl`, preferring port `39429` when needed.

Occupied preferred ports are replaced with available random ports. Read the `[kandev] url:` line rather than assuming `38429`. Open or forward the **backend** URL: it proxies the Vite page and carries the same-origin HTTP and WebSocket traffic. The Vite port is internal development plumbing and normally does not need a provider public-port rule.

To request fixed preferred ports:

```bash
KANDEV_BACKEND_PORT=38429 KANDEV_WEB_PORT=37429 make dev
```

Startup still fails when an explicitly selected port is unavailable. Stop with `Ctrl-C`; the development launcher supervises the backend and web children and shuts them down together.

## Network and credential security

Both the development backend and Vite command bind to `0.0.0.0`. Development mode also enables debug/pprof endpoints, ACP frame logging, the mock agent alongside real agents, and the in-development Office feature. Kandev's web, HTTP, WebSocket, debug, and external MCP endpoints do not provide a user-authentication boundary.

Do not mark these ports public. Use a provider's private authenticated forward, SSH tunnel, or private VPN; forward only the backend port printed at startup. Provider “port visibility” and proxy authentication vary and are outside Kandev's implementation.

For an SSH tunnel from a machine that can reach the VM:

```bash
ssh -L 38429:127.0.0.1:38429 user@remote-host
```

Adjust the left and right port when the launcher selected another one. `KANDEV_SERVER_HOST=127.0.0.1 make dev` restricts the backend listener, but the current Vite package script still binds its own development port to all interfaces; retain host firewall/private-network controls.

Use the cloud provider's encrypted secret mechanism or short-lived CLI authentication for repository and agent credentials. Avoid placing tokens in the repository. The development processes inherit their shell environment, and agents or Git host CLIs may consume credentials from it. Treat `.kandev-dev`, terminal output, and captured test artifacts as potentially sensitive.

For GitHub PR paths, install and authenticate `gh`. Azure Repos PR creation additionally requires `az`, the `azure-devops` extension, and Azure authentication:

```bash
az extension add --name azure-devops
az login
# Headless alternative supported by Azure CLI tooling:
export AZURE_DEVOPS_EXT_PAT='...'
```

The Azure tooling is optional and only needed for Azure Repos operations. See [Git operations](git-operations.md#create-a-pull-request-or-merge-request).

## Development state and persistence

Normal `make dev` isolates state under `<checkout>/.kandev-dev/`, including its SQLite database, logs, task workspaces, and other Kandev home data. It does not use `~/.kandev` by default. The selected development profile uses embedded SQLite and the in-memory event bus, so PostgreSQL and NATS are not prerequisites.

When the checkout itself is a Kandev task workspace, the launcher deliberately clears a parent-provided `KANDEV_DATABASE_PATH` and uses the local `.kandev-dev` state. In a normal shell, an explicit `KANDEV_DATABASE_PATH` is honored. If that path is outside a `.kandev-dev` directory, the launcher treats it as production data, copies an existing database to `~/.kandev/data/backups/dev-prod-db-<timestamp>.db`, retains the newest five such snapshots, and aborts startup if the copy fails.

That automatic safeguard is a raw copy of the main `.db` file only. Kandev uses SQLite WAL mode, so the copy is not a transaction-consistent backup while another process is writing and it does not include the `-wal` or `-shm` files. Stop every writer before relying on that copy, or first create a consistent backup through **Settings > System > Backups**, which uses SQLite `VACUUM INTO`; see [SQLite backups](operations.md#sqlite-backups).

`make dev-prod-db` deliberately points development mode at `~/.kandev/data/kandev.db`. Use it only when testing against that database is intentional and after an independent backup.

To erase disposable development state after stopping `make dev`:

```bash
make clean-db
```

That target removes the entire `.kandev-dev` tree, not only the SQLite file. Any tasks, repositories, sessions, and debug logs below it are lost. Ephemeral-VM deletion likewise loses uncommitted changes and provider-local secrets unless the provider volume persists; push or export wanted work before destroying the workspace.

Development mode writes raw ACP frames under `.kandev-dev/logs/acp`. They can include full prompts, file content, and tool payloads. The built-in writer rotates and prunes them, but you should still exclude them from support bundles and delete the development home when no longer needed.

## Build and test commands

Run commands from the repository root after mise activation:

| Command | Current behavior |
| --- | --- |
| `make test` | Backend Go tests, web Vitest suite, CLI tests, and script tests |
| `make test-backend` | Backend tests only |
| `make test-web` | Web tests only |
| `make test-cli` | CLI tests only |
| `make lint` | Go, web, and harness linting |
| `make typecheck` | TypeScript checks across packages |
| `make fmt` | Rewrites backend and web files with project formatters |
| `make test-e2e` | Builds backend and web, then runs Playwright headlessly |
| `make test-e2e-headed` | Builds and runs Playwright with a visible browser |

`make test-e2e` already depends on the backend and web builds; no separate build command is required. Headed and UI modes need a display or provider display forwarding.

The web package's normal `dev`, `build`, `test`, and `typecheck` scripts generate release-note and changelog JSON through lifecycle hooks. The root `make typecheck` calls `tsc` directly and bypasses those package hooks. Generate the files first in a new checkout:

```bash
node apps/web/scripts/generate-release-notes.mjs
node apps/web/scripts/generate-changelog.mjs
make typecheck
```

Alternatively, a web-only package-script typecheck runs its generator hooks:

```bash
cd apps
pnpm --filter @kandev/web typecheck
```

## Playwright on constrained VMs

`scripts/bootstrap-dev-env --with-e2e` calls `scripts/install-playwright-browsers.sh`. That helper installs Chromium and the headless shell. It first asks Playwright to install system dependencies, attempts normal browser installation with a 90-second timeout, and then uses a manual unzip fallback for Firecracker-style environments where Playwright extraction hangs.

System dependency installation is best effort because it may require `sudo`. If the browser artifacts install but tests report missing shared libraries, add Playwright's Linux dependencies to the provider image or run, with appropriate privilege:

```bash
cd apps
pnpm --filter @kandev/web exec playwright install-deps chromium
```

The fallback can use only complete ZIP files already downloaded under `/tmp`. It currently prints completion and exits successfully even when it skipped a missing or corrupt archive, so verify that Playwright reports installed browser locations before treating bootstrap as complete:

```bash
cd apps
pnpm --filter @kandev/web exec playwright install --dry-run chromium chromium-headless-shell
```

Rerun the installer after correcting network, disk, or unzip failures.

## Executor limitations in cloud workspaces

Local and Worktree executors run inside the development VM and need their agent CLIs there. A Local Docker profile additionally needs a usable Docker API and privileges to build images and bind ports. Many hosted sandboxes do not expose a Docker daemon or prohibit nested containers; in that case, disable or avoid Docker profiles rather than mounting an untrusted daemon socket.

SSH and Sprites profiles still use their normal remote credentials and network paths. The fact that Kandev itself is already on a cloud VM does not convert a Local executor into a managed remote executor. See [Executors](executors.md#current-support).

## Troubleshooting

### `mise`, `node`, `go`, or `pnpm` is missing in a new shell

The bootstrap persists activation to `~/.bashrc` only. Start a new bash login shell or activate mise for the shell currently in use. Confirm the checkout is trusted with `mise trust` and inspect `mise current`.

### CGO or SQLite compilation fails

Review the warning from the OS-package phase. Install a compiler toolchain, `pkg-config`, and the platform SQLite development package (`libsqlite3-dev`, `sqlite-devel`, or Homebrew `sqlite`), then rerun bootstrap.

### The forwarded page does not load

Use the backend port from the latest startup banner, not the preferred port or the Vite port. Confirm locally inside the VM:

```bash
curl --fail http://127.0.0.1:38429/health
```

Substitute the logged port. Then check the provider's port visibility, proxy path/WebSocket support, firewall, and process logs. Do not solve the problem by making an unauthenticated development port public.

### The browser does not open automatically

Remote workspaces usually cannot open a browser on your local machine. The automatic open is convenience only; copy the logged backend URL into the provider's private port-forwarding UI or create an SSH tunnel.

### Agent or Docker checks fail while the UI works

The control plane can start without every optional executable. Install and authenticate the selected agent CLI in the VM. For Docker, verify `docker version` against the intended daemon and review cloud-provider restrictions. For SSH/Sprites, use the executor health checks and troubleshooting guidance in [Executors](executors.md#troubleshooting).

### E2E browser setup hangs or tests are killed

Use the repository Playwright installer rather than invoking `playwright install` directly on a Firecracker VM. Check free disk, shared memory, RAM, and required libraries. Reduce unrelated concurrent builds before retrying; resource limits and provider eviction are not Kandev test failures.
