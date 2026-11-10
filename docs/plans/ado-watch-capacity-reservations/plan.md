---
created: 2026-11-06
status: completed
requirements:
  - REQ-INTEGRATIONS-ADO-WATCH-ADMISSION-001
system_design:
  - ../../specs/integrations/system-design/ado-watch-admission.md
legacy_specs: []
---

# Implementation Plan: Azure Watch Capacity Reservations

## Overview

Repair preclaimed reservation ownership at capacity admission. One focused work
order adds the failing store/coordinator regression, implements cleanup at the
capacity-refusal boundary, and verifies retry plus duplicate prevention.
The result is a focused upstream patch. Deployment is independently operated.

## Scope

In scope: preclaimed Azure work-item reservations, the maximum in-flight rejection path,
maximum in-flight two and compatibility of the shared PR-watch path.
Out of scope: backlog buffering, operator-specific selection, new UI or cap setting,
provider writes, historical ambiguous-claim deletion, watch reset and deployment.

## Technical approach

Follow [the design](../../specs/integrations/system-design/ado-watch-admission.md).
Integrations owns provider-item deduplication. The existing orchestrator throttle
owns active plus pending task admission. Release only an event's explicit preclaim when the capacity gate refuses it. Keep task-backed
reservations after successful creation even if association fails.

## Tests

AC-001.1/001.2: five preclaimed Azure matches are offered, cap two; after callbacks
settle only two task-backed claims remain; after completion the next poll creates
a previously refused item. AC-001.3: both Azure sources preserve unclaimed reservations; other providers
retain existing ownership. AC-001.4: existing throttle bypass and concurrent-burst
checks remain green.

## E2E tests

A store-to-event-to-coordinator integration regression is the end-to-end
evidence for this backend behavior. No rendered UI changes or artificial browser
flow are added. Fake Azure and a temporary SQLite database avoid provider writes.

## Work orders

- [x] [Task 01: Settle Azure preclaims at admission boundaries](task-01-admission-ownership.md)

## Verification results

The failing-before regression reproduces a missing third task after both completion
and archive. The corrected focused dispatch/source/throttle tests pass with
`go test -trimpath -race ./internal/orchestrator -run 'TestDispatchWatcherEvent_|TestAcquireWatcherSlot_|TestAzure.*WatcherSource' -count=1`.
Normal commit hooks enforce changed-package lint. This work does not authorize
a deployment or historical reservation reset.

## Risks

Do not erase another in-flight claim or a task-backed dedup row. Async event
publication cannot be treated as proof of creation. Shared source cleanup must
remain opt-in for proven preclaims. Historical leaked rows are not safely
repairable by a blanket delete. Deployment and historical reservation repair remain separate operations.
