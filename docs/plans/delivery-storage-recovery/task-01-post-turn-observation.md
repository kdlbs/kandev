---
id: "01-post-turn-observation"
title: "Observe delivery health between turns"
status: complete
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002
acceptance_criteria:
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.1
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.4
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.5
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.7
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.15
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.17
system_design:
  - ../../specs/platform/system-design/delivery-storage-diagnostics.md
---

# Task 01: Observe delivery health between turns

## Summary

Keep the existing delivery-health monitor useful after completion clears its active submission.
An idle observation updates runtime pressure without granting prompt or settlement authority.

## In scope

- Remove the empty-submission prerequisite for status I/O in `probeDeliveryHealth`.
- Keep active correlated failure recovery and its cooldown unchanged.
- Accept idle pressure only after execution, client, attachment, generation, and submission revalidation.
- Recognize supported healthy/nonblocking states. Missing, unknown, and malformed states supply no clearance evidence.
- Retain the existing interval, deadline, single request, lifecycle ownership, and cancellation behavior.

## Out of scope

Freshness fields, rendered UI, state settlement, journal admission, native restart, queue drain, and new polling loops.

## Acceptance

1. RED: `TestDeliveryHealthSettledOwnerRecoversWithoutPrompt` reports stale pressure before the correction. GREEN: a supported healthy idle status clears only that pressure.
2. Idle probes never reconcile an empty or remembered submission, mutate a recovery block, or dispatch work. Active correlated failures retain their existing path.
3. Late idle responses cannot affect a successor prompt, attachment, client, execution, or harness generation. Stop and shutdown cancel and join the same observer.

## Verification

Run from the repository root, sequentially:

```bash
(cd apps/backend && GOMEMLIMIT=2048MiB GOMAXPROCS=2 go test -p 1 -race -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -run 'TestDeliveryHealth|TestDeliveryStorage' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/delivery_health_monitor.go apps/backend/internal/agent/runtime/lifecycle/delivery_health_monitor_test.go
git diff --check
```

Use `/tdd` and the backend test guidance.
Cover unchanged blocked health, confirmed recovery, unavailable status, unsupported peer, and historical failure preservation.
Use fake time and channels for active-to-idle and idle-to-active races.
Do not use a live filesystem shortage or native provider account.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/delivery_health_monitor.go`.
- `apps/backend/internal/agent/runtime/lifecycle/delivery_health_monitor_test.go`.
- A new `delivery_health_idle_test.go` in the same package if test-file limits require it.

Include every additional changed Go file in changed-file lint.

## Dependencies

None.

## Risks

- An empty submission is an observation scope, not a canonical submission identity.
- A missing health object currently resembles default zero values. Those values cannot prove recovery.
- A new prompt can start during status I/O on the same execution.

## Parallelism

`sequential`

## Inputs

- [Storage requirements](../../specs/platform/requirements/delivery-storage-diagnostics.md), requirement 002.
- [Post-turn design](../../specs/platform/system-design/delivery-storage-diagnostics.md#post-turn-observation-and-freshness).
- Existing `deliveryHealthOwner`, `deliveryStreamAttachment`, and `TestDeliveryHealthStaleOwnerAndSiblingIsolation` patterns.
- [Plan evidence](plan.md#evidence-and-root-causes).

## Results

Implemented post-settlement observation in `delivery_health_monitor.go`.
Idle probes revalidate the captured execution, client, attachment, harness generation, prompt generation, and submission, then update pressure only.
Active correlated failures retain the existing reconciliation and cooldown path.
Supported healthy and nonblocking states clear live pressure; missing or unknown states supply no clearance evidence.
Tests cover confined recovery without a prompt, successor rejection, unsupported status, and owner stop.
Review follow-up carries the captured submission, prompt generation, and client lease into the pressure-cache commit, so a successor that starts while the cache mutex is contended cannot receive the predecessor's idle result. The cache regression mutates each identity after the monitor precheck and asserts the old observation is rejected without changing pressure values or receipt age.
The targeted lifecycle race tests and changed-Go lint passed.
