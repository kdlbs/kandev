---
id: "01-session-concurrency-ceiling"
title: "Preserve session ceiling launch ownership"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-SESSION-CEILING-001
acceptance_criteria:
  - AC-AGENTS-SESSION-CEILING-001.1
  - AC-AGENTS-SESSION-CEILING-001.2
  - AC-AGENTS-SESSION-CEILING-001.3
  - AC-AGENTS-SESSION-CEILING-001.4
  - AC-AGENTS-SESSION-CEILING-001.5
  - AC-AGENTS-SESSION-CEILING-001.6
  - AC-AGENTS-SESSION-CEILING-001.7
system_design:
  - ../../specs/agents/system-design/session-concurrency-ceiling.md
---

# Task 01: Preserve Session Ceiling Launch Ownership

## Outcome

Keep every accepted or deferred launch owned by one component across refusal,
callback, replay, and prompt-edit paths.

## In scope

- Return the existing resume attempt when seam-3 admission refuses a cold
  resume.
- Stop a successful early prompt-fallback result before callback preparation.
- Fence process callbacks by the current session execution identity.
- Preserve manual origin for the compound resume-and-prompt operation.
- Distinguish dynamic relaunch success, deferral, and failure.
- Keep Office start records out of workflow auto-start drop checks.
- Report a different pending payload as an explicit conflict.
- Requeue ordinary messages after seam-3 refusal.
- Update nested ceiling launch payload prompts.

## Exclusions

- New launch seams, new storage tables, or a frontend settings surface.
- Changes to provider protocols or the task card presentation.

## Acceptance

1. Automatic refusals either retain their exact durable replay record or return
   an explicit conflict. Manual actions remain admitted and audited.
2. A stale execution callback cannot release a successor reservation, and a
   deferred detached relaunch does not finalize its automation run.
3. Cold resume, queue retry, Office replay, dynamic outcome, and nested prompt
   regressions pass in focused tests.

## Verification

```bash
cd apps/backend && go test ./internal/orchestrator ./internal/task/service -count=1
cd apps/backend && go test -race ./internal/orchestrator -count=1
git diff --check
```

## Likely files

- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/dynamic_launch.go`
- `apps/backend/internal/orchestrator/ceiling_defer.go`
- `apps/backend/internal/orchestrator/ceiling_replay.go`
- `apps/backend/internal/orchestrator/ceiling_release.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/task/service/service_deferred_launch.go`
- matching regression tests

## Results

- Focused orchestrator regressions pass for cold-resume ownership, stale
  callback fencing, Office replay, duplicate conflicts, queue retry, and the
  dynamic deferred outcome.
- Focused task-service regressions pass for top-level and nested prompt edits.
