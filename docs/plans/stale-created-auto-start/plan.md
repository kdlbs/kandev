---
created: 2026-10-20
status: complete
requirements:
  - REQ-TASKS-STALE-CREATED-AUTOSTART-001
system_design:
  - ../../specs/tasks/system-design/stale-created-auto-start.md
legacy_specs: []
---

# Implementation Plan: Stale CREATED auto-start

## Overview

Stop an auto-start launch that lost its CREATED race from marking the session
and the task FAILED, and continue the dispatch from the state that won.

## Confirmed defect

An auto-start launch for a CREATED session persisted STARTING through a
CREATED-guarded write. When a concurrent manual move had set the session
WAITING_FOR_INPUT, the write returned an unmarked error. `startCreatedSession`
passed it to `handleSessionLaunchFailure`, whose `recordSessionLaunchFailure`
set the session FAILED and `updateTaskStateForEarlyLaunchFailure` set the task
FAILED. `launchCreatedAutoStartStepPrompt` reloaded the session before the
launch but checked only for terminal states, so a snapshot that was already
stale also launched a WAITING_FOR_INPUT session through the CREATED start path.

## Scope

### In scope

- Mark the refused STARTING write for non-terminal agentless states.
- Skip launch-failure recording for that marker in `startCreatedSession`.
- Continue a stale or lost CREATED auto-start without launching.
- Unit tests that replay the production sequence.

### Out of scope

- The manual-move entry path that writes WAITING_FOR_INPUT after a refused
  profile switch.
- Queue, delivery-ledger and capacity policy.

## Work packages

| Work order | Scope | Status |
| --- | --- | --- |
| [task-01](task-01-lost-created-race.md) | Persistence marker, start failure skip, auto-start continuation | done |

## Validation

- `go test -tags fts5 ./internal/orchestrator/ -run 'TestStaleCreatedAutoStart|TestAutoStartCreatedLaunch|TestWorkflowAsyncStartFailure' -count=1`
- `go test -tags fts5 ./internal/orchestrator/executor/ -count=1`
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all`
