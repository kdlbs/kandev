---
created: 2026-11-23
status: done
requirements:
  - REQ-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001
system_design:
  - ../../specs/integrations/system-design/github-fork-review-start.md
legacy_specs: []
---

# Implementation plan: Fork review false positive

## Overview

Review-watch tasks for same-repository pull requests can be marked as fork
reviews by mistake. Opening such a task then fails with
`Failed to ensure session: fork pull request review requires a manual start`.
A user hit this with a same-repository PR (`isCrossRepository: false`). Edit >
Start Task worked, but opening the task always failed.

The [requirements](../../specs/integrations/requirements/github-fork-review-start.md)
now separate a failed detail read (AC-…-001.5: retry on a later poll) from a
head repository that the provider confirms is missing (AC-…-001.2: manual
start). They also add AC-…-001.6: opening a marked task must not fail. The
[system design](../../specs/integrations/system-design/github-fork-review-start.md)
describes both corrections.

## Root cause

1. `CheckReviewWatch` (`apps/backend/internal/github/service_reviews.go`)
   calls `enrichPRDetails`. That function logs a failed `GetPR` call, here
   `background provider admission deferred: background_pacing`, and keeps the
   lightweight search result. The search result has no head repository
   owner/name, and the PR event is still published.
2. `buildReviewTaskRequest` (`apps/backend/internal/orchestrator/event_handlers_github.go`)
   treats the empty head identity as a fork. It sets
   `fork_pr_requires_manual_start` on the task, and that marker never changes.
3. When the task opens in an auto-start step, `EnsureSession`
   (`apps/backend/internal/orchestrator/session_ensure.go`) chooses
   `IntentStart` with `AutoStart=true`. The central `StartTask` guard then
   returns `errForkPRManualStartRequired`, and the web client shows that error
   on every open. In prepare steps, the passive open instead created a session
   and launched workspace setup against fork-controlled files.

Reproduction: the previous assertions in `service_reviews_test.go` encoded the
bug by expecting an event after a failed or nil detail read. A temporary
orchestrator test confirmed the rest: empty head identity →
`buildReviewTaskRequest` sets the marker → `EnsureSession` with
`ActivationSource: session_open` in an auto-start step returns
`fork pull request review requires a manual start`.

## Scope

### In scope

- Skip publishing a review PR until its detail read succeeds.
- Make passive `EnsureSession` opens return no session for marked tasks in any
  workflow step, without launching workspace setup. Keep explicit automatic
  starts on the central `StartTask` rejection path.
- Add the matching TypeScript source literal.
- One regression test per work order.

### Out of scope

- Clearing or recomputing the marker on tasks created before this fix. These
  tasks become openable through task 02 and can be started manually. The
  marker is durable by design.
- Preparing a workspace for marked tasks. Preparing the workspace runs the
  setup script on fork-controlled files.
- A new fork notice or banner, which would need new localized copy.
- Changing explicit prepare requests outside passive `session_open` calls.

## Work orders

| Order | Work order | Depends on | Result |
| --- | --- | --- | --- |
| 1 | [task-01-defer-unenriched-review-prs](task-01-defer-unenriched-review-prs.md) | none | New false fork markers stop |
| 2 | [task-02-session-less-open-for-manual-start](task-02-session-less-open-for-manual-start.md) | none | Marked tasks open without an error |

The work orders are independent. Implement them in order in one session.

## Risks

- The poller retries on its normal interval. A PR whose detail read keeps
  failing gets no task until a read succeeds. This is intentional because the
  previous behavior created a task that could not auto-start.

## Implementation results

Both work orders are complete. Detail lookup failures and nil responses now
defer review task creation until a later successful poll. Marked review tasks
open without a session or workspace preparation for passive opens in prepare
and auto-start steps, and remain available for an explicit manual start.

## Verification

```bash
go -C apps/backend test -trimpath ./internal/github -run 'TestCheckReviewWatch' -count=1
go -C apps/backend test -trimpath ./internal/orchestrator -run 'TestAutoStart_ForkReview|TestEnsureSession_ForkReview' -count=1
go -C apps/backend test -trimpath ./internal/orchestrator -run 'TestReviewWatchSearchResultIdentityControlsAutoStart' -count=1
go -C apps/backend test -trimpath ./internal/github ./internal/orchestrator -count=1
bash scripts/lint-go-changed apps/backend/internal/github/service_reviews.go apps/backend/internal/github/service_reviews_test.go apps/backend/internal/github/enterprise_watch_fences_test.go apps/backend/internal/orchestrator/session_ensure.go apps/backend/internal/orchestrator/session_ensure_fork_review_test.go apps/backend/internal/orchestrator/event_handlers_github_review_test.go
(cd apps/backend && go build -trimpath ./cmd/kandev)
(cd apps && pnpm --filter @kandev/web typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

All listed commands passed, including the prepare-step passive-open regression
and the explicit `AutoStart: false` session-open case.
