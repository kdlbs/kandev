---
status: in_progress
---

# Task 01: Shutdown settlement

## Outcome and scope

Publish original-child interruption before adapter Close, persist bounded
settlement after cancellation, preserve genuine deterministic failures and
retained terminals. Own process/submission_delivery.go, manager.go and sibling
tests. No protocol replacement or user prompt replay.

## Contract

REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-002, criteria .1 and .2.
[Design](../../specs/platform/system-design/session-recovery-convergence.md).

## Acceptance

- Real Stop while a background prompt is active records uncertainty without terminal invention.
- Genuine provider failure and retained complete/failed/cancelled evidence survive teardown.
- Cancellation and child replacement cannot leave replayable or cross-child settlement.

## Check

From apps/backend:
`GOMEMLIMIT=768MiB GOMAXPROCS=2 go test -p 1 -race -trimpath -tags fts5 ./internal/agentctl/server/process -run 'TestSubmission|TestStop' -count=1`

## Results

Real graceful-stop regression confirmed red before production correction.
Focused TestSubmission race suite passed after initial correction; controls pending.

[Parent plan](plan.md). [Requirements](../../specs/platform/requirements/durable-restart-reconciliation.md).
