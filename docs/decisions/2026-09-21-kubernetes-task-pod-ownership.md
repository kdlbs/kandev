# ADR-2026-09-21-kubernetes-task-pod-ownership: Task-owned Kubernetes compute

**Status:** accepted
**Date:** 2026-09-21
**Area:** backend, infra

## Context

The user requires one Kubernetes pod per task with multiple agent sessions.
The foundation intentionally allocated one pod per lifecycle/session instance.
Simply changing resource names would leave session cleanup and authentication
owning shared resources, and would race concurrent creation and recovery.

## Decision

Make the canonical task environment the durable owner of Kubernetes compute.
Sessions own independent agentctl instances inside that pod. Persist physical
resource inventory and encrypted control credentials independently of session
execution rows; task cleanup owns physical deletion.

This decision supersedes only the session-scoped ownership choice in
[the original ownership ADR](2026-08-24-kubernetes-executor-resource-ownership.md)
for newly shared task resources. Exact UID, full ownership validation, admission,
create-nonce continuity, launch snapshots, and external-PVC protection remain.
Legacy ambiguous multi-pod tasks are never automatically consolidated.
Implementation follows the [design](../specs/executors/system-design/kubernetes-task-pod.md); validation evidence is recorded in the [plan](../plans/kubernetes-task-pod/plan.md).

## Consequences

Additional sessions share workspace data and compute without duplicate pods.
Pod failure affects all attached sessions. Durable environment inventory and
creation/deletion exclusion are required even when no sessions remain.
Compatibility must preserve legacy independent workspaces and recorded identities.

## Alternatives Considered

- Keep per-session pods: violates the requested compute and workspace contract.
- Change names to task IDs only: leaves races, per-session cleanup, and control
  authentication unresolved.
- Reuse an arbitrary sibling execution: loses ownership when that session is
  deleted and cannot safely choose among legacy independent workspaces.
