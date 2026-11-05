---
status: active
system: integrations
created: 2026-10-22
owners:
  - kandev
---

# Jira Watch Dialog Lifetime Requirements

## Overview

Integrations owns Jira issue-watch configuration, including continuity of its
install-wide create/edit form while previously accepted saves settle.

## Terminology

- **Opening:** A continuous create or selected-watch edit presentation.
  Cancel followed by reopening the same watch starts another opening.
- **Accepted save:** A request submitted by Save, retaining its captured target
  and raw payload independently of later form activity.

## Requirements

### REQ-INTEGRATIONS-JIRA-WATCH-DIALOG-001: Current watch form continuity

**Intent:** Users can dismiss a pending save and continue editing without its
completion disrupting the current form.

#### Acceptance criteria

- **AC-INTEGRATIONS-JIRA-WATCH-DIALOG-001.1:** When an earlier update or create succeeds or fails after Cancel
  and opening the same watch, another watch, or a create form, the current form
  shall retain its raw fields, focus, caret/selection, saving state and usable
  Save/Cancel actions.
- **AC-INTEGRATIONS-JIRA-WATCH-DIALOG-001.2:** A previously accepted request shall retain its original workspace,
  watch ID and payload, including JQL whitespace. Success shall retain existing
  store/list publication and truthful success feedback; failure shall retain
  truthful failure feedback without changing a later form.
- **AC-INTEGRATIONS-JIRA-WATCH-DIALOG-001.3:** Current-owner success shall close the form. Current-owner failure
  shall retain raw fields, clear its busy state and permit retry. Cancel shall
  dismiss without another write. An older completion shall not clear a newer
  Save's busy state.
- **AC-INTEGRATIONS-JIRA-WATCH-DIALOG-001.4:** Same-identity passive watch hydration shall preserve the draft and
  pending ownership. Committed selection, bound workspace or mount replacement
  shall initialize a new owner. A suspended uncommitted replacement shall leave
  the visible form's authority intact, including development lifecycle replay.
- **AC-INTEGRATIONS-JIRA-WATCH-DIALOG-001.5:** Explicit workspace selection in an unlocked create form shall
  preserve non-workspace raw fields and clear workflow, step, repository and
  base branch only when workspace changes. Such a change shall retire an older
  save's authority over the form without retargeting its request. Passive active
  workspace/default hydration shall not overwrite a selected create workspace
  or an existing edit draft. An initially empty workspace may adopt its first
  available default without replacing raw fields.
- **AC-INTEGRATIONS-JIRA-WATCH-DIALOG-001.6:** Desktop and phone users shall receive these outcomes through the
  shipped Jira settings flow, retaining existing JQL Test, defaults, project,
  profile and repository behavior.

## Out of scope

Connection settings, health/read publication, other integrations, polling,
list-cache/API/backend/auth/schema changes, server write ordering, shared
frameworks, durable authority, queues and request abortion.
