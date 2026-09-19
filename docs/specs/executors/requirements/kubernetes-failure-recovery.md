---
status: draft
system: executors
created: 2026-09-19
owners:
  - kandev
---

# Kubernetes Failure Recovery

## Overview

A failed agent turn must not destroy the Kubernetes workspace that the user is
offered to resume. This document defines the recoverable agent-failure case
alongside the [Kubernetes foundation](../../kubernetes-executor/spec.md).
The executor system owns resource retention; task state remains task-owned.

## Requirements

### REQ-EXECUTORS-K8S-FAILURE-RECOVERY-001: Retained recovery resources

**Intent:** Keep an established Kubernetes session usable after an agent error.

#### Acceptance criteria

- **AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.1:** When an established Kubernetes
  session encounters a recoverable agent failure, stopping its failed execution
  shall preserve its Pod, workspace, and credentials needed for recovery.
- **AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.2:** After that failure, Resume shall
  reconnect to the retained session and preserve workspace changes, including
  after a backend restart. Cleanup shall not leave recovery referencing
  credentials that cleanup itself removed.
- **AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.3:** A delayed or repeated failure for
  an earlier execution shall not stop a newer execution or delete its resources.
- **AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.4:** Explicit archive, delete, and
  destructive cleanup shall retain their existing ownership verification and
  cleanup behavior. Existing operator-owned claims shall remain untouched.
- **AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.5:** Missing or inaccessible retained
  credentials shall remain an actionable recovery failure; the system shall not
  silently substitute credentials, adopt another workload, or recreate an empty
  workspace as if recovery succeeded.

## Exclusions

Restoring already-deleted data, changing provider error classification, changing
other executors' teardown behavior, and redesigning error presentation are outside
this contract. Fresh launch rollback remains governed by the foundation.
