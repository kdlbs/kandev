---
id: "06-makefile-cutover"
title: "Makefile cutover to the Go dev launcher"
status: done
wave: 3
depends_on: ["05-run-dev"]
plan: "plan.md"
spec: "../../specs/platform/requirements/go-dev-launcher.md"
---

# Task 06: Makefile cutover to the Go dev launcher

Point `make dev` at the native launcher and drop the Node and root winjob steps.

## Acceptance

- `make dev` prebuilds only `apps/backend/bin/kandev`, copies it to a distinct launcher
  path, and `exec`s that copy with `dev` plus the existing `DEV_FLAGS`. The supervised
  backend `dev` target builds the native agentctl and, on hosts other than Linux/amd64,
  one Linux/amd64 helper. It no longer invokes `pnpm` or `tsx` directly.
- `make dev PORT=… WEB_PORT=…` still forwards `--port` / `--web-internal-port`
  unchanged, and `make dev-prod-db` still exports `KANDEV_DATABASE_PATH` and delegates
  to `make dev`.
- The copied launcher path is git-ignored and removed by `make clean`.
- `apps/backend/Makefile:156`'s comment no longer points at `apps/cli/src/dev.ts` for
  winjob usage.

## Verification

~~~bash
make dev
~~~

Confirm by hand: backend and Vite both start, the browser opens on the backend port, the
banner shows the `.kandev-dev` DB path, and Ctrl-C leaves nothing behind
(`pgrep -af 'kandev|vite' || echo clean`). Then repeat with
`make dev PORT=38500 WEB_PORT=37500` and with `make dev-prod-db`, checking that a
`dev-prod-db-*.db` file appears in `~/.kandev/data/backups/`.

## Files

- `Makefile` — the `dev` target (currently lines 187-195) and `clean`
- `apps/backend/Makefile` — the lean dev build targets and `build-winjob` doc comment
- `.gitignore` — the copied launcher binary

## Inputs

- `Makefile:277` — `make start`'s `@exec $(BACKEND_DIR)/bin/kandev start …`, the shape
  to mirror.
- `Makefile:60-62` — `DEV_PORT_FLAG`, `DEV_WEB_PORT_FLAG`, `DEV_FLAGS`, unchanged.
- `apps/backend/Makefile:30` — `EXE` is defined in the backend Makefile, not the root
  one. The root `dev` target needs its own suffix handling for the `cp`, or should
  delegate the copy to a backend target.

## Risks

- The copy exists because the child `make` rebuilds `bin/kandev` underneath the running
  launcher. Do not "simplify" it away by exec'ing `bin/kandev` directly — that breaks
  backend restart on Windows and is fragile everywhere.
- `build-winjob` leaving `make dev` is the one behavior change with no automated
  coverage on Windows. Either verify a real Windows `make dev` Ctrl-C, or keep the
  `build-winjob` line for one release and remove it in a follow-up; say which in the PR.
