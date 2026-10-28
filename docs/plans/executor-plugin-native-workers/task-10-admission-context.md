---
id: "10-admission-context"
title: "Launch and resume admission context (M2)"
status: pending
wave: 6
depends_on:
  - "09"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PLUGIN-ADMISSION-001
acceptance_criteria:
  - AC-EXECUTORS-PLUGIN-ADMISSION-001.1
  - AC-EXECUTORS-PLUGIN-ADMISSION-001.2
  - AC-EXECUTORS-PLUGIN-ADMISSION-001.3
  - AC-EXECUTORS-PLUGIN-ADMISSION-001.4
  - AC-EXECUTORS-PLUGIN-ADMISSION-001.5
  - AC-EXECUTORS-PLUGIN-ADMISSION-001.6
  - AC-EXECUTORS-PLUGIN-ADMISSION-001.7
  - AC-EXECUTORS-PLUGIN-ADMISSION-001.8
  - AC-EXECUTORS-PLUGIN-ADMISSION-001.9
system_design:
  - ../../specs/executors/system-design/remote-executor-plugin-shared-admission-placement.md
---

# Task 10: Launch and resume admission context (M2)

## Summary

Add the admission message on provision and recover, the host `AdmissionHook` with an atomic reservation, the applied resource class, and enforcement at first launch, delayed launch, and resume.

## Size

L: ~500 source, ~900 test lines.

## In scope

- `admitLaunch` order: capability gate, label validation, `Admit` outside any transaction with a 5 s deadline, then one transaction that takes the scope lock, counts, and writes the reservation with the operation; the hook never counts. A resume keeps its reservation and takes none; the new launch that no longer fits gets `admission_denied` (`scope_full`). Test enumerates every caller of `validateLaunch`.
- `admission.go` hook interface and default allow-all; one shared call function used by all three launch paths.
- Reservation written with the operation record; exactly-once release; PostgreSQL advisory lock on the admission scope.
- Label validation; applied-class check; resume keeps its class.

## Out of scope

- Quota policy itself, resizing a live environment, per-person quotas.

## Acceptance

- Two launches for the last unit of a scope admit one; hook failure fails closed with no allocation.
- A resume that races an admission request never changes the applied class.

## Failing-first tests

- Two launches compete for the last unit (sqlite and PostgreSQL race).
- Admission hook races a resume of the same scope: serialized, loser sees the reservation.
- Hook unavailable: `admission_unavailable`, no provider call.
- Reservation released exactly once after failure, destroy, and cancel.
- Call-site test fails if a launch path skips the hook; the request builder cannot read names or credentials.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -race -trimpath ./internal/plugins/... ./cmd/plugin-fixture -count=1)
```

```bash
(cd apps/backend && go test -race -trimpath ./internal/agent/runtime/lifecycle -count=1)
```

## Likely files and risks

- `apps/backend/internal/agent/runtime/lifecycle/admission.go (new)`
- `apps/backend/internal/agent/runtime/lifecycle/executor_plugin.go`
- `apps/backend/proto/kandev/plugin/v1/plugin.proto`
- Risk: concurrency on both dialects; add the PostgreSQL race run (`KANDEV_TEST_POSTGRES_DSN`) before marking done.
