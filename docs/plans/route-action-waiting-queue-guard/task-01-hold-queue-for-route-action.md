---
id: "01-hold-queue-for-route-action"
title: "Hold queued delivery while a dynamic route awaits action"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.15
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.16
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-02.md
---

# Task 01: Hold queued delivery while a dynamic route awaits action

## Summary

Stop the requeue loop for a queued message whose dynamic session cannot run
until the user applies a route action, and show that block through the route
recovery state.

## In scope

- `queue_dynamic_route_block.go`: the `action_required` without executor row
  predicate, the drain skip, and the post-dispatch retention that releases a
  stranded `starting` route.
- Call the drain skip from the two queue-head reservation cores and the
  retention from `finishQueuedMessageExecution`.
- Reuse the orphan-sweep route reason constant in the sweep.
- Focused regression in `dynamic_stranded_starting_route_test.go`; no new
  permanent test file.

## Out of scope

Queue transfer on an explicit primary switch, reuse selection, and the
runtime-unavailable classification for sessions without a dynamic route.

## Acceptance

- AC 001.15: a drain leaves the entry queued for an `action_required` session
  without an executor row.
- AC 001.16: a failed dispatch to a stranded `starting` route marks it
  `action_required`, keeps the session state and the entry, and blocks the next
  drain.

## Verification

```bash
cd apps/backend && go test -trimpath ./internal/orchestrator/ -run 'TestQueueDrainSkipsSessionWhoseDynamicRouteAwaitsAction|TestQueuedDispatchToStrandedStartingRouteMarksActionRequired|TestQueuedDispatchToStrandedStartingRouteReleasesDurableReservation|TestStrandedRouteReleaseHoldsRouteActionUntilEntryVisible' -count=1
```

## Results

- Both focused tests pass after the production change; they are kept in the
  existing `dynamic_stranded_starting_route_test.go`.
- `golangci-lint` and `gofmt` are clean; the documentation validator reports
  `covered`.
