---
id: "08-crash-barrier"
title: "Provider-neutral crash barrier (R8)"
status: pending
wave: 5
depends_on:
  - "07"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-NATIVE-WORKER-008
acceptance_criteria:
  - AC-EXECUTORS-NATIVE-WORKER-008.1
  - AC-EXECUTORS-NATIVE-WORKER-008.2
  - AC-EXECUTORS-NATIVE-WORKER-008.3
system_design:
  - ../../specs/executors/system-design/remote-executor-plugin-native-workers.md
---

# Task 08: Provider-neutral crash barrier (R8)

## Summary

Add the closed set of host crash barriers and the e2e-mock-only arming route, and use them to prove the process-level interleavings of tasks 02 to 06.

## Size

S: ~150 source, ~300 test lines.

## In scope

- The eight closed barrier names (the five of the first release plus after the pending placement is persisted, before a placement poll commits, and after a cancel is marked); arming an armed name returns `conflict`; the first caller reaching an armed name fires it; releasing an unarmed barrier does nothing.
- Add the closed set of host crash barriers and the e2e-mock-only arming route, and use them to prove the process-level interleavings of tasks 02 to 06.

## Out of scope

- Vendor names; production behavior change.

## Acceptance

- Barriers are inert and unarmable unless KANDEV_E2E_MOCK is true.
- Each interleaving in the design table has a barrier test that kills the backend and recovers.

## Failing-first tests

- Arming refused outside e2e mock mode (fails: no barrier).
- Barrier fires once; block mode and terminate mode.
- Restart after termination recovers per tasks 02 to 04.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -race -trimpath ./internal/agent/runtime/lifecycle -count=1)
```

## Likely files and risks

- `internal/agent/runtime/lifecycle/crash_barrier.go`
- `internal/backendapp (route)`
- Risk: concurrency on both dialects; add the PostgreSQL race run before marking done.
