---
id: "08-reviewer-input-hardening"
title: "Reviewer input hardening"
status: done
wave: 4
depends_on:
  - "01-backend-review-request"
plan: "plan.md"
spec: "../../specs/ui/requirements/github-pr-review-actions.md"
---

# Task 08: Reviewer input hardening

## Acceptance

- The HTTP boundary caps request-body bytes, reviewer count, and individual
  login length before GitHub client delegation.
- Reviewer logins are trimmed and deduplicated case-insensitively while
  preserving first-seen display casing and order.
- Empty-after-normalization, oversized, and over-count payloads return 400
  without invoking the client.
- Controller tests cover missing configuration and upstream rejection responses.

## Ownership

- `apps/backend/internal/github/controller.go`
- `apps/backend/internal/github/controller_test.go`
- New narrowly scoped controller tests if preferable.

Do not change GitHub transport, cache, frontend, or docs files.

## Output contract

Use TDD. Report RED/GREEN evidence, exact checks, files changed, chosen limits
and rationale, residual risks, and set only this task file's status to `done`.
