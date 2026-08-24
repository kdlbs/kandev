---
status: draft
system: platform
created: 2026-08-07
owners:
  - kandev
---
# Go dev launcher and minimal Node surface Requirements

## Overview

`make dev` is the last Kandev entrypoint that runs through the TypeScript launcher. `make start` (Makefile:277) and every `make service-*` target already exec the native Go binary at `apps/backend/bin/kandev`; only `make dev` still shells out to `pnpm -C cli dev -- dev` (Makefile:195), which runs `tsx src/cli.ts`.

## Requirements

### REQ-PLATFORM-GO-DEV-LAUNCHER-001: Go dev launcher and minimal Node surface

**Intent:** `make dev` is the last Kandev entrypoint that runs through the TypeScript launcher. `make start` (Makefile:277) and every `make service-*` target already exec the native Go binary at `apps/backend/bin/kandev`; only `make dev` still shells out to `pnpm -C cli dev -- dev` (Makefile:195), which runs `tsx src/cli.ts`.

#### Acceptance criteria

- **AC-PLATFORM-GO-DEV-LAUNCHER-001.1:** Inside a Kandev task workspace — signalled by `KANDEV_TASK_ID` being set, or by the repo root living under `~/.kandev/tasks/` — any inherited `KANDEV_DATABASE_PATH` is assumed to be leaked from the parent backend and is cleared, so the backend derives its DB from `KANDEV_HOME_DIR`. The launcher prints that it detected a task workspace.
- **AC-PLATFORM-GO-DEV-LAUNCHER-001.2:** Otherwise an explicit `KANDEV_DATABASE_PATH` is honored as an escape hatch.
- **AC-PLATFORM-GO-DEV-LAUNCHER-001.3:** Otherwise the DB is `<repoRoot>/.kandev-dev/data/kandev.db`.
- **AC-PLATFORM-GO-DEV-LAUNCHER-001.4:** **GIVEN** a host with non-loopback LAN and Tailscale IPv4 addresses, **WHEN** the `dev`, `start`, or `run` launcher prints its startup banner, **THEN** it prints one `network:` URL for each address using the backend port.
- **AC-PLATFORM-GO-DEV-LAUNCHER-001.5:** **GIVEN** a host with duplicate addresses, loopback addresses, or link-local addresses, **WHEN** the startup banner is printed, **THEN** duplicate, loopback, and link-local addresses are omitted.
- **AC-PLATFORM-GO-DEV-LAUNCHER-001.6:** **GIVEN** a host with an IPv6 address, **WHEN** the startup banner is printed, **THEN** the address is rendered as `http://[<address>]:<port>`.
- **AC-PLATFORM-GO-DEV-LAUNCHER-001.7:** **GIVEN** interface enumeration returns an error, **WHEN** the launcher starts, **THEN** it continues startup and prints the localhost URL without a `network:` line.
- **AC-PLATFORM-GO-DEV-LAUNCHER-001.8:** **GIVEN** `KANDEV_SERVER_HOST` is a loopback or specific bind address, **WHEN** the startup banner is printed, **THEN** it omits unrelated network addresses.

## Migrated source detail

## Why

`make dev` is the last Kandev entrypoint that runs through the TypeScript launcher.
`make start` (Makefile:277) and every `make service-*` target already exec the native Go
binary at `apps/backend/bin/kandev`; only `make dev` still shells out to
`pnpm -C cli dev -- dev` (Makefile:195), which runs `tsx src/cli.ts`.

That single call site keeps roughly 3,000 lines of TypeScript alive under
`apps/cli/src/**` — `dev.ts`, plus `run.ts`, `start.ts`, `service/`, and `supervisor/`,
which are already dead code because their Go equivalents (`launcher.runInstalled`,
`launcher.runStart`, `launcher.runService`, `launcher/supervisor.go`) shipped and took
over the Makefile targets. None of that TypeScript is published: `apps/cli/package.json`
declares `files: ["bin/cli.js", "bin/native-shim.js"]`, so the npm `kandev` package is
already a four-line Node shim that execs the Go binary. The `src/` tree exists solely to
serve `make dev`.

The cost of keeping it is not just lines. The dev launch chain today is
`bash → make → pnpm → node → make → sh → kandev`, and that chain is why
`apps/backend/cmd/winjob` exists at all: MSYS bash, native Win32 processes, and Node
disagree on signal propagation, so Ctrl-C leaks processes on Windows unless the backend
is wrapped in a Job Object (`apps/cli/src/dev.ts:169-199`). The Go launcher already
solves this generically with `CREATE_NEW_PROCESS_GROUP` plus `taskkill /F /T`
(`launcher/process_group_windows.go`), so moving `dev` to Go removes the winjob
dependency from the dev path rather than reimplementing it.

Behavior also drifts between the two implementations. PR #2394 ("harden launcher port
ownership") had to land the same health-token and explicit-port-preflight fix twice —
once in `apps/cli/src/{args,health,ports,run,shared,dev}.ts` and once in
`apps/backend/internal/launcher/{env,health,ports,run,start}.go`. Every launcher change
pays that double-implementation tax until `dev` moves.

