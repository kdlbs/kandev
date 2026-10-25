---
id: "03-task-service"
title: "Identify and resolve the task-service failure"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-006
acceptance_criteria:
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.3
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.9
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.10
system_design:
  - ../../specs/plugins/system-design/conversation-source-reconciliation.md
---

# Task 03: Identify and resolve the task-service failure

## Summary

Identify the exact failing task-service test and resolve its demonstrated cause.
The earlier full backend run recorded only the package failure, so its cause is currently unknown.

## In scope

- Own failure isolation and the smallest task-service correction with regression coverage.
- Capture package output and identify each failing test before editing production code.
- Reproduce the same test on the comparison base. Do not assume it is unrelated or pre-existing.
- For conversation failures, preserve persisted source values, post-commit publication, and complete
  receipt intervals. Publication failures must not turn a successful durable write into a false rollback.
- If reproduction finds an unrelated defect, document the actual contract and add its existing
  requirement/design references before implementing a behavior change.

## Out of scope

A task-service rewrite, new conversation contracts, unrelated cleanup, and weakening tests.

## Acceptance

1. The report names every failing test and includes head/base evidence or an explicit reproduction limit.
2. The narrow regression and full task-service package pass under the race detector.
3. Tests prove the corrected behavior; no failure is hidden through skips or reduced assertions.

## Verification

Run at current and comparison worktree roots. Isolate reported test names with `-run` before fixing.
Record that exact command once the failure is known, then rerun the package after remediation.

```bash
(cd apps/backend && go test -race ./internal/task/service -count=1 -v)
```

If the failure cannot be reproduced, run the same command three times and record all results and
relevant environment differences. Do not claim a fix without a demonstrated change.

## Files likely touched

- `apps/backend/internal/task/service/*_test.go` (exact test determined by reproduction)
- `apps/backend/internal/task/service/service_messages.go`
- `apps/backend/internal/task/service/service_events.go`
- Other task-service files only as required by the isolated cause.

## Risks

The unknown failure may depend on timing or environment. Preserve full diagnostics and avoid speculative fixes.

## Dependencies

None. Run sequentially in the recommended order.

## Parallelism

`sequential`

## Inputs

- [Host requirements](../../specs/plugins/requirements/prompt-history-extraction-host.md) and [source reconciliation design](../../specs/plugins/system-design/conversation-source-reconciliation.md).
- [Architecture decision](../../decisions/2026-09-16-conversation-source-reconciliation.md).
- Implementation commit `c0a048bc128f7ef9a1051caf95ed442627faf9df`; comparison base `88c6c0fe0a6ae5d25332070d603b99f7d9241645`.
- [Original package](../conversation-storage-replacement/plan.md) and its recorded evidence.

## Results

- The current implementation head did not reproduce a task-service defect after isolating the package. `go test -race ./internal/task/service -count=1` passed in three consecutive runs (268.522s, 289.599s, and 272.722s in the recorded runs).
- The comparison base `88c6c0fe0a6ae5d25332070d603b99f7d9241645` also passed the same command (312.033s). No task-service production change was required, and no failure was hidden with a skip or reduced assertion.
- The package preserves durable-write success and post-commit publication behavior covered by its existing tests.
- Tested code revision: `213492517315abb38697b765e91e6fe0ea5388c7`.
