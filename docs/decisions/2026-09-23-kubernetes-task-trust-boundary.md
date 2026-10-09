# ADR-2026-09-23-kubernetes-task-trust-boundary: Shared task credential trust

**Status:** accepted
**Date:** 2026-09-23
**Area:** backend, infra

## Context

[Task-owned Kubernetes compute](2026-09-21-kubernetes-task-pod-ownership.md)
places independent agent sessions in one container under one OS user. Separate
HOME paths and private file permissions prevent accidental configuration mixing,
but sibling processes can read each other's credentials. A security review
required an explicit boundary before supporting different credential bindings.
The user selected shared task trust on 2026-09-23.

## Decision

Treat every session in a task as mutually trusted. Allow different agent profiles
and credentials in the shared pod. Per-session environment and auth preparation
must preserve each session's launch settings, without claiming security isolation
from other sessions. Operators own agent trust and provider-side credential
revocation. Document this boundary in the public Kubernetes executor guide.

## Consequences

A compromised agent can expose sibling credentials. Operators must stop the task's
agents and revoke or rotate exposed credentials with their providers. Stopping or
deleting one session cannot undo exposure. Agents requiring separate trust must
use separate tasks with appropriate executor isolation policies.

Regression tests verify that different profiles and credentials share one pod
while retaining distinct launch configuration. They do not claim same-UID access
is prevented. The [requirements](../specs/executors/requirements/kubernetes-task-pod.md)
and [design](../specs/executors/system-design/kubernetes-task-pod.md) define this contract.

## Alternatives Considered

- Reject differing credential bindings: limits multi-profile tasks and does not
  isolate other same-UID secrets. The user chose to permit differing bindings.
- Separate OS identities or a credential broker: changes the executor security
  architecture and operational contract; not selected for shared task trust.
