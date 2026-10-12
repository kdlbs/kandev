---
id: "01-shutdown-settlement"
title: "Shutdown settlement"
status: in_progress
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-002
acceptance_criteria:
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.1
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.2
system_design:
  - ../../specs/platform/system-design/session-recovery-convergence.md
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
Focused producer race checks pass, including deterministic rejection, cancelled
RequestError, retained error settlement and queued terminal draining. Shutdown
intent is independent of update-forwarder termination. CI remains authoritative.

[Parent plan](plan.md). [Requirements](../../specs/platform/requirements/durable-restart-reconciliation.md).
