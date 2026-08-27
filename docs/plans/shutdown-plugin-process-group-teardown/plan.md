---
spec: docs/specs/platform/requirements/shutdown-log-noise.md
created: 2026-08-13
status: done
---

# Implementation Plan: Let plugin cleanup run before launcher force kill

## Overview

Change the launcher’s graceful child shutdown so its first signal targets only
the supervised root process on platforms where that is safe. Unix dev runs
through a make wrapper, so the backend writes its actual PID to a launcher-owned
file and the launcher targets that validated same-process-group child. Keep
process-group termination for the existing grace-expiry and second-signal force
paths, plus the existing tree-wide graceful fallback on platforms with wrapper
processes that could otherwise be orphaned. Force cleanup also runs after the
root exits so remaining descendants are not leaked. The existing go-plugin
adapter already has the correct severity policy: expected deliberate
termination is DEBUG, while an unexpected plugin exit remains ERROR. The
launcher change makes that policy reachable before a plugin is killed directly
by the parent’s process-group signal.

## Confirmed root cause

The launcher creates a separate process group for each supervised child and
currently sends SIGTERM to the entire group from
`apps/backend/internal/launcher/process.go`. Backend-owned plugin subprocesses
inherit that group. On Ctrl+C they therefore receive SIGTERM at the same time as
the backend, before `runtime.Manager.StopAll` calls `hcProcess.Kill` and sets
the adapter’s intentional-stop flag. Hashicorp go-plugin then logs
`plugin process exited ... error="signal: terminated"` at ERROR. The backend
still reports `Graceful shutdown complete` with `error_count: 0`, so the
observed plugin lines are misleading teardown noise rather than plugin
failures.

## Backend

### Launcher graceful signal boundary

Modify `apps/backend/internal/launcher/process.go` and the platform helpers in
`apps/backend/internal/launcher/process_group_{unix,windows,default}.go`:

- Add a platform-specific root-only graceful termination helper.
- Make `managedProcess.kill` use the root-only helper for its first signal,
  resolving the backend PID handoff for Unix dev wrappers.
- Retain `killManagedProcessGroup` for the existing grace-expiry and
  second-signal force paths.
- Run process-group force cleanup after a root exits before descendants, and
  retain the unsupported-platform limitation where no portable group cleanup
  exists.
- Update shutdown debug messages and error handling to distinguish root-only
  graceful signaling from process-group force cleanup.
- Preserve the existing 75-second grace duration, second-signal behavior,
  result summaries, child exit-code handling, and process-group setup.

The platform helper must preserve the launcher contract on Unix, Windows, and
unsupported platforms. On Windows, the implementation must account for the
existing `make -C apps/backend dev` wrapper rather than leaving the backend
child orphaned when only the wrapper receives the graceful signal, so Windows
retains tree-wide graceful termination. If the platform cannot provide a safe
root-only signal or portable descendant cleanup, retain the available fallback
and document that limitation instead of claiming a complete-tree guarantee.

### Plugin log-level policy

Do not broadly downgrade all plugin exits. Keep
`apps/backend/internal/plugins/runtime/hclog_adapter.go` unchanged unless the
implementation demonstrates that an intentional runtime kill still reaches
ERROR after the launcher sequencing fix. The required policy is:

- deliberate shutdown termination (`signal: terminated` or `signal: killed`)
  is DEBUG/no ERROR;
- an unexpected plugin exit while the runtime is active remains ERROR in the
  go-plugin adapter and WARN in the runtime supervisor’s existing unexpected
  exit path.

## Tests

1. **Root-only graceful signal.** Extend
   `apps/backend/internal/launcher/process_group_unix_test.go` with a helper
   process that starts a descendant in the same process group. Assert that
   `managedProcess.kill` lets the root handle SIGTERM and exit while the
   descendant does not receive the initial graceful signal. Clean up the
   descendant through the existing process-group force helper.
2. **Backend PID handoff.** Exercise a managed Unix wrapper whose PID file
   names an in-group backend child. Assert that the launcher signals that
   child, not the wrapper, and still completes shutdown without an error.
3. **Force cleanup remains tree-wide.** Keep or extend the existing second
   signal/force-kill coverage to assert that a descendant that ignores the
   graceful phase is reaped by process-group force cleanup, including after
   the root exits.
4. **Plugin severity regression.** Re-run the existing focused adapter tests in
   `apps/backend/internal/plugins/runtime/hclog_adapter_test.go`; add a test
   only if the launcher/runtime integration exposes a new failure mode. Assert
   that deliberate termination is DEBUG and an unexpected exit remains ERROR.

## Verification Results

- Root-only graceful signaling and post-root descendant cleanup regressions
  passed under `go test -race`.
- Backend PID handoff writer and Unix wrapper-target regression passed.
- Existing second-signal force-shutdown coverage passed.
- Plugin adapter severity coverage passed all six focused tests.
- `go test ./internal/launcher ./internal/plugins/runtime` passed.
- Windows launcher and backend packages cross-compiled successfully with
  `GOOS=windows GOARCH=amd64 go test -c`.
- `golangci-lint run ./internal/launcher ./cmd/kandev --timeout=5m` passed
  (0 issues).
- `git diff --check` passed.

The plugin adapter required no additional log-level change. Unix and Darwin
now signal the supervised root first, or the backend PID under the Unix dev
make wrapper. Windows keeps tree-wide graceful termination because the make
wrapper makes root-only signaling unsafe. Process-group force cleanup also
runs after root exit where supported; unsupported platforms retain their
available root cleanup behavior.

## Implementation Waves And Parallel Candidates

Wave 1, sequential:

- [x] [task-01-launcher-graceful-signal](task-01-launcher-graceful-signal.md)

The task is sequential because the launcher implementation, platform helpers,
process-group integration tests, and plugin severity verification share the
same shutdown boundary.

## Risks

- A root-only signal must not orphan the `make` wrapper or Vite child on
  Windows. Windows retains the documented tree-wide graceful fallback, while
  Unix dev targets the backend PID handoff.
- Changing the first signal must not weaken the existing second-signal or
  grace-expiry force cleanup.
- No task/session state, WS response, plugin restart policy, or grace timeout
  change is intended.