## What

### `kandev dev` becomes a native launcher command

The Go launcher gains a third command alongside `run` and `start`. It accepts the same
surface the TypeScript `dev` accepted:

| Input | Behavior |
| --- | --- |
| `dev`, `--dev` | Select dev mode. |
| `--port`, `--backend-port`, `KANDEV_BACKEND_PORT`, `KANDEV_PORT` | Backend port. Existing preflight applies: an explicitly requested occupied port is a hard error naming the port and its source. |
| `--web-internal-port`, `KANDEV_WEB_PORT` | Internal Vite port. Rejected outside dev mode. |
| `--web-port` | Rejected with the existing "use `--web-internal-port` for dev mode" message. |
| `--verbose`, `--debug`, `--headless`, `--no-browser` | Unchanged meaning. |

Omitted ports keep the current preferred-then-random selection (`38429` backend,
`37429` web, `39429` agentctl), and the web and agentctl ports must not collide with the
backend port or with each other.

### Dev state stays isolated from production

Dev mode roots Kandev at `<repoRoot>/.kandev-dev` so `make dev` never mutates the user's
`~/.kandev` state and so `make clean-db` (which removes `.kandev-dev/`) matches what
`make dev` writes. The repo root is discovered by walking up from the working directory
for a parent containing both `apps/backend` and `apps/web`; failing to find one is a
usage error, not a silent fallback.

`KANDEV_DATABASE_PATH` resolution keeps its current three-way rule:

- Inside a Kandev task workspace — signalled by `KANDEV_TASK_ID` being set, or by the
  repo root living under `~/.kandev/tasks/` — any inherited `KANDEV_DATABASE_PATH` is
  assumed to be leaked from the parent backend and is cleared, so the backend derives
  its DB from `KANDEV_HOME_DIR`. The launcher prints that it detected a task workspace.
- Otherwise an explicit `KANDEV_DATABASE_PATH` is honored as an escape hatch.
- Otherwise the DB is `<repoRoot>/.kandev-dev/data/kandev.db`.

Dev mode always sets `KANDEV_DEBUG_DEV_MODE=true`, which is the profile selector; the
concrete dev defaults stay in `profiles.yaml`. The launcher must not restate them.

### Production databases are backed up before dev touches them

When the resolved DB path is not under a `.kandev-dev` segment, the launcher snapshots it
to `~/.kandev/data/backups/dev-prod-db-<timestamp>.db` before starting the backend,
retaining the five newest such snapshots and leaving other backup families
(`kandev-*`, `manual-*`) untouched. This is what makes `make dev-prod-db` safe.

A backup failure aborts startup with a non-zero exit. The guard exists precisely to
protect production state before dev mode touches it; continuing past a failed backup
would remove the guarantee that justified it.

### The Vite dev server is a supervised child

The launcher starts the frontend with `pnpm -C apps --filter @kandev/web dev`, passing
`PORT`, `HOSTNAME=127.0.0.1`, and `KANDEV_API_BASE_URL`, setting `KANDEV_DEBUG=true` and
`VITE_KANDEV_DEBUG=true`, and explicitly removing any inherited `VITE_KANDEV_API_PORT` so
a host `.env`, Docker env, or CI variable cannot reintroduce cross-origin browser API
calls. On Windows, `pnpm` is invoked through `cmd.exe /c` because Node-style `.cmd` shims
cannot be spawned directly.

Browser traffic still enters through the Go backend, which reverse-proxies to the Vite
port via `KANDEV_WEB_INTERNAL_URL`. The launcher waits for the backend health endpoint
(dev timeout: 600s, versus 45s for release) and then for the Vite URL to answer, then
opens the backend URL. The Vite readiness probe accepts any HTTP response and does not
require the launcher health token — Vite is not a Kandev process and does not emit one.

If either child exits, the launcher shuts the whole tree down and exits with the child's
code, matching current behavior.

### Startup output advertises network addresses

The startup banner for `dev`, `start`, and `run` prints the localhost backend URL and
then one `network:` URL for each unique non-loopback, non-link-local host address on the machine. This
includes addresses from local-network and Tailscale interfaces so that a user running
Kandev on a remote VM can identify an address to open from another machine.

The banner lists IPv4 addresses before IPv6 addresses, omits link-local addresses, and
wraps IPv6 hosts in brackets. Network-address discovery is best effort: if the host
cannot enumerate its interfaces, the launcher still starts and prints the localhost
URL. When `KANDEV_SERVER_HOST` restricts the backend to loopback or specific addresses,
the banner omits unreachable interface addresses and only advertises matching binds.

### Backend restart from the UI still rebuilds

The restart supervisor (ADR 0019) writes a launch manifest and listens on a control
socket so Settings can restart the backend. In dev that restart must rebuild from source,
which is what makes the affordance useful after editing Go code. The dev launcher
therefore keeps `make -C apps/backend dev` as the backend child command rather than
re-execing itself.

