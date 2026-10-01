---
id: "04-resume-safe-idle-reclaim"
title: "Resume-safe idle reclaim"
status: done
wave: 4
depends_on: ["03-resume-validation"]
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-IDLE-PARKING-002
acceptance_criteria:
  - AC-EXECUTORS-IDLE-PARKING-002.1
  - AC-EXECUTORS-IDLE-PARKING-002.2
  - AC-EXECUTORS-IDLE-PARKING-002.3
system_design:
  - ../../specs/executors/system-design/idle-runtime-parking.md
---

# Task 04: Resume-safe idle reclaim

## Summary

Keep the always-on idle reclaim from releasing the runtime of a waiting
session whose row lifecycle cleanup would delete: no resume token and a status
other than `running`. Without that row, a later prompt returns
`ErrSessionRuntimeUnavailable` and the message stays queued; no code path was
found that launches the runtime for it afterwards.

## In scope

- In `classifyIdleReclaim`, skip a `WAITING_FOR_INPUT` or `IDLE` session whose
  row has no resume token and a status other than `running`, and log the skip
  as `skipped_no_resume_token`.
- Keep reclaim unchanged for token-bearing rows, tokenless `running` rows, and
  `COMPLETED` sessions.
- Table tests for the decision matrix, the resulting row for each case under
  the lifecycle cleanup rule, and the prompt that follows a reclaim.
- A lifecycle test that pins the deletion of a tokenless `prepared` or `ready`
  row by stale-execution cleanup.

## Out of scope

- Changing the lifecycle stale-execution cleanup deletion rule.
- Changing the workspace ACP idle-suspension policy or its eligibility.
- Recovering sessions whose row an earlier reclaim already removed.

## Acceptance

- `AC-EXECUTORS-IDLE-PARKING-002.1`: a `WAITING_FOR_INPUT` or `IDLE` session
  whose row has no resume token and is not `running` keeps its runtime and row.
- `AC-EXECUTORS-IDLE-PARKING-002.2`: a prompt to such a `WAITING_FOR_INPUT`
  session after the reclaim pass attempts an agent launch and does not return
  `ErrSessionRuntimeUnavailable`.
- `AC-EXECUTORS-IDLE-PARKING-002.3`: token-bearing and tokenless `running`
  rows are still repaired in place, and tokenless `COMPLETED` rows are still
  reclaimed.

## Verification

```bash
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run 'TestClassifyIdleReclaimDisposition|TestReclaimIdleSession|TestPromptAfterIdleReclaimLaunchesPreparedSession|TestIdleReaper_' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle -run 'TestDeleteExecutorRunning' -count=1)
(cd apps/backend && golangci-lint run ./internal/orchestrator/... ./internal/agent/runtime/lifecycle/... --new-from-rev=e96910ff9a44d9c5e2970f8b92f50039b001d31e --timeout=10m)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/orchestrator/reconcile_liveness.go`
- `apps/backend/internal/orchestrator/idle_session_reaper.go` (comments)
- `apps/backend/internal/orchestrator/reclaim_lifecycle_test.go`
- `apps/backend/internal/orchestrator/reclaim_resume_token_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/resume_safety_test.go`

## Results

- `classifyIdleReclaim` returns `skipped_no_resume_token` for a
  `WAITING_FOR_INPUT` or `IDLE` session whose row has no resume token and is
  not `running`, after the live-runtime and active-turn checks. Tokenless
  `running` rows and `COMPLETED` sessions are unchanged.
- Before the production change, the new tests failed: the tokenless `prepared`
  waiting row and `ready` idle row were deleted, and the following prompt
  returned
  `session runtime unavailable: ... session is not resumable: no executor record`.
  After the change they pass.
- The existing idle reaper tests pass unchanged; their rows are `running`.

## Maintainer fixup results (2026-10-01)

- Each backend verification command now runs in a subshell, so later commands
  start from the repository root.
- The idle-reclaim skip log includes the row status and whether a resume token
  exists. It does not log the token value.
- The focused orchestrator and lifecycle tests pass, and the changed-package
  backend lint reports no issues.
- Specification validation and whitespace checks pass.
