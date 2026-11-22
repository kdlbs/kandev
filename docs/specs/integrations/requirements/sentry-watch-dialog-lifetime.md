---
status: active
system: integrations
created: 2026-10-22
owners:
  - kandev
---

# Sentry Watch Dialog Lifetime Requirements

## Overview

Integrations owns Sentry issue-watch configuration and its create/edit dialog.
Users can cancel an in-flight save and continue in a later opening while the
accepted request settles. That later form must retain its own lifetime.

## Terminology

- **Opening:** One continuous create or selected-watch edit presentation.
  Reopening the same watch starts a new opening.
- **Accepted request:** A create or update already submitted through Save.

## Requirements

### REQ-INTEGRATIONS-SENTRY-WATCH-DIALOG-001: Current watch form continuity

**Intent:** Preserve the current Sentry watch form while earlier writes settle.

#### Acceptance criteria

- **AC-INTEGRATIONS-SENTRY-WATCH-DIALOG-001.1:** When an earlier accepted request
  succeeds or fails after Cancel and reopening the same watch, another watch,
  or a new create form, the current form shall retain its exact raw fields,
  focus, caret/selection, and its own saving state and usable Save/Cancel actions.
- **AC-INTEGRATIONS-SENTRY-WATCH-DIALOG-001.2:** Cancellation, unmount or scope
  replacement shall not retarget or cancel an accepted request. Its original
  workspace, watch, instance and payload shall remain captured. Successful
  writes shall retain existing list publication and truthful success feedback;
  failures shall retain truthful error feedback without changing a later form.
- **AC-INTEGRATIONS-SENTRY-WATCH-DIALOG-001.3:** A successful current opening
  shall close. A failed current opening shall keep its draft and permit retry.
  Cancel shall dismiss the current opening without an additional write.
- **AC-INTEGRATIONS-SENTRY-WATCH-DIALOG-001.4:** Same-identity passive watch
  hydration shall preserve the raw draft and pending ownership. A newly opened
  or differently selected watch/workspace/instance shall initialize its own
  saved values or create defaults, retaining existing selector/default behavior.
- **AC-INTEGRATIONS-SENTRY-WATCH-DIALOG-001.5:** Desktop and phone users shall
  receive these outcomes through the shipped Sentry settings flow.

## Out of scope

Sentry instance forms and health refresh, other integrations, watch polling,
lookback semantics, Query/store migration, API/backend/auth changes, server write
ordering, shared durable authority, cross-tab coordination and request abortion.
