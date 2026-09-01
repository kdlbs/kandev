---
id: "04-github-auth-backoff-observability"
title: "Back off failed GitHub integrations and expose health"
status: pending
wave: 2
depends_on: ["02-idempotent-polling-events", "03-contention-safe-projection"]
plan: "plan.md"
spec: "../../specs/platform/pr-watch-and-storage-bounds.md"
---

# Task 04: Back off failed GitHub integrations and expose health

## Intent

Treat permanent GitHub credential/configuration failures as degraded state, not continuous work, and expose operator-visible bounded metrics.

## Acceptance

- Authentication/configuration errors enter generation-aware exponential backoff/circuit breaking and resume after credentials/configuration changes.
- Health/status distinguishes healthy, degraded, and disabled GitHub integration without exposing secrets.
- Required canonical-watch, poll-request, CAS/event failure, queue/runtime, storage, database/WAL, and hydration-latency measures are emitted with bounded safe labels.

## Files likely touched

- `apps/backend/internal/github/errors.go`
- `apps/backend/internal/github/poller.go`
- `apps/backend/internal/github/service*.go`
- `apps/backend/internal/workflowsync/*.go`
- `apps/backend/internal/health/*.go`
- `apps/backend/internal/backendapp/*.go`
- Relevant `*_test.go` files in those packages

## Dependencies

Tasks 02 and 03.

## Parallelism

Sequential. Health measures consume the finalized event/projector semantics.

## Verification

```bash
cd apps/backend && go test ./internal/github ./internal/workflowsync ./internal/health ./internal/backendapp -run 'Test.*(Auth|Credential|Backoff|Health|Metric|Poller).*' -count=1 -v
cd apps/backend && go test ./internal/github ./internal/workflowsync ./internal/health -count=1
git diff --check
```

## Output contract

Report invalid-credential call counts before/within/after backoff, generation-change recovery, metric names/labels, and secret-redaction evidence. Update task and plan status.

## Results

Pending.

