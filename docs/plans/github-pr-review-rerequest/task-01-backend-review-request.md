---
id: "01-backend-review-request"
title: "Backend review-request contract"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/github-pr-review-actions.md"
---

# Task 01: Backend review-request contract

## Acceptance

- Every GitHub client implementation satisfies a typed reviewer-request
  method; gh/PAT send the official endpoint and JSON reviewer list.
- The Kandev route validates input, delegates one request, and returns
  `{"requested": true}` on success.
- Success evicts only the affected PR feedback/status cache key, and the mock
  makes the requested reviewer visible on the next feedback fetch.

## Verification

```bash
cd apps/backend
go test ./internal/github -run 'RequestReview'
go test ./internal/github
```

## Files likely touched

- `apps/backend/internal/github/client.go`
- `apps/backend/internal/github/gh_client.go`
- `apps/backend/internal/github/gh_client_test.go`
- `apps/backend/internal/github/pat_client.go`
- `apps/backend/internal/github/pat_client_test.go`
- `apps/backend/internal/github/mock_client.go`
- `apps/backend/internal/github/mock_client_test.go`
- `apps/backend/internal/github/noop_client.go`
- `apps/backend/internal/github/noop_client_test.go`
- `apps/backend/internal/github/service_pr.go`
- `apps/backend/internal/github/service_request_review_test.go`
- `apps/backend/internal/github/controller.go`
- `apps/backend/internal/github/controller_test.go`

## Dependencies

None.

## Inputs

- Spec API surface, permissions, failure modes, and first five scenarios.
- Existing `SubmitReview` client/service/controller path.
- Official GitHub request-reviewers endpoint.

## Output contract

Report summary, files changed, RED/GREEN/REFACTOR evidence, commands/results,
blockers, risks, and set only this task file's status to `done`. Do not edit
`plan.md`.
