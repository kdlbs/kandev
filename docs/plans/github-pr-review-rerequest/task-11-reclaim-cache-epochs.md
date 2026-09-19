---
id: "11-reclaim-cache-epochs"
title: "Reclaim key invalidation metadata"
status: done
wave: 5
depends_on:
  - "09-key-scoped-cache-invalidation"
plan: "plan.md"
spec: "../../specs/ui/requirements/github-pr-review-actions.md"
---

# Task 11: Reclaim Key Invalidation Metadata

## Acceptance

- Per-key invalidation still prevents affected stale fills and preserves
  unrelated fills.
- Version metadata is retained only while an in-flight call can still write
  under that key; completed/high-cardinality invalidations do not grow an
  unbounded map.
- Cache-wide `clear()` and repository-error `del()` semantics remain unchanged.
- Deterministic race tests and a high-cardinality reclamation test cover the
  lifecycle.

## Ownership

- `apps/backend/internal/github/ttl_cache.go`
- `apps/backend/internal/github/ttl_cache_test.go`
- `apps/backend/internal/github/service_request_review_test.go` only if needed.
- Task 11 status only.

Do not edit service/controller/client/frontend/E2E/docs/spec/`plan.md`.

## Output contract

Use TDD, avoid sleeps, run race-enabled tests, report exact checks and
compatibility risk, and set only this task file to `done`.
