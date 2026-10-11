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

- **AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.6:** On Kubernetes Resume, the
  resumed process shall receive current environment definitions from the profile
  recorded for its retained runtime, even if the session's selected profile has
  changed. Subsequent process starts in that resumed session shall retain that
  profile identity. Environment edits shall not change retained Pod or storage
  settings.
- **AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.7:** A deleted recorded profile shall
  permit recovery without its unavailable environment definitions. Other profile
  lookup failures or a profile owned by another executor shall block Resume.
- **AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.8:** Profile secret references shall
  remain unresolved until the existing launch checkpoint; recovery shall retain
  the existing environment precedence and secret-resolution rules.

- **AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.9:** Cleanup immediately after an
  agent-container restart shall authenticate against the exact retained Pod,
  recover its changed control token, and finish without waiting for a status poll.
  An authenticated already-absent remote instance shall count as successful stop.
- **AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.10:** Concurrent stop, attachment, and
  refresh for sibling sessions shall share recovered environment credentials;
  they shall not consume the same bootstrap handshake independently or restore
  an older token over the recovered token.
- **AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.11:** When authentication, identity
  verification, remote deletion, or credential persistence fails, cleanup shall
  retain the exact execution's repairable ownership and credential recovery state.
  Retry, including after backend restart, shall not require a database edit.
- **AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.12:** Successful cleanup shall close
  only the stopped execution's local connections. Failed cleanup shall preserve
  the recovery information needed to rebuild broken connections; it shall not
  disable shared recovery or disrupt live siblings.
- **AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.13:** Resume after container restart
  shall retain the same session, native conversation identity, and workspace
  edits. It shall start at most one replacement execution and continue through
  the normal queued-prompt path without replaying the original task prompt.

## Exclusions

Restoring already-deleted data, changing provider error classification, changing
other executors' teardown behavior, and redesigning error presentation are outside
this contract. Fresh launch rollback remains governed by the foundation.