That creates a constraint the TypeScript launcher never had: the launcher and the backend
are the same binary (`bin/kandev`, dispatched by the `__backend` argument), so a rebuild
would overwrite the running launcher's own executable — which fails outright on Windows.
`make dev` therefore execs a copy of the freshly built binary from a distinct path, and
the copy supervises `make -C apps/backend dev`. The copy is a build artifact, not a
second shipped binary.

### `make dev` stops depending on Node and winjob

```makefile
dev: doctor
	@$(MAKE) -C $(BACKEND_DIR) build-kandev
	@cp bin/kandev → bin/kandev-launcher
	@exec $(BACKEND_DIR)/bin/kandev-launcher dev $(DEV_FLAGS)
```

`build-kandev` is the only root prebuild: the copied supervisor binary does not need
agentctl helpers before it starts the supervised child. The child's `make -C apps/backend
dev` runs `dev: build-dev`, which builds the native agentctl and, on hosts other than
Linux/amd64, one `linux/amd64` helper, plus mock-agent, acpdbg, and winjob. Full
four-platform helper builds remain under `build-runtime` for release and service bundles.

`build-winjob` leaves the dev path: the Go launcher's process-group handling replaces it,
and the root `dev` target no longer invokes the winjob build step. `cmd/winjob` itself
stays in the tree for any other consumer. The lean `dev: build-dev` target still builds
winjob once because the backend's Windows process handling remains part of the local
development binary set.

### `apps/cli` shrinks to the published shim

After `dev` lands in Go, `apps/cli/src/**` has no consumer and is deleted in full,
together with `tsconfig.json` and `vitest.config.ts`. What remains is exactly what npm
publishes plus its test:

```text
apps/cli/
├── package.json
└── bin/
    ├── cli.js              # 4-line exec of the native binary
    ├── native-shim.js      # platform-package resolution
    └── native-shim.test.mjs
```

`native-shim.test.mjs` moves from Vitest to the Node built-in test runner, so `apps/cli`
needs no `devDependencies` at all and `make test-cli` becomes `node --test`. The package
keeps its `name`, `version`, `bin`, `files`, and `optionalDependencies` untouched: the
npm release flow (`scripts/release/publish-npm.sh`, `.github/workflows/release.yml`)
reads and bumps `apps/cli/package.json` and must keep working unchanged.

Node remains required for exactly three things, and this spec does not attempt to remove
them: building and serving the web app (Vite), the published npm shim, and repo tooling
(prettier, commitlint, Playwright). That is the floor.

## Non-goals

- Removing Node from the web build or from `apps/web`. Vite is the dev server.
- Changing what the npm `kandev` package ships or how it resolves `@kdlbs/runtime-*`.
- Removing `apps/backend/cmd/winjob` from the repository.
- Changing `make start`, `make service-*`, or the desktop/Tauri launch path, all of
  which already use the Go binary.
- Changing E2E backend spawning, which already execs `KANDEV_BIN __backend` directly
  (`apps/web/e2e/fixtures/backend.ts:287`).

## Failure modes

| Condition | Expected behavior |
| --- | --- |
| Repo root not found from CWD | Exit 2 with a message telling the user to run from the repo. |
| Explicit backend or web port occupied | Exit 1 naming the port and its source (flag vs. env). No browser is opened. |
| Production DB backup fails | Exit 1 before the backend starts; the message names the DB path. |
| Backend fails health within 600s | Print the captured backend output, shut down the tree, exit 1. |
| Vite never becomes ready | Shut down the tree, exit 1. |
| Either child exits | Shut down the tree; exit with that child's code, or 0 if it was signalled. |
| `pnpm` missing | Surface the spawn error rather than hanging on the readiness probe. |
| Ctrl-C | Whole tree terminates, including on Windows, without winjob. |
| Network interface enumeration fails | Continue startup and omit `network:` lines; keep the localhost URL. |

## Persistence guarantees

- `.kandev-dev/` is the only state `make dev` writes, unless the user explicitly sets
  `KANDEV_DATABASE_PATH` outside a task workspace.
- `dev-prod-db-*.db` snapshots are retained five deep in `~/.kandev/data/backups/` and
  never prune other backup families.
- The supervisor manifest and control socket live under
  `<KANDEV_HOME_DIR>/supervisor/` at mode `0600`, unchanged from `start`.

## Scenarios

- **GIVEN** a host with non-loopback LAN and Tailscale IPv4 addresses, **WHEN** the
  `dev`, `start`, or `run` launcher prints its startup banner, **THEN** it prints one
  `network:` URL for each address using the backend port.
- **GIVEN** a host with duplicate addresses, loopback addresses, or link-local
  addresses, **WHEN** the startup banner is printed, **THEN** duplicate, loopback, and
  link-local addresses are omitted.
- **GIVEN** a host with an IPv6 address, **WHEN** the startup banner is printed, **THEN**
  the address is rendered as `http://[<address>]:<port>`.
- **GIVEN** interface enumeration returns an error, **WHEN** the launcher starts,
  **THEN** it continues startup and prints the localhost URL without a `network:` line.
- **GIVEN** `KANDEV_SERVER_HOST` is a loopback or specific bind address, **WHEN** the
  startup banner is printed, **THEN** it omits unrelated network addresses.
