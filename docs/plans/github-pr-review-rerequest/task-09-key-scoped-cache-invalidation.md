---
id: "09-key-scoped-cache-invalidation"
title: "Key-scoped PR cache invalidation"
status: done
wave: 4
depends_on:
  - "01-backend-review-request"
plan: "plan.md"
spec: "../../specs/ui/requirements/github-pr-review-actions.md"
---

# Task 09: Key-scoped PR cache invalidation

## Acceptance

- Review re-request invalidation prevents an older fill for the affected PR
  from repopulating either feedback or status cache.
- An unrelated PR's concurrent cache fill still completes and remains cached.
- Existing cache-wide clear and repository-error invalidation race guarantees
  remain unchanged.
- Deterministic barrier tests cover same-key and unrelated-key concurrent fills.

## Ownership

- `apps/backend/internal/github/ttl_cache.go`
- `apps/backend/internal/github/ttl_cache_test.go`
- `apps/backend/internal/github/service_pr.go`
- `apps/backend/internal/github/service_request_review_test.go`

Do not convert repository-error caching or edit controllers, transports,
frontend, E2E, docs, spec, or `plan.md`.

## Output contract

Use TDD. Report RED/GREEN evidence, exact race-enabled checks, files changed,
compatibility risks, and set only this task file's status to `done`.
