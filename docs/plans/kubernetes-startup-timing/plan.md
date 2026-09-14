---
created: 2026-09-09
status: implemented
requirements:
  - REQ-EXECUTORS-K8S-TIMING-001
  - REQ-EXECUTORS-K8S-TIMING-002
system_design:
  - ../../specs/executors/system-design/kubernetes-startup-timing.md
legacy_specs:
  - ../../specs/kubernetes-executor/spec.md
---

# Implementation Plan: Kubernetes Startup Timing

## Overview

Expose fresh environment stage durations through bounded structured logs.
This package shares the first delivery group with
[worker presets](../kubernetes-worker-presets/plan.md). Either can land first.
Timing data informs future optimization without changing the executor architecture.

## Scope

### In scope

- Per-attempt storage, Pod-readiness, bootstrap, connection, and total durations.
- Explicit failure/cancellation outcomes, correlation, privacy tests, and diagnostic documentation.

### Out of scope

- A metrics registry, new UI stages, historical storage, reconnect measurements, or runtime policy changes.
- First-response latency claims and per-image-pull attribution.

## Technical approach

Add a Kubernetes-local recorder near `KubernetesExecutor.createFresh`.
Use the existing Zap logger and monotonic elapsed time. Bind records to one
attempt ID and allowlisted lifecycle identity fields.
Keep all cluster calls, return errors, checkpoint ordering, and rollback ownership unchanged.
The summary records final return after rollback, not full manager/agent readiness.

## Tests

`executor_kubernetes_timing_test.go` contains these proposed tests:

- `TestKubernetesLaunchTimingSuccess`: TIMING-001.1, 001.2, 002.3.
- `TestKubernetesLaunchTimingFailures`: TIMING-001.3, 002.1.
- `TestKubernetesLaunchTimingFinalization`: TIMING-001.2, 002.1.
- `TestKubernetesLaunchTimingSecrets`: TIMING-002.2.
- `TestKubernetesLaunchTimingConcurrentAttempts`: TIMING-002.3.
- `TestKubernetesLaunchTimingReconnectExcluded`: TIMING-001.4, 002.1.

IDs use the `AC-EXECUTORS-K8S-` prefix. Test fake-client action counts, bounded
record counts, and original errors through the production launch path.

## E2E tests

Extend the existing kubeconfig launch case in
`tests/kubernetes/kubernetes-executor.spec.ts`, project `containers`.
Read its isolated backend file log with Info logging enabled.
Assert one correlated total and the four successful stages for its fresh instance.
The scenario still verifies a working task. It sets no duration threshold.
No UI change requires new mobile tests.

## Work orders

- [x] [Task 01: Record Kubernetes launch durations](task-01-launch-durations.md)

## Verification results

Implemented. Unit timing coverage and the real Kind kubeconfig launch both
produce one correlated summary and four stage records without changing launch
errors or adding a duration threshold.

## Risks

- Labeling this total as agent readiness creates misleading performance claims.
- Deferred rollback can change the final error. The summary must observe that result.
- Info-level stdout filtering differs from backend file logging. The test must read the correct log sink.
- A process crash can prevent the final record. The implementation cannot claim crash-durable timings.
