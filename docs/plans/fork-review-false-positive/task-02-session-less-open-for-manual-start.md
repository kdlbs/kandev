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

Opening a task marked `fork_pr_requires_manual_start` from a passive session
open returns no session without preparing its workspace, in any workflow step.
Explicit automatic starts still reach the central rejection. Manual start
still works.

## Change

In `EnsureSession` (`internal/orchestrator/session_ensure.go`), suppress a
marked task's passive `session_open` request unless it explicitly requests an
automatic start, regardless of the resolved workflow intent. Also suppress
step-derived automatic starts from other sources. Return `Success: true`, no
session ID, `Source: "awaiting_manual_start"`, and
`ActivationDisposition: activationDispositionSuppressed`. An explicit
`AutoStart: true` continues through to the central `StartTask` rejection. Add
`"awaiting_manual_start"` to the `source` union in
`apps/web/lib/services/session-launch-service.ts`.

## Out of scope

Workspace-only prepare, because it would run the setup script on fork code.
Changing the `StartTask` guard. A UI notice.

## Steps

1. In the new file `internal/orchestrator/session_ensure_fork_review_test.go`,
   seed a marked task in an auto-start step and another in a prepare step.
   Call `EnsureSession` with passive session-open options, including explicit
   `AutoStart: false`, then assert each returns no error, creates no sessions,
   and makes no `LaunchAgent` call. Confirm that the prepare-step test fails
   first.
2. Add the guard and the TS literal.

## Verification

```bash
go -C apps/backend test -trimpath ./internal/orchestrator -run 'TestEnsureSession_ForkReview|TestAutoStart_ForkReview' -count=1
go -C apps/backend test -trimpath ./internal/github -run 'TestCheckReviewWatch' -count=1
go -C apps/backend test -trimpath ./internal/github ./internal/orchestrator -count=1
bash scripts/lint-go-changed apps/backend/internal/github/service_reviews.go apps/backend/internal/github/service_reviews_test.go apps/backend/internal/github/enterprise_watch_fences_test.go apps/backend/internal/orchestrator/session_ensure.go apps/backend/internal/orchestrator/session_ensure_fork_review_test.go apps/backend/internal/orchestrator/event_handlers_github_review_test.go
cd apps && pnpm --filter @kandev/web typecheck
```

## Results

The auto-start regression failed first with `fork pull request review requires
a manual start`. The prepare-step regression failed first after creating a
session and launching workspace preparation. After the change, passive
session-open requests in prepare and auto-start steps return the expected
session-less response for unset and false `AutoStart`, without creating a
session or calling `LaunchAgent`. An explicit `AutoStart: true` still reaches
the central launch guard and returns `errForkPRManualStartRequired` without
creating a session or calling `LaunchAgent`. Existing coverage confirms manual
start still launches the fork review task.

Passed:

- `go -C apps/backend test -trimpath ./internal/orchestrator -run 'TestEnsureSession_ForkReview|TestAutoStart_ForkReview' -count=1`
- `go -C apps/backend test -trimpath ./internal/github -run 'TestCheckReviewWatch' -count=1`
- `go -C apps/backend test -trimpath ./internal/github ./internal/orchestrator -count=1`
- `bash scripts/lint-go-changed apps/backend/internal/github/service_reviews.go apps/backend/internal/github/service_reviews_test.go apps/backend/internal/github/enterprise_watch_fences_test.go apps/backend/internal/orchestrator/session_ensure.go apps/backend/internal/orchestrator/session_ensure_fork_review_test.go apps/backend/internal/orchestrator/event_handlers_github_review_test.go`
- `(cd apps/backend && go build -trimpath ./cmd/kandev)`
- `cd apps && pnpm --filter @kandev/web typecheck`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
