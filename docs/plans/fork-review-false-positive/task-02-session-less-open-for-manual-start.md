---
id: "02-session-less-open-for-manual-start"
title: "Open manual-start review tasks without an error"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.6
system_design:
  - ../../specs/integrations/system-design/github-fork-review-start.md
---

# Task 02: Open manual-start review tasks without an error

## Summary

Opening a task marked `fork_pr_requires_manual_start` in an auto-start step
returns no session instead of an error. Manual start still works.

## Change

In `EnsureSession` (`internal/orchestrator/session_ensure.go`), when
`intent == IntentStart` and `taskRequiresManualForkPRStart(task)` is true,
return without calling `LaunchSession`. Return `Success: true`, no session ID,
`Source: "awaiting_manual_start"`, and `ActivationDisposition:
activationDispositionSuppressed`. Add `"awaiting_manual_start"` to the
`source` union in `apps/web/lib/services/session-launch-service.ts`.

## Out of scope

Workspace-only prepare, because it would run the setup script on fork code.
Changing the `StartTask` guard. A UI notice.

## Steps

1. In the new file `internal/orchestrator/session_ensure_fork_review_test.go`,
   seed a marked task in a step that has `OnEnterAutoStartAgent`, using the
   same setup as `TestAutoStart_ForkReviewWaitsForManualStart`. Call
   `EnsureSession`, then assert that it returns no error, creates no sessions,
   and makes no `LaunchAgent` call. Confirm that the test fails first.
2. Add the guard and the TS literal.

## Verification

```bash
go -C apps/backend test -trimpath ./internal/orchestrator -run 'TestEnsureSession_ForkReview|TestAutoStart_ForkReview' -count=1
bash scripts/lint-go-changed apps/backend/internal/orchestrator/session_ensure.go apps/backend/internal/orchestrator/session_ensure_fork_review_test.go
cd apps && pnpm --filter @kandev/web typecheck
```

## Results

The regression failed first with `fork pull request review requires a manual
start`. After the change, opening the marked task returns the expected
session-less response and does not create a session or call `LaunchAgent`.
An explicit `AutoStart: true` still reaches the central launch guard and
returns `errForkPRManualStartRequired` without creating a session or calling
`LaunchAgent`. Existing coverage confirms manual start still launches the
fork review task.

Passed:

- `go -C apps/backend test -trimpath ./internal/orchestrator -run 'TestEnsureSession_ForkReview|TestAutoStart_ForkReview' -count=1`
- `bash scripts/lint-go-changed apps/backend/internal/orchestrator/session_ensure.go apps/backend/internal/orchestrator/session_ensure_fork_review_test.go`
- `cd apps && pnpm --filter @kandev/web typecheck`
