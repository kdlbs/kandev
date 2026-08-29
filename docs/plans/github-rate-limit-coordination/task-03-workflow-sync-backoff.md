---
id: 03-workflow-sync-backoff
title: Workflow Sync retry persistence
status: pending
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

Pending.
