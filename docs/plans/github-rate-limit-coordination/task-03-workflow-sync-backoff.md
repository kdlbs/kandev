---
id: 03-workflow-sync-backoff
title: Workflow Sync retry persistence
status: done
wave: 3
depends_on: [02-rate-coordinator]
plan: plan.md
requirements:
  - REQ-INTEGRATIONS-GITHUB-RATE-003
system_design: ../../specs/integrations/system-design/github-rate-limit-coordination.md
---

# Task 03: Workflow Sync Retry Persistence

## Acceptance

- Transient failures persist deterministic-testable equal-jitter backoff and
  make no request before next_attempt_at.
- Invalid credentials/access and missing targets suspend automatic polling
  after one actionable stored error.
- Config save and explicit Sync now provide recovery, and schema replay is
  idempotent.

## Verification

- `cd apps/backend && go test ./internal/workflowsync -count=1`
- `cd apps/backend && go test -race ./internal/workflowsync -count=1`

## Results

- Added idempotent recovery columns and round-trip coverage for fresh,
  upgraded, and replayed Workflow Sync schemas.
- Automatic failures persist equal-jitter exponential backoff, honor provider
  retry lower bounds, and skip provider resolution before `next_attempt_at`.
- Invalid GitHub credentials/access and missing targets suspend polling after
  one attempt; skipped ticks stay silent. GitLab failures retain generic
  transient recovery.
- Config saves and explicit Sync now re-arm recovery state, while successful
  explicit sync clears failures and suspension.
- Package and race suites passed with task-local Go caches.
