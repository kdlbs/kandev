---
status: implemented
created: 2026-08-07
spec: "../../specs/platform/requirements/go-dev-launcher.md"
---

# Plan: Go dev launcher and minimal Node surface

## Current state

`apps/backend/internal/launcher` already implements everything `dev` needs except the
dev-specific pieces. Reusable as-is:

| Concern | Location |
| --- | --- |
| Arg parsing, `ParseError`, port flags | `options.go` |
| Port selection, explicit-port preflight, `canBind` | `ports.go` |
| Health probe with launcher token, foreign-process detection | `health.go` |
| Process supervision, groups, Ctrl-C, force-kill, ring-buffered output | `process.go`, `process_group_{unix,windows}.go` |
| Restart control socket + launch manifest | `supervisor.go` |
| Backend env assembly | `env.go` |
| Startup banner, browser open, `waitForAppExit` | `start.go` |

Two details matter for the design:

- `supervisor.go:169` `allowedSupervisorEnv` already allowlists
  `KANDEV_WEB_INTERNAL_URL` and `KANDEV_DEBUG_DEV_MODE`. No change needed there.
- `supervisor.go:41` `launchRestartableBackend` calls `resolveHomeDir()` internally,
  reading the launcher's own environment. Dev needs a repo-local home, so this must
  become an explicit parameter (task 05).

Gaps, all dev-only:

1. No `dev` command, no `--web-internal-port`, no `WebPort` in `portConfig`.
2. No repo-root discovery, `.kandev-dev` home, or task-workspace detection.
3. No production-DB backup.
4. No Vite child launch or token-less URL readiness probe.

## Approach

Port `dev` into the Go launcher in three waves: pure helpers first (independently
testable, no ordering constraints between them), then command wiring and orchestration,
then the Makefile cutover and the TypeScript deletion.

The deletion is deliberately last and in its own task. Until `make dev` is proven on the
Go path, `apps/cli/src/**` is the rollback.

### Design decisions

**Backend child stays `make -C apps/backend dev`.** Re-execing `self __backend` (what
`start` does) would be simpler but would silently drop rebuild-on-restart, which is the
point of the restart button in dev. Keeping `make` preserves it.

**`make dev` execs a copy of the binary.** Because launcher and backend are one binary,
the child `make` rebuilds `bin/kandev` underneath the running launcher. On Linux/macOS
`go build` unlinks before writing so the running process survives; on Windows the delete
fails and the rebuild errors. Exec'ing `bin/kandev-launcher` (a copy made after `build`)
removes the hazard on every platform for one `cp`.

**Vite readiness is a separate probe.** `waitForHealth` requires the launcher health
token by design (PR #2394 made a token mismatch a hard failure). Vite emits no such
token, so it needs its own probe that accepts any HTTP response. Do not weaken
`waitForHealth` to accommodate it.

## Dependency order

```text
wave 1  task-01 dev paths      task-02 db backup      task-03 web child
            │                       │                      │
wave 2      └────────┬──────────────┴──────────────────────┘
                     ├── task-04 dev command wiring
                     └── task-05 runDev orchestration  (depends on 01–04)
                                  │
wave 3      ┌─────────────────────┼─────────────────────┐
            task-06 Makefile   task-07 delete TS    task-08 docs
                                (depends on 06)
```

Wave 1 tasks touch disjoint new files and are parallel-safe. Wave 3's task-07 must follow
task-06, because deleting `apps/cli/src` before the Makefile switches would break
`make dev` outright.

## Tasks

| ID | Title | Wave | Status |
| --- | --- | --- | --- |
| 01 | Dev paths, repo root, and dev backend env | 1 | done |
| 02 | Production database backup in Go | 1 | done |
| 03 | Vite dev-server child and URL readiness | 1 | done |
| 04 | `dev` command, web port, and dev env wiring | 2 | done |
| 05 | `runDev` orchestration | 2 | done |
| 06 | Makefile cutover to the Go dev launcher | 3 | done |
| 07 | Delete `apps/cli/src` and drop CLI devDependencies | 3 | done |
| 08 | Documentation and guidance updates | 3 | done |

## Likely files

New under `apps/backend/internal/launcher/`:
`devpaths.go`, `devpaths_test.go`, `backup.go`, `backup_test.go`, `webchild.go`,
`webchild_test.go`, `dev.go`, `dev_test.go`.

Modified: `options.go`, `options_test.go`, `ports.go`, `ports_test.go`, `env.go`,
`env_test.go`, `supervisor.go`, `supervisor_test.go`, `launcher.go`, `cli/help.go`,
`constants.go`.

Root: `Makefile`, `apps/backend/Makefile`, `apps/cli/package.json`,
`apps/cli/bin/native-shim.test.mjs`, `apps/package.json`,
`.github/workflows/frontend-tests.yml`, `CLAUDE.md`,
`docs/remote-cloud-environment.md`.

Deleted: `apps/cli/src/**`, `apps/cli/tsconfig.json`, `apps/cli/vitest.config.ts`.

## Risks

- **Windows dev has no CI coverage.** `make test-windows` runs a unit subset, not a real
  `make dev`. The winjob removal and the `cmd.exe /c pnpm` shim need manual verification
  on Windows before merge, or the winjob line in `make dev` should stay until someone
  can test. Call this out in the PR.
- **`pnpm -r exec tsc` breaks** once `apps/cli/tsconfig.json` is gone (Makefile:650).
  Task 07 must convert `make typecheck` to explicit `--filter` arguments.
- **`knip` dead-code detection** may flag the trimmed `apps/cli` differently. Run
  `pnpm run dead-code` as part of task 07.
- **Release flow reads `apps/cli/package.json`** in three places
  (`release.yml:224,251,381`, `publish-npm.sh:134,259`). Task 07 must not touch `name`,
  `version`, `bin`, `files`, or `optionalDependencies`.
- **`.kandev-dev` vs. inherited `KANDEV_HOME_DIR`.** The launcher's own
  `resolveHomeDir()` reads the ambient env; inside a Kandev task workspace that env is
  the parent backend's. Task 05's explicit-homeDir refactor is what prevents the dev
  supervisor directory from landing in the parent's home.

## Verification

Per-task commands are in the task files. Before opening the PR:

```bash
make -C apps/backend test
make -C apps/backend lint
cd apps && pnpm --filter kandev test
```

Manual, on the branch, since no automated test covers the real launch:

```bash
make dev                    # backend + Vite come up, browser opens, Ctrl-C leaves nothing
make dev PORT=38500 WEB_PORT=37500
make dev-prod-db            # verify a dev-prod-db-*.db lands in ~/.kandev/data/backups/
```

Confirm no stray processes after Ctrl-C:

```bash
pgrep -af 'kandev|vite' || echo clean
```
