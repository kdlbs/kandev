# ADR-2026-08-08-go-launcher-owns-all-launch-modes: The Go launcher owns every entrypoint; apps/cli is a publish-only shim

**Status:** accepted
**Date:** 2026-08-08
**Area:** backend, cli, infra

## Context

The TypeScript launcher under `apps/cli/src/**` shrank target by target as the Go launcher
(`apps/backend/internal/launcher/`) took over: `service` first, then `start`/`run`, and finally
`dev` — the last consumer of the entire `src/` tree. Every launcher behavior fix had been landing
twice (PR #2394 hardened port ownership in both TypeScript and Go in one PR), and `make dev`'s
`bash → make → pnpm → node → make → sh → kandev` chain forced the winjob Job-Object wrapper on
Windows. With `dev` moved, `apps/cli/src/**` was unpublished dead code (the npm package already
shipped only `bin/cli.js` + `bin/native-shim.js`) and was deleted in full. This change completes
the boundary that the earlier migrations were building toward.

## Decision

All four launch modes (`dev`, `start`, `run`, `service`) are owned by the native Go binary at
`apps/backend/bin/kandev`; new launcher features and behavior changes are implemented in
`apps/backend/internal/launcher/` (or `internal/launcher/cli/` for CLI-surface text), never in
Node. `apps/cli` is a publish-only npm shim: its `files` are exactly the two shim modules plus
package metadata, it has no TypeScript source and no devDependencies, and its test runs on the
Node built-in test runner (`node --test`). `make dev` execs a copy of the freshly built binary
(`apps/backend/bin/kandev-launcher`, git-ignored) because the supervised child `make -C
apps/backend dev` rebuilds `bin/kandev` underneath the running launcher. Node remains required for
exactly three things: the Vite dev server (the web build), the published npm shim, and repo
tooling (prettier, commitlint, Playwright).

## Consequences

- Launcher behavior exists once; port preflight, health tokens, Ctrl-C handling, and the restart
  supervisor no longer have parallel implementations.
- Windows `make dev` no longer needs winjob — the launcher's `CREATE_NEW_PROCESS_GROUP` +
  `taskkill /F /T` handling covers the dev path, though real Windows Ctrl-C still needs manual
  verification because CI does not run a full `make dev`.
- The npm `kandev` package keeps its published surface byte-identical (`name`, `version`, `bin`,
  `files`, `main`, `engines`, `optionalDependencies`), so the release flow is untouched.
- Backend restarts from the UI rebuild from source in dev because the child command stays
  `make -C apps/backend dev`; `bin/kandev-launcher` is a build artifact, not a second shipped
  binary.

## Alternatives Considered

- **Re-exec `self __backend` for dev** (what `start` does): simpler, but silently drops
  rebuild-on-restart, which is the point of the restart button in dev. Rejected.
- **Exec `bin/kandev` directly in `make dev`**: the child `make` rebuilds that file underneath
  the running launcher; the running process survives on Unix (go build unlinks first) but the
  rebuild fails outright on Windows. The one-`cp` indirection removes the hazard on every
  platform. Rejected.
- **Keeping `apps/cli/src` as the dev rollback**: it was the safety net during the cutover but
  became pure dead weight once `make dev` was proven on the Go path. Deleted in the same change.
