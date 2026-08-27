---
id: "01-launcher-graceful-signal"
title: "Signal launcher roots before force cleanup"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/shutdown-log-noise.md"
parallelism: sequential
---

# Task 01: Signal launcher roots before force cleanup

## Intent

Prevent Ctrl+C from directly terminating backend-owned plugin processes before
the backend can run its plugin cleanup. On platforms with safe root-only
signaling, the first graceful signal targets the supervised root process. The
existing process-group force path remains the fallback for grace expiry and
repeated interrupts; platforms with unsafe wrapper processes retain the
tree-wide graceful fallback.

## Acceptance

- On supported platforms, `managedProcess.kill` sends the first graceful
  termination signal only to the supervised root process. For the Unix dev
  make wrapper, it targets the backend PID written by the backend binary. A
  remaining descendant in the same process group is not directly signalled
  during that phase.
- If graceful shutdown does not complete before the existing grace period, or
  the launcher receives a second signal, process-group force cleanup still
  terminates the complete supervised tree, including descendants after root
  exit, where process-group support is available.
- The existing plugin adapter policy remains conservative: deliberate plugin
  termination is not logged at ERROR, while an unexpected active-runtime exit
  remains visible at ERROR/WARN through the existing paths. No broad log-level
  suppression is added.
- No change occurs to shutdown timeout, exit-code mapping, plugin lifecycle or
  restart policy, task/session state, or WS responses.

## Verification

From the repository root:

```bash
cd apps/backend && go test ./internal/launcher ./internal/plugins/runtime
```

If platform-specific launcher tests are unavailable on the current host, run
the available Unix/Linux package tests and record the unverified platform in
`## Results`.

## Files likely touched

- `apps/backend/internal/launcher/process.go`
- `apps/backend/internal/launcher/process_group_unix.go`
- `apps/backend/internal/launcher/process_group_windows.go`
- `apps/backend/internal/launcher/process_group_default.go`
- `apps/backend/internal/launcher/process_group_unix_test.go`
- `apps/backend/internal/launcher/process_group_windows_test.go` (if needed)
- `apps/backend/cmd/kandev/main.go` and `main_test.go`
- `apps/backend/internal/plugins/runtime/hclog_adapter.go` and its test only
  if focused verification proves the existing policy is still bypassed

## Dependencies and risks

No external dependencies. The main platform risk is preserving descendant
cleanup on Windows, where the dev backend is launched through `make` without a
POSIX `exec`; the implementation therefore retains Windows tree-wide graceful
termination.

## Results

- `go test -race ./internal/launcher -run
  'TestManagedProcessKillSignalsOnlyRootBeforeForceKill|TestAttachSignalsSecondSignalForceKillsChildren' -count=1`
  passed, including descendant cleanup after root exit.
- `go test ./internal/launcher -run
  'TestManagedProcessKill(SignalsOnlyRootBeforeForceKill|TargetsBackendPIDFile)$' -count=1`
  passed (2 tests).
- `go test ./cmd/kandev -run '^TestDispatchWritesBackendPIDFile$' -count=1`
  passed.
- The process-tree assertion treats a reaped Linux zombie as terminated, which
  avoids false leak failures while container init reaps force-killed children.
- `go test ./internal/plugins/runtime -run '^TestHCLogAdapter' -count=1`
  passed (6 tests).
- `go test ./internal/launcher ./internal/plugins/runtime` passed (227 tests).
- `GOOS=windows GOARCH=amd64 go test -c` passed for `./internal/launcher` and
  `./cmd/kandev`.
- `golangci-lint run ./internal/launcher ./cmd/kandev --timeout=5m` passed
  (0 issues).
- `git diff --check` passed.

Unix and Darwin use root-only graceful signaling, with the Unix dev make
wrapper targeting the backend PID handoff. Windows retains the existing
tree-wide graceful fallback because signaling only that wrapper could orphan
descendants. Process-group cleanup runs after root exit on supported platforms.
The plugin adapter was not modified: its existing DEBUG-for-deliberate-stop and
ERROR-for-unexpected-exit policy passed the focused tests.
