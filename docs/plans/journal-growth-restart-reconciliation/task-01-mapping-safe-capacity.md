---
id: "01-mapping-safe-capacity"
title: "Make capacity accounting safe during mmap growth"
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.4
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
  - ../../specs/platform/system-design/durable-restart-reconciliation.md
---

# Task 01: Make capacity accounting safe during mmap growth

## Summary

Capture page size under exclusive handle initialization before journal operations can run. Remove runtime DB.Info reads and refresh the immutable value whenever Compact reopens a handle, including rename-failure recovery. Keep synchronized free-page Stats and existing reserve/pending-write accounting.

## In scope

- Real mmap growth under concurrent pressure inspection and event/submission writes
- Close, successful compaction, failed-swap reopen, free-page reuse, and reserve tests

## Out of scope

- New replay, recovery, schema, or provider behavior

## Acceptance

- A race-enabled subprocess regression reproduces the old fault or data race and completes without either after the correction. Force actual mapping growth; use a bounded child process so a regression cannot kill the test runner.
- All open/reopen paths use valid page-size metadata and preserve reserve/overflow/reuse behavior.

## Verification

```bash
(cd apps/backend && go test -trimpath -race ./internal/agentctl/journal -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/agentctl/journal/journal.go`
- `apps/backend/internal/agentctl/journal/disk_capacity.go`
- `apps/backend/internal/agentctl/journal/disk_capacity_test.go`
- `apps/backend/internal/agentctl/journal/journal_shutdown_test.go`

## Dependencies

None.

## Risks

Initializing the cache after a concurrent writer starts would preserve the race; do not treat the journal lifetime lock as bbolt mmap exclusion.

## Parallelism

`sequential`. No delegation is authorized.

## Inputs

- Linked platform requirements and system designs.
- Plan evidence and existing test patterns.
- Existing journal shutdown, native retirement, and silent restore packages.

## Results

Regression failed with a bbolt Info/munmap data race before the fix. `go test -trimpath -race ./internal/agentctl/journal -count=1` passed after caching page size on open and both compaction reopen paths.
