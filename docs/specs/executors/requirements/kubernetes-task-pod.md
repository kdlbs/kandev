---
status: active
system: executors
created: 2026-09-21
owners:
  - kandev
---

# Kubernetes task pod requirements

## Overview

A task's additional Kubernetes sessions share its pod and workspace while
retaining independent agent conversations. Executors owns this contract because
it governs remote compute ownership. The task system continues to own canonical
workspace selection and session identity.

This contract replaces the session-per-pod rule and concurrent-sharing
exclusion in the [foundation](../../kubernetes-executor/spec.md).
It implements the user's explicit one-pod-per-task expectation.

## Requirements

### REQ-EXECUTORS-KUBERNETES-TASK-POD-001: Task-owned Kubernetes compute

**Intent:** Starting another agent session must not allocate another task pod.

#### Acceptance criteria

- **AC-EXECUTORS-KUBERNETES-TASK-POD-001.1:** When an additional session starts in a task with a ready
  Kubernetes environment, it shall use the same pod UID and workspace, with an
  independent agent instance and conversation. Session attachment shall preserve
  tracked and untracked files without rerunning workspace preparation.
- **AC-EXECUTORS-KUBERNETES-TASK-POD-001.2:** When sessions start concurrently, they shall not allocate
  duplicate task pods or managed workspace claims. An environment still being
  prepared shall return the existing recoverable preparation error.
- **AC-EXECUTORS-KUBERNETES-TASK-POD-001.3:** Stopping, deleting, or failing one session shall not interrupt
  a sibling's agent, pod, or workspace. A task with no remaining sessions shall
  retain its environment until task-level cleanup authorizes removal.
- **AC-EXECUTORS-KUBERNETES-TASK-POD-001.4:** Backend restart and session resume shall recover the shared
  recorded pod. Missing-pod recovery shall create at most one replacement for
  the task, preserve managed workspace data, and retain separate session state.
  Loss of ephemeral workspace data shall follow the existing explicit recovery
  failure contract, never silently become a fresh workspace.
- **AC-EXECUTORS-KUBERNETES-TASK-POD-001.5:** Final task resource cleanup shall remove only the exactly
  recorded owned pod and managed claim, after preventing concurrent attachment.
  Existing administrator-owned claims shall never be deleted.
- **AC-EXECUTORS-KUBERNETES-TASK-POD-001.6:** Incompatible executor selection, ambiguous legacy pod inventory,
  or ownership mismatch shall fail attachment with a recoverable error without
  creating another pod, adopting foreign resources, or deleting existing data.
  Existing legacy sessions shall remain resumable under their recorded identity;
  independent legacy workspaces shall not be merged automatically.
- **AC-EXECUTORS-KUBERNETES-TASK-POD-001.7:** Each authorized session's Kubernetes status shall identify its
  actual shared pod on desktop and mobile. Profile edits shall not mutate the
  retained task's workload; additional sessions shall use its recorded workload
  configuration while keeping independent agent-profile runtime settings.

- **AC-EXECUTORS-KUBERNETES-TASK-POD-001.8:** The task shall be one credential trust boundary.
  Sessions may use different agent profiles and credentials in the same pod.
  Per-session settings shall prevent accidental credential inheritance or overwrite,
  but shall not promise protection from sibling processes running as the same OS user.
  Operators shall trust every agent attached to a task and manage provider-side
  credential revocation after suspected exposure.

- **AC-EXECUTORS-KUBERNETES-TASK-POD-001.9:** With managed task Git access selected,
  each fresh or resumed agent session shall be able to read and fetch its authorized
  private repositories from its actual command subprocesses. Successful preparation
  alone is insufficient. Retained-pod Stop/Resume shall preserve pod and workspace
  identity while delivering the current session's credentials.
- **AC-EXECUTORS-KUBERNETES-TASK-POD-001.10:** Credential replacement, revocation,
  logout, or a switch away from managed access shall not retain stale managed
  credentials or generated credential routing in subsequently started processes.
  Managed access shall preserve repository-scoped redemption and verified TLS,
  without placing raw Git tokens in the agent environment, profile, or image.

## Implementation plans

- [Managed Git handoff repair](../../../plans/kubernetes-managed-git-handoff/plan.md).

## Related contracts

- [Additional-session workspace reuse](../../tasks/requirements/additional-session-workspace-reuse.md).
- [Kubernetes retained compute](kubernetes-retained-compute.md).

## Out of scope

Cross-task pod pooling, changing workspace inheritance/group semantics,
automatically consolidating legacy pods, concurrent-edit conflict resolution,
and redesigning executor UI controls.
