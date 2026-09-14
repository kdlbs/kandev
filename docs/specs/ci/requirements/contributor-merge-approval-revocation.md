---
status: draft
system: ci
created: 2026-08-31
owners:
  - kandev
---

# Contributor merge approval revocation requirements

## Overview

The `ready-to-merge` label is a maintainer approval for a specific pull-request
revision. A new commit from a user without write access makes that approval
stale. The CI automation system owns the base-controlled workflow that revokes
the stale approval and prevents an already-started merge operation from using
it.

## Terminology

- **Merge approval label:** The exact `ready-to-merge` pull-request label.
- **Pusher:** The user identified by the `synchronize` event sender.
- **Write-capable pusher:** A pusher whose current repository permission is
  `write` or `admin`. GitHub's `maintain` role is included in the `write`
  permission value.
- **Untrusted push:** A `synchronize` event whose pusher is not write-capable,
  including an unknown pusher or an unavailable permission result.

## Requirements

### REQ-CI-MERGE-APPROVAL-001: Revoke stale merge approval after an untrusted push

**Intent:** Ensure that a contributor cannot change a pull request after a
maintainer approves it for merge and leave the stale approval connected to an
active merge operation.

#### Acceptance criteria

- **AC-CI-MERGE-APPROVAL-001.1:** When an untrusted push synchronizes a pull
  request that had the `ready-to-merge` label in the event snapshot, the
  system shall remove that label from the pull request.
- **AC-CI-MERGE-APPROVAL-001.2:** After the label revocation for an untrusted
  push, when GitHub reports an active auto-merge request, the system shall
  disable that auto-merge request, and when GitHub reports an active merge
  queue entry, the system shall remove the pull request from the queue.
- **AC-CI-MERGE-APPROVAL-001.3:** When the event snapshot does not contain the
  `ready-to-merge` label, the system shall leave the pull request's labels,
  auto-merge state, and merge-queue state unchanged.
- **AC-CI-MERGE-APPROVAL-001.4:** When a write-capable pusher synchronizes a
  pull request, the system shall leave the pull request's labels, auto-merge
  state, and merge-queue state unchanged.
- **AC-CI-MERGE-APPROVAL-001.5:** When the pusher is missing, unknown, or the
  permission lookup fails, the system shall classify the push as untrusted and
  attempt the same approval and merge-state cleanup, while reporting the
  permission lookup failure in the workflow result.
- **AC-CI-MERGE-APPROVAL-001.6:** When cleanup is retried or an already-clean
  state is observed, the system shall not remove unrelated labels or report a
  successful merge-state change that was not needed.
- **AC-CI-MERGE-APPROVAL-001.7:** The cleanup shall run from trusted
  base-controlled workflow content without checking out or executing
  pull-request files, and its job shall have only the pull-request write
  permission required for the cleanup.

## Out of scope

- Removing or changing the existing `safe-to-review` contributor automation
  label contract.
- Re-evaluating code changes, reviews, checks, or branch protection rules.
- Automatically reapplying `ready-to-merge` after a maintainer review.
- Changing merge behavior for commits pushed by a write-capable user.
- Supporting providers other than GitHub pull requests.
