---
id: "01-admit-focus-resume-as-automatic"
title: "Admit focus resume as automatic"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-SESSION-CEILING-001
acceptance_criteria:
  - AC-AGENTS-SESSION-CEILING-001.1
system_design:
  - ../../specs/agents/system-design/session-concurrency-ceiling.md
---

# Task 01: Admit focus resume as automatic

## Summary

Admit a focus-driven idle-suspension resume against the session ceiling as an
automatic launch, so passive focus never uses a manual override.

## In scope

- Add a regression test for focus with a saturated ceiling before the change.
- Change the focus resume origin from manual to automatic.
- Keep the completed-session permission and the idle-suspension check in the
  deferred resume record, so a refused focus resume of a parked completed
  session replays once capacity frees.
- Record the focus origin and its replay in the session ceiling design.

## Out of scope

Explicit Resume, message delivery, ceiling configuration, and UI changes.

## Acceptance

- With the ceiling full, focusing an idle-suspended session launches no agent.
- A deferred focus resume of a parked completed session reaches the executor
  when the sweep replays it with free capacity.
- Existing focus, idle-suspension, and ceiling replay tests pass.

## Verification

From the repository root:

```bash
(cd apps/backend && go test ./internal/orchestrator -run 'Ceiling|Seam|Focus|IdleSuspension|IdleSession|Deferred|Replay|AutoResume|SessionOpen|Resume' -count=1)
(cd apps/backend && go vet ./internal/orchestrator)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- apps/backend/internal/orchestrator/idle_session_focus.go
- apps/backend/internal/orchestrator/idle_session_focus_ceiling_test.go
- apps/backend/internal/orchestrator/ceiling_seam4.go
- apps/backend/internal/orchestrator/ceiling_replay.go
- apps/backend/internal/orchestrator/ceiling_entry.go
- apps/backend/internal/orchestrator/ceiling_dispatch_admission.go
- apps/backend/internal/orchestrator/task_operations.go
- docs/specs/agents/system-design/session-concurrency-ceiling.md

## Dependencies

None.

## Risks

A refused focus resume waits for capacity through the existing deferral path.
If the idle policy no longer parks the session when the sweep replays the
record, the record is dropped as superseded instead of retried.

## Parallelism

`sequential`

## Inputs

- Issue #4321.
- REQ-AGENTS-SESSION-CEILING-001 and its automatic and manual admission criteria.
- The linked session ceiling design.

## Results

- RED: the new test failed on the original code with one agent launched past
  the ceiling.
- GREEN: the new test and the existing focus and idle-suspension tests pass.
- RED: the completed-session replay test launched no agent before the deferred
  record kept the focus resume permission; it passes with the change.
- `TestCeilingDispatchAdmissionSerializesRouteMutation` (`start_created` and
  `prompt_ensure`) fails on Windows with and without this change.
- `go vet` and `golangci-lint` on the changed package report no issues.
