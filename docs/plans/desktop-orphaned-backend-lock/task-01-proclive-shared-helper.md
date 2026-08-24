---
id: "01-proclive-shared-helper"
title: "Shared process-liveness helper"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/desktop/requirements/desktop-tauri-app.md"
parallelism: sequential
---

# Task 01: Shared process-liveness helper

Promote the existing private `processAlive` probe out of
`internal/system/storage/tempartifacts` into `internal/common/proclive` so the
launcher watchdog (task 02) and the ownership-lock diagnostics (task 04) consume
one implementation instead of adding a second and third copy.

This is a pure move plus re-export. Do not change the probe's behavior.

## Inputs

- Plan section "Shared process liveness (`internal/common/proclive`, new)".
- Existing implementation to move:
  `apps/backend/internal/system/storage/tempartifacts/process_alive_unix.go`
  and `process_alive_windows.go`.
- Its only current call site: `tempartifacts/registry.go:209`.
- `apps/backend/AGENTS.md`, "Cross-tier shared code belongs in `internal/common/`",
  which is why this lands as a shared package rather than being duplicated.

## Implementation

Create `apps/backend/internal/common/proclive/` with:

- `proclive.go` holding only the package doc comment. State that `known=false`
  means the platform cannot determine liveness and that callers must treat that
  as "do not act", since both consumers depend on that reading.
- `proclive_unix.go` (`//go:build !windows`) and `proclive_windows.go`
  (`//go:build windows`), each exporting:

```go
func Alive(pid int64) (alive, known bool)
```

Move the two existing bodies verbatim, changing only the package clause and the
identifier from `processAlive` to `Alive`. Keep the `int64` parameter: it matches
`tempartifacts`' `artifact.OwnerPID` exactly and avoids a 32-bit truncation
question at the boundary. Keep the Windows comment explaining why it returns
unknown.

Then delete both `tempartifacts/process_alive_*.go` files and update
`registry.go:209` to call `proclive.Alive(artifact.OwnerPID)`. Update
`registry_test.go:119` the same way. Do not change any surrounding
reconciliation logic.

## Acceptance

- `internal/common/proclive` exports `Alive(pid int64) (alive, known bool)` with
  Unix and Windows implementations, and both `tempartifacts/process_alive_*.go`
  files are gone.
- `tempartifacts` calls `proclive.Alive` and its existing tests pass unchanged.
- No behavior change: signal 0 on Unix with `EPERM` treated as alive and `ESRCH`
  as gone, `(false, false)` on Windows.

## Verification

```bash
cd apps/backend && go test ./internal/common/proclive/... ./internal/system/storage/tempartifacts/...
```

```bash
cd apps/backend && gofmt -l internal/common/proclive internal/system/storage/tempartifacts
```

The `gofmt` invocation must print nothing; the pre-commit hook fails the commit
otherwise.

## Files likely touched

- `apps/backend/internal/common/proclive/proclive.go` (new)
- `apps/backend/internal/common/proclive/proclive_unix.go` (new)
- `apps/backend/internal/common/proclive/proclive_windows.go` (new)
- `apps/backend/internal/common/proclive/proclive_test.go` (new)
- `apps/backend/internal/system/storage/tempartifacts/process_alive_unix.go` (delete)
- `apps/backend/internal/system/storage/tempartifacts/process_alive_windows.go` (delete)
- `apps/backend/internal/system/storage/tempartifacts/registry.go`
- `apps/backend/internal/system/storage/tempartifacts/registry_test.go`

## Dependencies

None.

## Tests to write

In `proclive_test.go`, table-driven:

- The current process (`int64(os.Getpid())`) reports `(true, true)` on Unix.
- A reaped child reports `(false, true)` on Unix. Start a real short-lived
  process with `os/exec`, `Wait` for it so the PID is fully released, then probe.
  Guard this case with `//go:build !windows` or a `runtime.GOOS` skip, since
  Windows returns unknown by design.
- `0` and a negative PID report `(false, true)` on Unix without touching the
  platform call.

## Output contract

Report the moved files, the migrated call sites, the exact commands run with
their outcomes, and confirmation that no `tempartifacts` behavior changed. Update
this file's `status` and `## Results`, and the matching checkbox in `plan.md`.

## Results

- Added `internal/common/proclive` with Unix signal-zero and Windows unknown
  implementations, plus Unix tests for the current process, a reaped child, and
  non-positive PIDs.
- Migrated `tempartifacts` production and test call sites to `proclive.Alive`;
  removed the duplicated platform files.
- `cd apps/backend && go test ./internal/common/proclive/... ./internal/system/storage/tempartifacts/...`
  passed (`ok` for both packages).
- `gofmt -w internal/common/proclive internal/system/storage/tempartifacts`
  completed successfully.
- Cleanup: the subprocess used by the liveness test is reaped with `Wait`;
  no temporary capture files or external resources remain.
- Security/trust and external side-effect boundaries: none.
