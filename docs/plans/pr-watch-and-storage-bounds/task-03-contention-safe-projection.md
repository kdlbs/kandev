---
id: "03-contention-safe-projection"
title: "Coalesce task-status projection contention"
status: pending
wave: 1
depends_on: ["02-idempotent-polling-events"]
plan: "plan.md"
spec: "../../specs/platform/pr-watch-and-storage-bounds.md"
---

# Task 03: Coalesce task-status projection contention

## Intent

Ensure duplicate/equivalent PR signals and genuine CAS races converge without handler errors or lost final summaries.

## Acceptance

- Equivalent pending refreshes for one task are single-flight/coalesced and result in at most one effective summary persistence/publication.
- Genuine CAS loss reloads/rebases authoritative state and uses bounded exponential backoff with jitter; it does not merely raise the retry count.
- Concurrent PR-event tests preserve the correct final summary and report zero exhausted-CAS handler errors.

## Files likely touched

- `apps/backend/internal/task/statussummary/projector.go`
- `apps/backend/internal/task/statussummary/projector_events.go`
- `apps/backend/internal/task/statussummary/projector_helpers.go`
- `apps/backend/internal/task/statussummary/projector_test.go`
- `apps/backend/internal/task/statussummary/projector_pending_authority_test.go`
- New concurrent projector regression test

## Dependencies

Task 02.

## Parallelism

Sequential. It depends on the canonical PR-event semantics from task 02.

## Verification

```bash
cd apps/backend && go test -race ./internal/task/statussummary -run 'Test.*(Projector|CAS|Concurrent|PullRequest).*' -count=1 -v
cd apps/backend && go test ./internal/task/statussummary -count=1
git diff --check
```

## Output contract

Report expected red failure, contention test worker/count setup, final revision/summary assertions, and no-exhaustion result. Update task and plan status.

## Results

Pending.

