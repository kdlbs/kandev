---
id: "10-admission-context"
title: "Launch and resume admission context (M2)"
status: pending
wave: 6
depends_on:
  - "08"
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
  - AC-EXECUTORS-PLUGIN-ADMISSION-001.10
  - AC-EXECUTORS-PLUGIN-ADMISSION-001.11
  - AC-EXECUTORS-PLUGIN-ADMISSION-001.12
  - AC-EXECUTORS-PLUGIN-ADMISSION-001.13
system_design:
  - ../../specs/executors/system-design/remote-executor-plugin-admission.md
---

# Task 10: Launch and resume admission context (M2)

## Summary

Add the admission message on provision and recover, the host `AdmissionHook` with an atomic reservation, and enforcement at first launch and resume. The continuation of a pending placement is not an admission: it keeps the reservation and re-runs only the capability gate.

## Size

L: ~500 source, ~900 test lines.

## In scope

- `admitLaunch` order: capability gate, `profile_too_large` measure, `Admit` outside any transaction with a 5 s deadline, then one transaction that takes the scope lock, counts, and writes the reservation with the operation; the hook never counts. A resume never takes the scope lock and keeps its reservation; only the new launch can get `admission_denied` (`scope_full`), in either order (AC 001.9). Test enumerates every caller of both entries. Resume follows the table of AC 001.11 (deny refuses, unavailable goes ahead, no reservation resumes by the same rules).
- `admission.go` hook interface and default allow-all; one shared gate function called from both entries, `validateLaunch` (first launch) and `attachRetainedPluginExecutor` (resume, which runs before `validateLaunch` on main).
- Reservation written with the operation record; exactly-once release; PostgreSQL advisory lock on the admission scope.
- Identity snapshot of seven fields in the envelope, carried unchanged to recovery and resume (AC 001.12). Resource classes, labels and provider-reported values are cut to card 11c92feb-2b4c-4d05-8318-9aff24e2fa10.

## Out of scope

- Quota policy itself, resizing a live environment, per-person quotas.

## Acceptance

- Two launches for the last unit of a scope admit one; hook failure fails closed with no allocation.
- A resume racing a new launch of the same scope gives the same outcome in either order.

## Failing-first tests

- Two launches compete for the last unit (sqlite and PostgreSQL race).
- Admission hook races a resume of the same scope: serialized, loser sees the reservation.
- Hook unavailable: first launch `admission_unavailable`, no provider call; resume goes ahead.
- Resume through `attachRetainedPluginExecutor`: hook deny refuses, no reservation resumes, capability gate and measure run.
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
