---
id: "01-break-lock-cycle"
title: "Break the replay lock cycle"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-SESSION-CEILING-001
acceptance_criteria:
  - AC-AGENTS-SESSION-CEILING-001.2
  - AC-AGENTS-SESSION-CEILING-001.5
system_design:
  - ../../specs/agents/system-design/session-concurrency-ceiling.md
---

# Task 01: Break the replay lock cycle

## Summary

Release replay admission before provider work and session-guard waits.
Keep unrelated scheduling available during task-local admission waits.

## In scope

- Add deterministic boundary tests before production changes.
- Narrow replay lock scope and correct review-state lock order.
- Preserve existing stale-entry rejection and claim settlement.

## Out of scope

Live data, process restart, UI changes, and ceiling configuration.

## Acceptance

- Provider dispatch permits an independent task-admission operation.
- A review callback waiting for task admission does not block another task's scheduling.
- Existing replay identity, claim, and boot-ready tests pass with the race detector.

## Verification

From the repository root:

This session uses a writable cache because the host Go cache is read-only.

```bash
export GOCACHE=/root/kandev/.worktrees/go-cache
(cd apps/backend && go test -race ./internal/orchestrator -run 'Test(Ceiling|ReplayCeiling|ClaimCeiling|ClearCeiling|WriteTaskReview|ReviewAdmission|HandleAgentBootReady|AgentBootReady|WorkflowEntryDispatch)' -count=1 -timeout=5m)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- apps/backend/internal/orchestrator/ceiling_replay.go
- apps/backend/internal/orchestrator/event_handlers_streaming.go
- apps/backend/internal/orchestrator/ceiling_boot_deadlock_test.go
- docs/specs/agents/system-design/session-concurrency-ceiling.md

## Dependencies

None.

## Risks

The released validation context must not represent continued lock ownership.
Final workflow-binding validation and durable claims must remain intact.

## Parallelism

`sequential`

## Inputs

- REQ-AGENTS-SESSION-CEILING-001 and its replay and manual-admission criteria.
- The linked session ceiling design.
- Live goroutine evidence summarized in the plan.
- Existing ceiling replay and boot-ready test fixtures.

## Results

- RED: Both new tests failed on the original code. Replay retained admission
  during provider dispatch, and review reconciliation blocked independent scheduling.
- GREEN: Both new tests passed after the correction (0.156 seconds).
- The race command above passed (4.449 seconds).
- `python3 scripts/list-docs.py validate` passed: 288 decisions and 1002 specifications.
- `python3 scripts/lint-spec-files.py --all` passed.
- `git diff --check` passed.
- The package status check found the plan and work order before staging.
- Public docs retain the existing automatic-retry contract. No public API, configuration,
  or rendered UI changes require documentation or screenshots.
