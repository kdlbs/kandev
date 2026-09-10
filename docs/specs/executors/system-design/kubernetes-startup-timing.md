---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-K8S-TIMING-001
  - REQ-EXECUTORS-K8S-TIMING-002
---

# Kubernetes Startup Timing System Design

## Purpose and boundaries

The [requirements](../requirements/kubernetes-startup-timing.md) cover only
`KubernetesExecutor.createFresh` in `internal/agent/runtime/lifecycle`.
`KubernetesPreparer.Prepare` currently emits a short progress step before actual
resource creation. Its duration is not an environment-start measurement.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| REQ-EXECUTORS-K8S-TIMING-001 | Measurement contract |
| REQ-EXECUTORS-K8S-TIMING-002 | Passive collection and failures |

## Measurement contract

One local recorder belongs to one `createFresh` call. A new opaque attempt ID
distinguishes retries that reuse a session or instance ID. Existing task, session,
and instance IDs provide correlation with lifecycle evidence.

| Stage | Start | End |
| --- | --- | --- |
| `storage` | Before `launch.provisionWorkspace` | After workspace verification/provisioning and its checkpoint |
| `pod_ready` | Before `launch.createRunningPod` | After admitted main-container-running observation and checkpoint |
| `bootstrap` | Before nonce generation and binary resolution | After `bootstrapPod` returns |
| `agentctl_connect` | Before `connectNewAgentctl` | After its health/nonce handshake and forward setup return |

`pod_ready` includes scheduling, image pulls, init work, and storage binding waits
that occur inside that call. It does not measure those substeps independently.
`storage` can complete before a deferred-binding PVC becomes Bound.

The total begins at `createFresh` entry and ends at its final return.
It includes `launch.complete`, bookkeeping, and failure rollback.
Stage sums therefore need not equal the total. Queue wait, instance-lock wait,
client construction, manager secret persistence, and agent-process startup are outside this total.
The summary means environment-call completion, not durable session readiness.

Proposed Info-level records are `kubernetes.launch.stage` and
`kubernetes.launch.completed`. Both use `attempt_id`, `instance_id`, `task_id`,
`session_id`, `mode: fresh`, `duration_ms`, and `outcome`.
Stage records add `stage`. The summary adds `failed_stage` only when an entered
stage failed. A post-stage `launch.complete` failure uses `failure_site: finalize`.
An identity/setup failure before stages uses `failure_site: initialize`.
These fields are a bounded diagnostic vocabulary, not a public API.

Outcomes are `success`, `error`, `canceled`, and `timeout`.
Context outcomes use `errors.Is` on the returned error.
A later context cancellation does not relabel a successful result.
No record contains `err.Error()` or serialized request metadata.

## Passive collection and failures

Use the existing logger and Go monotonic durations (`time.Now`/`time.Since`).
A narrow clock seam or `testing/synctest` supports deterministic duration tests.
The helper stays local to the Kubernetes lifecycle package. No shared executor
interface, global registry, timer goroutine, or telemetry queue is needed.

A stage emits once on return, including errors. Skipped stages emit nothing.
The final summary defer is registered before rollback so it observes the final
returned error after rollback. It preserves the original failed-stage result.
Rollback errors can change the summary outcome but do not overwrite stage evidence.
The helper never intercepts or replaces the original return error.

Existing reconnect and replacement paths bypass this recorder.
Their successful behavior does not emit a misleading fresh-start measurement.
Each attempt emits at most four stage records and one summary.
Existing log filtering and retention govern storage. Info records are available
in backend file logs under normal configuration, not necessarily `kubectl logs`.

## Verification

`executor_kubernetes_timing_test.go` drives real fresh-launch wiring with the
existing fake clients and a Zap observer. Assertions cover phase order, bounded
record counts, concurrent attempts, and failures at every stage and finalization.
Sentinel credentials, nonces, scripts, and hostile error text must not appear.
Fake-client action counts and existing lifecycle tests prove no new API operations.
Reconnect and missing-Pod replacement must emit no `mode: fresh` records.

The existing Kind launch scenario supplies a real backend-log smoke assertion.
It checks correlation and entered-stage completeness without timing thresholds.
Performance comparisons remain observations on a named machine and image.

## Persistence and related contracts

No timing fields enter `executors_running`, task state, or browser payloads.
No schema migration or new runtime configuration is required.
The [resource ownership ADR](../../../decisions/2026-08-24-kubernetes-executor-resource-ownership.md)
continues to govern checkpoint and rollback behavior.

## Delivery

See the [startup timing plan](../../../plans/kubernetes-startup-timing/plan.md).
