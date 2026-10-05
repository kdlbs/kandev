---
status: draft
system: plugins
created: 2026-10-04
owners:
  - kandev
---

# Managed conversation deletion admission requirements

## Overview

The Plugins system owns installation-scoped managed conversation lifetime and
the exact Host command used to delete it. This focused supplement extends
[managed coordination](managed-coordination.md), especially
`REQ-PLUGINS-MANAGED-COORDINATION-002`, without promoting the broad draft or
claiming that atomic launch-settings admission already protects deletion.
The paired [design](../system-design/managed-deletion-admission.md) records
ROOT's selected exclusive admission boundary. Lifetime implementation was
released and remains incomplete. ROOT reviewed and released the shared SQLite writer-entry extension;
acceptance and delivery remain pending.

## Terminology

- **Retained identity:** Installation, workspace, instance, and the particular
  retained task incarnation that owns the transcript.
- **Exclusive admission:** A committed, operation-owned authorization and
  exclusion boundary between an exact deletion and competing conversation
  changes. Its prepared barrier is reversible and does not remove the task.
- **Deletion commit:** The durable removal of the admitted task and its sessions.
- **Rejected deletion:** An attempt whose exclusive admission has not committed.
- **Admitted failure:** An error after exclusive admission. The task may still
  exist and environment or canvas preparation may already have partial effects.

## Requirements

### REQ-PLUGINS-MANAGED-COORDINATION-013: Current managed deletion authority

**Intent:** Prevent a delayed exact deletion from destroying an independently
accepted revision or a detached retained transcript, while preserving host
lifecycle cleanup and truthful command outcomes.

#### Acceptance criteria

- **AC-PLUGINS-MANAGED-COORDINATION-013.1:** When an independently accepted
  configuration or pause revision wins before deletion admission, deletion at
  the earlier revision shall conflict. The accepted task, primary session,
  launch settings, and retained transcript shall remain available.
- **AC-PLUGINS-MANAGED-COORDINATION-013.2:** When detach or a change of retained
  identity wins before admission, the old installation's deletion shall return
  not found and preserve the retained task, sessions, and transcript. A missing
  identity shall not authorize deletion of a replacement incarnation.
- **AC-PLUGINS-MANAGED-COORDINATION-013.3:** When current deletion wins admission,
  a competing revision change or detach shall not be accepted and then erased
  by that deletion. Contention shall settle through typed outcomes without
  deadlocking a competing caller behind external cleanup.
- **AC-PLUGINS-MANAGED-COORDINATION-013.4:** A rejected deletion or cancellation
  before exclusive admission commits shall leave accepted rows intact
  and shall not stop execution, transfer environment ownership, remove canvas
  authority, delete attachment bytes or worktrees, publish deletion, or activate
  a destructive cleanup job. After admission, preparation may have existing
  partial environment or canvas effects; an own failed final deletion shall
  preserve task, session, and transcript rows without promising rollback of
  those preparation effects. The reversible barrier shall be released only
  by its owner or proven recovery authority, and uncertainty shall retain it.
- **AC-PLUGINS-MANAGED-COORDINATION-013.5:** A current accepted deletion shall
  perform host lifecycle cleanup, preserve borrowed resources for surviving
  tasks, obey parent and child deletion rules, remove the admitted task and
  sessions, and emit existing deletion and queue notifications only for an
  actual deletion. Canvas preparation failure shall continue to abort physical
  task removal. Pre-admission errors, admitted failures, deletion commit,
  and postcommit cleanup failures shall remain distinguishable. Reopening a
  retained task after admitted failure shall not claim to undo partial effects.
- **AC-PLUGINS-MANAGED-COORDINATION-013.6:** The registered exact Host command
  shall retain current approval and installation boundaries, durable receipts,
  conflict/not-found/unavailable mappings, payload-conflict handling, completed
  replay, and acknowledgement recovery. It shall not describe a failed receipt
  acknowledgement after deletion as an effect-free rejection or delete a newly
  created conversation while replaying a completed command. Admitted failure
  or uncertain commit shall be unavailable, rather than success or clean
  conflict. An incomplete replay shall require evidence of that operation's
  committed deletion before reporting already applied from task absence.
- **AC-PLUGINS-MANAGED-COORDINATION-013.7:** These outcomes shall hold across
  independent services sharing SQLite or PostgreSQL, including physical waits,
  current identity and revision after waiting, cancellation, and rollback.
  Dedicated read snapshots shall continue progressing while independent writers
  contend. Cancellation shall settle without accepted admission or leaked
  database ownership; native busy-handler latency is bounded rather than
  guaranteed immediate. Missing native admission support shall fail unavailable
  before effects.
- **AC-PLUGINS-MANAGED-COORDINATION-013.8:** Ordinary task deletion and legacy
  conversation cleanup shall preserve their current ownership, hierarchy,
  missing-row, error, worktree-consent, cleanup, and notification behavior.
  Retained managed conversations shall remain excluded from legacy plugin
  uninstall deletion; uninstall shall detach their transcripts. Shared native
  paths shall exclude a foreign live managed-deletion owner where necessary;
  unrelated ordinary and legacy behavior shall retain its baseline semantics.

## Out of scope

No new wire method, schema, approval model, UI, runtime toggle, generic callback
or version framework, generic cleanup-engine replacement, or production plugin.
This is a backend data/lifecycle correction. It adds no desktop or mobile
interaction, copy, rendering, navigation, or localization change.

## Related delivery

- [Design](../system-design/managed-deletion-admission.md)
- [Plan](../../../plans/managed-deletion-admission/plan.md)
- [Existing atomic settings correction](../../../plans/managed-conversation-admission/plan.md)
