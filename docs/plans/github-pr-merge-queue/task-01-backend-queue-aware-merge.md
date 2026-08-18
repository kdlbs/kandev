---
id: "01-backend-queue-aware-merge"
title: "Backend queue-aware merge"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/github-pr-merge-queue/spec.md"
---

# Task 01: Backend Queue-Aware Merge

## Inputs

- `docs/specs/github-pr-merge-queue/spec.md`
- `docs/plans/github-pr-merge-queue/plan.md`
- `apps/backend/AGENTS.md`

## Likely Files

- `apps/backend/internal/github/client.go`
- `apps/backend/internal/github/gh_client.go`
- `apps/backend/internal/github/pat_client.go`
- `apps/backend/internal/github/mock_client.go`
- `apps/backend/internal/github/noop_client.go`
- `apps/backend/internal/github/service_pr.go`
- `apps/backend/internal/github/controller.go`
- Their focused `*_test.go` files

## Acceptance

- GitHub merge requests use the asynchronous endpoint with
  `merge_action=default` and preserve allowed merge-method selection.
- Provider responses normalize to explicit `merged` or `queued` outcomes,
  including already-queued idempotency, and unknown results fail closed.
- The workspace-scoped HTTP endpoint returns the normalized outcome while
  retaining auth routing, cache invalidation, validation, and error mapping.

## Verification

```bash
make -C apps/backend test ARGS='./internal/github/...'
```

## Dependencies And Risks

- No task dependency.
- GitHub's asynchronous API response shapes and HTTP statuses must be captured
  from the official contract rather than inferred from the synchronous API.
- Both PAT and `gh` CLI transports must remain behaviorally identical.

## Results

- `make -C apps/backend test ARGS='./internal/github/...'` passed (the Make
  target executed the full backend `go test -tags fts5 ./...` suite).
