---
id: "01-continuous-draining"
title: "Preserve continuous child log draining"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DIAGNOSTIC-SIGNAL-001
  - REQ-PLATFORM-DIAGNOSTIC-SIGNAL-003
acceptance_criteria:
  - AC-PLATFORM-DIAGNOSTIC-SIGNAL-001.1
  - AC-PLATFORM-DIAGNOSTIC-SIGNAL-001.2
  - AC-PLATFORM-DIAGNOSTIC-SIGNAL-001.3
  - AC-PLATFORM-DIAGNOSTIC-SIGNAL-001.4
  - AC-PLATFORM-DIAGNOSTIC-SIGNAL-003.1
  - AC-PLATFORM-DIAGNOSTIC-SIGNAL-003.2
  - AC-PLATFORM-DIAGNOSTIC-SIGNAL-003.3
  - AC-PLATFORM-DIAGNOSTIC-SIGNAL-003.4
system_design:
  - ../../specs/platform/system-design/runtime-diagnostic-signal.md
---

# Task 01: Preserve continuous child log draining

## Summary

Replace silent Scanner termination with bounded record draining on both launcher-owned streams.
Deliver failing regressions first, then the correction and targeted race validation.
Keep existing severity, sanitization, cancellation, and process ownership.

## Progress

- [x] Reconcile the current requirements, design, plan, and launcher source.
- [x] Mark this work order and its plan in progress.
- [x] Add failing pipe regressions.
- [x] Implement bounded continuous draining.
- [x] Run the targeted tests and build.
- [x] Record results and complete the work order.

## In scope

- New tests in `launcher_output_test.go` against the real `Launcher.pipeOutput` path.
- Fixed fragment reads, 64 KiB accumulation, and complete discard through oversized-record delimiters.
- Content-free, bounded diagnostics for discarded records and unexpected read failures.
- Existing relay test call sites after the private input changes to `io.Reader`.
- EOF, pipe closure, subprocess progress, and lifecycle compatibility evidence.

## Out of scope

- Production or permanent test changes before a later explicit implementation request.
- Readiness budgets, resume retry/error changes, and execution tracking changes.
- Live mutations, restarts, commits, pushes, deployment, or new persistent tasks.
- New logging infrastructure, metrics, flags, UI, and stronger post-exit tail guarantees.

## Acceptance

1. Both-stream regressions first fail because an oversized record stops the old drainer and blocks the writer.
   After the correction, writers complete and the next ordinary record reaches the parent observer.
2. Fragment consumption uses fixed memory and preserves all bounded-record severity and sanitization tests.
   Repeated oversized records, missing LF, and injected failures produce only bounded, safe diagnostics.
3. EOF and supervised descriptor closure release readers and helper processes within bounded test waits.
   All listed checks pass without additional production goroutines or changed lifecycle semantics.

## TDD sequence

1. Read the requirement, design, investigation findings, and current launcher tests.
2. Mark this work order `in_progress` and synchronize the plan.
3. Add `TestPipeOutputOversizedRecordContinuesDraining` before changing production code.
   Parameterize stdout and stderr. Use `os.Pipe`, an observer logger, a 128 KiB record, and a normal record.
   Assert writer completion, continued reader lifetime before EOF, and subsequent record forwarding.
   Bound waits at three seconds. Close both descriptors in cleanup and join every owned goroutine.
   Record the expected failure against the unchanged implementation.
4. Add boundary and injected-read tests named in the plan.
   Supply a large stream through small reusable chunks to avoid a test fixture that hides unbounded accumulation.
   Cover final records, EOF, closure while discarding, repeated records, and error text containing a synthetic secret.
5. Implement fixed fragment reads and bounded diagnostics in a focused helper file if necessary.
   Keep normal forwarding in the existing parser path.
   Update the private input and existing observer call sites.
6. Add `TestPipeOutputOversizedRecordDoesNotBlockChild` using the Go test executable as a helper subprocess.
   Exercise both stdout and stderr with large output followed by normal output and process exit.
   Bound process waits. On failure, kill and reap only the owned helper, close pipes, and join readers.
   Do not run a real agentctl server or mutate live data.
