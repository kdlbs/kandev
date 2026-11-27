---
status: active
system: workspaces
created: 2026-10-08
owners:
  - kandev
---

# Commit draft retry requirements

## Overview

A rejected commit must leave the user's message available for correction and
retry. Commit-hook failures and rejected requests are frequent error-recovery
paths; discarding a written message loses user work. Workspaces owns this
capability because it owns workspace Git state and repository scope. Shared
Git status reads, task environment bindings and reusable UI presentation keep
their existing owners.

## Terms and boundary

- **Draft:** The exact entered title and body, repository choice and Stage all
  choice. Whitespace in the draft is user data.
- **Acknowledgement:** The commit operation reports success, including the
  aggregate outcome when several repositories participate.
- **Current draft:** The draft in the mounted commit surface's current session
  and repository scope. A deliberate opening for a different repository or a
  session/environment change starts a fresh scope, rather than moving a draft.
- **Retryable draft:** A submitted draft that is pending or failed, or edits
  made during a submission that have not themselves been acknowledged.

## Requirements

### REQ-WORKSPACES-COMMIT-DRAFT-RETRY-001: Commit draft recovery

**Intent:** Users can correct and retry failed commits without reconstructing
their message or inadvertently changing the repository or staging choice.

#### Acceptance criteria

- **AC-WORKSPACES-COMMIT-DRAFT-RETRY-001.1:** When a current commit reports
  failure, the dialog shall remain open with the exact title/body bytes,
  repository and Stage all choice retained, and existing failure feedback
  shall remain visible. Dismissing and reopening the same scope shall retain
  that retryable draft.
- **AC-WORKSPACES-COMMIT-DRAFT-RETRY-001.2:** When a current commit request
  rejects, the same retention and retry behavior shall apply, including the
  existing exception feedback. Failure shall not trigger an automatic retry.
- **AC-WORKSPACES-COMMIT-DRAFT-RETRY-001.3:** When an unchanged current draft
  receives a successful acknowledgement, the dialog shall close and reset;
  its next opening shall have empty title/body and Stage all off. Existing
  success feedback shall remain available.
- **AC-WORKSPACES-COMMIT-DRAFT-RETRY-001.4:** A blank or whitespace-only title
  shall admit no commit and shall not close or clear the draft. A valid
  submission shall trim the title and body for the payload only, separating a
  non-empty body with two newlines, and shall use the captured repository and
  Stage all choice with amend disabled.
- **AC-WORKSPACES-COMMIT-DRAFT-RETRY-001.5:** Edits to title, body or Stage all
  during a pending submission shall survive either outcome. Success of the
  older submitted values shall not close or clear these newer edits. The
  request shall use its admitted values, rather than later edits. A second
  commit shall not be admitted while the current submission is pending.
- **AC-WORKSPACES-COMMIT-DRAFT-RETRY-001.6:** Dismissal during a submission
  shall not cause settlement to reopen the dialog. Reopening the same scope
  while pending shall preserve the current draft and pending admission guard.
  A deliberate opening for a different repository shall start a fresh draft;
  an older completion shall not close, replace or unlock a newer submission.
  Omitted scope, explicit workspace-root scope and named repositories shall
  remain distinct. Ordinary dismissal before submission retains its existing
  fresh-opening behavior.
- **AC-WORKSPACES-COMMIT-DRAFT-RETRY-001.7:** Changing session or its task
  environment, or unmounting the commit owner, shall retire its pending draft
  ownership. An old outcome shall not expose the old draft in the new scope
  or mutate the new scope's draft/pending state, including a change away and
  back to the same identity. No commit shall be admitted without a session.
- **AC-WORKSPACES-COMMIT-DRAFT-RETRY-001.8:** Multi-repository submissions
  shall retain the existing repository selection and fan-out rules. A partial
  failure shall retain the draft and existing aggregate error feedback;
  successful Git writes shall remain completed. A retry is a new user action
  against current Git state and shall not roll back or automatically replay
  already completed writes.
- **AC-WORKSPACES-COMMIT-DRAFT-RETRY-001.9:** Desktop and phone commit surfaces
  shall share these state and acknowledgement semantics while retaining the
  existing composition, localized copy, controls, navigation and dismissal.
  Viewport changes alone shall not clear the current draft.

## Out of scope

- Durable storage, drafts across owner unmount/reload, or a per-repository draft
  collection.
- Git rollback, abort/cancellation transport, automatic retries, amend/reset or
  other Git-operation behavior changes.
- Backend/API/permission policy changes, generic mutation infrastructure,
  arbitrary session lifetime redesign or rendered UI redesign.

## Implementation plans

[Commit draft retry plan](../../../plans/commit-draft-retry/plan.md).
