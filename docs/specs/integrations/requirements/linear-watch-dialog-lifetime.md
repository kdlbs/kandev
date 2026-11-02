---
status: active
system: integrations
created: 2026-10-22
owners:
  - kandev
---

# Linear Watch Dialog Lifetime Requirements

## Overview

Integrations owns Linear watch configuration and its install-wide create/edit
flow. A pending save must leave later user work intact while its original write
and feedback complete.

## Terminology

- **Opening:** A continuous create or selected-watch edit presentation.
- **Scope:** The selected watch's stored workspace, or the create form's chosen workspace.

## Requirements

### REQ-INTEGRATIONS-LINEAR-WATCH-DIALOG-001: Current watch form continuity

**Intent:** Preserve the user's current form while accepted writes settle.

#### Acceptance criteria

- **AC-INTEGRATIONS-LINEAR-WATCH-DIALOG-001.1:** After Cancel and reopening the
  same watch, another watch or create, earlier success or failure shall preserve
  the current raw fields, focus, caret/selection and current busy state. Later
  Save and Cancel shall remain usable.
- **AC-INTEGRATIONS-LINEAR-WATCH-DIALOG-001.2:** Accepted writes shall retain
  their submitted workspace, watch ID and payload across dismissal, replacement
  and unmount. Existing successful store/table publication and truthful success
  or error feedback shall complete independently of dialog ownership.
- **AC-INTEGRATIONS-LINEAR-WATCH-DIALOG-001.3:** Current success shall close its
  form. Current failure shall preserve raw fields and permit retry. Cancel shall
  dismiss without another write.
- **AC-INTEGRATIONS-LINEAR-WATCH-DIALOG-001.4:** Same-ID passive watch hydration
  and unrelated active-workspace changes shall preserve a draft and its pending
  ownership. Selected watch or locked workspace replacement shall initialize
  the replacement. New openings shall initialize saved values or create defaults.
- **AC-INTEGRATIONS-LINEAR-WATCH-DIALOG-001.5:** An explicit create workspace
  switch during a pending save shall retire that save's dialog completion and
  busy ownership. It shall preserve query, team key, prompt and other portable
  settings while clearing workspace-bound workflow, step, repository, branch
  and provider-ID selections under existing switching rules. Passive
  default hydration shall not discard an explicit workspace choice.
- **AC-INTEGRATIONS-LINEAR-WATCH-DIALOG-001.6:** Desktop and phone users shall
  receive these outcomes through the shipped install-wide Linear settings flow.

## Out of scope

Linear connection settings and publication, provider inventories, polling/scope
caching, Query migration, backend/API/schema/auth, server write ordering, other
integrations, request abortion and shared durable list authority.
