---
status: active
system: executors
created: 2026-09-09
owners:
  - kandev
---

# Kubernetes Retained Compute Visibility Requirements

## Overview

An operator must distinguish a stopped task session from its retained Kubernetes
Pod. The executor system owns that projection. Task state and task-access policy
remain with the task system.

## Terminology

- **Retained Pod:** The exact recorded Pod still exists after a session leaves active execution.
- **Resource request:** A configured main-container CPU or memory request, not measured use or a billing amount.
- **Session state:** Kandev's recorded session lifecycle state, separate from Pod phase and container state.

## Requirements

### REQ-EXECUTORS-K8S-RETAINED-001: Truthful resource status

**Intent:** Users can recognize retained resources without confusing an idle agent
with a stopped session or a failed status lookup.

#### Acceptance criteria

- **AC-EXECUTORS-K8S-RETAINED-001.1:** Authorized session rows shall show session
  state separately from Pod phase and container state.
- **AC-EXECUTORS-K8S-RETAINED-001.2:** A canceled session with an existing,
  verified, non-terminal Pod shall show that its Pod remains retained after Stop.
- **AC-EXECUTORS-K8S-RETAINED-001.3:** Missing, terminating, terminal, inaccessible,
  or identity-mismatched Pods shall not appear as confirmed retained compute.
- **AC-EXECUTORS-K8S-RETAINED-001.4:** Verified rows shall show the main container's
  CPU and memory requests with units. Missing requests shall show as unspecified, not zero usage.
- **AC-EXECUTORS-K8S-RETAINED-001.5:** Refresh shall replace stale classifications
  after Resume, deletion, or a status-access failure. Errors shall not appear as zero retained resources.

### REQ-EXECUTORS-K8S-RETAINED-002: Accessible operator guidance

**Intent:** Users can inspect the relevant task and understand the existing cleanup path.

#### Acceptance criteria

- **AC-EXECUTORS-K8S-RETAINED-002.1:** Desktop and mobile users shall see the
  same status, requests, workspace mode, and retention explanation in Active sessions.
- **AC-EXECUTORS-K8S-RETAINED-002.2:** Each visible row shall provide navigation
  to its authorized task. Navigation and refresh shall perform no destructive operation.
- **AC-EXECUTORS-K8S-RETAINED-002.3:** Visible guidance shall explain that Stop
  preserves resources and that Archive/Delete can remove a managed workspace. Existing claims remain operator-owned.
- **AC-EXECUTORS-K8S-RETAINED-002.4:** Phone cards shall support touch navigation,
  keyboard access, long identifiers, and localized text without document-level horizontal overflow.
- **AC-EXECUTORS-K8S-RETAINED-002.5:** Users shall receive no additional task
  identities, Pod details, or global retained-resource totals beyond their existing access.

## Out of scope

- Automatic suspend, expiration, garbage collection, or changes to Stop/Resume/Archive/Delete.
- New Pod mutation controls, bulk cleanup, or direct archive/delete buttons in executor configuration.
- CPU usage, billing estimates, cluster capacity, or aggregate Pod scheduling calculations.
- Resource requests for sidecars, init containers, Pod overhead, or Pod-level resource budgets.
- A new global dashboard, filter system, or changes to executor mutation-impact counts.
- A claim that retained sessions contain no background processes.