7. Run the exact checks and record results in both files.
   Mark the work order `done` only after every listed check passes.

## Verification

Run from the repository root. The cache path must remain writable and owned by this task.

```bash
mkdir -p /tmp/kandev-agentctl-drain-go-cache
(cd apps/backend && GOCACHE=/tmp/kandev-agentctl-drain-go-cache GOMAXPROCS=2 go test -trimpath -p 2 ./internal/agent/runtime/agentctl/launcher -run '^TestPipeOutput' -count=1 -timeout=90s)
(cd apps/backend && GOCACHE=/tmp/kandev-agentctl-drain-go-cache GOMAXPROCS=2 go test -trimpath -p 2 ./internal/agent/runtime/agentctl/launcher -count=1 -timeout=90s)
(cd apps/backend && GOCACHE=/tmp/kandev-agentctl-drain-go-cache GOMAXPROCS=2 go test -trimpath -race -p 2 ./internal/agent/runtime/agentctl/launcher -count=1 -timeout=120s)
(cd apps/backend && GOCACHE=/tmp/kandev-agentctl-drain-go-cache GOMAXPROCS=2 go test -trimpath -p 2 ./internal/agent/runtime/lifecycle -run '^TestManager_CleanupStaleExecution_' -count=1 -timeout=120s)
(cd apps/backend && GOCACHE=/tmp/kandev-agentctl-drain-go-cache GOMAXPROCS=2 CGO_ENABLED=1 go build -trimpath -tags fts5 -ldflags '-s -w' -o /tmp/kandev-agentctl-drain-backend ./cmd/kandev)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The first command is also the red-phase command.
Document its expected failure before the production correction.
Do not run package commands concurrently on this host.

## Files likely touched

- `apps/backend/internal/agent/runtime/agentctl/launcher/launcher.go`
- `apps/backend/internal/agent/runtime/agentctl/launcher/launcher_output.go` (new helper, if necessary)
- `apps/backend/internal/agent/runtime/agentctl/launcher/launcher_output_test.go` (new)
- `apps/backend/internal/agent/runtime/agentctl/launcher/launcher_json_log_test.go`
- `apps/backend/internal/agent/runtime/agentctl/launcher/launcher_relay_level_test.go`
- `docs/plans/agentctl-log-drain/plan.md`
- `docs/plans/agentctl-log-drain/task-01-continuous-draining.md`

## Dependencies

None. Existing JSON severity behavior and its completed work order are compatibility inputs.

## Risks

- Partial JSON must never reach envelope parsing or parent logs.
- Read errors can contain private data. Diagnostics require fixed classes rather than `zap.Error(err)`.
- A test timeout must not leak a blocked writer, reader, or subprocess.
- `Cmd.Wait` closes output descriptors. Tests must accept existing tail behavior without weakening writer-progress assertions.

## Parallelism

`sequential`

## Inputs

- [Runtime diagnostic requirements](../../specs/platform/requirements/runtime-diagnostic-signal.md), REQ-001 and REQ-003.
- [Runtime diagnostic design](../../specs/platform/system-design/runtime-diagnostic-signal.md), child forwarding and pipe draining.
- [Investigation and scope](plan.md#investigation-evidence).
- Existing JSON, console, slog, relay, exit-callback, survival, and stale-cleanup tests.

## Results

- Red phase: the targeted `TestPipeOutput` command failed as expected. Both stream writers blocked behind oversized records; boundary, no-newline, bounded-diagnostic, read-error, closure, and subprocess regressions also exposed the missing behavior.
- Post-fix targeted `TestPipeOutput` command: passed after implementing bounded draining.
- Full launcher package tests: passed (`0.091s`).
- Launcher package race tests: passed.
- Lifecycle stale-execution cleanup tests: passed (`0.017s`).
- Backend binary build with the `fts5` tag: passed.
- `python3 scripts/list-docs.py validate`: passed (355 decisions and 1404 specifications).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
