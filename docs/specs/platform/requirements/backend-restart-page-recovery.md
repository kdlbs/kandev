---
status: active
system: platform
created: 2026-09-03
owners:
  - kandev
---

# Backend Restart Page Recovery Requirements

## Overview

An open Kandev document can outlive the backend process that created it. The
document then contains old boot data, cached route data, and an old agent
settings interlock token. A later action can fail with an internal error, or it
can use data that no longer matches the running backend.

Kandev must detect a confirmed backend restart before the next user action. It
must show one clear reload alert on every authenticated application route.

## Terminology

- **Backend generation:** One backend process lifetime, identified by its
  `boot_id`.
- **Page generation:** The backend generation in the document boot payload.
- **Reload-required state:** A document state that starts after proof that the
  backend generation changed. It ends only when the browser loads a new
  document.
- **Application route:** An authenticated route inside the shared application
  shell. Login, setup, and other routes outside that shell are not application
  routes.

## Requirements

### REQ-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001: Detect and recover an old application document

**Intent:** Tell users to reload as soon as Kandev proves that an open document
belongs to an earlier backend process.

**User story:** As a Kandev user, I want an immediate reload alert after the
backend restarts. This alert stops me from using an old application document.

#### Acceptance criteria

- **AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.1:** Each application
  document shall receive the current backend `boot_id` in its boot payload.
- **AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.2:** After each successful
  WebSocket connection, the document shall request current system information
  without using a cached response.
- **AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.3:** If the returned
  `boot_id` differs from the page generation, Kandev shall enter the
  reload-required state without another user action.
- **AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.4:** Kandev shall not enter
  the reload-required state because of a disconnect, a failed identity request,
  or a response with the same `boot_id`.
- **AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.5:** Every application route
  shall show one persistent, non-dismissible, in-flow reload alert while the
  document is in the reload-required state. Client-side navigation shall not
  hide or duplicate it.
- **AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.6:** The alert shall state
  that Kandev restarted and that the user must reload the page to continue. It
  shall warn that the reload discards unsaved changes.
- **AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.7:** The alert shall include
  a **Reload page** action. Kandev shall not reload automatically.
- **AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.8:** The reload action shall
  reload the current browser location. The new document shall use the current
  backend generation and clear the reload-required state.
- **AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.9:** A protected agent
  settings rejection with the stable stale-interlock code shall also enter the
  same reload-required state. This fallback shall suppress the internal error
  and duplicate operation errors.
- **AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.10:** An intentional restart
  or self-update flow shall use the same recovery state. Kandev shall show only
  one reload-required surface when that flow already owns the reload action.
- **AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.11:** Other authorization,
  connectivity, and settings errors shall keep their current behavior.
- **AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.12:** Desktop, tablet, and
  phone layouts shall provide the same recovery outcome. The action shall
  support keyboard use and a touch target of at least 44px on phone layouts.
  The alert shall not cause horizontal overflow or cover navigation.

## User-visible contract

The English alert uses this copy:

- Title: **Reload required**
- Body: **Kandev restarted. Reload this page to continue. Reloading discards
  unsaved changes.**
- Action: **Reload page**

All visible copy is localized. Diagnostic details and the internal interlock
message do not appear in this alert.

## Scenarios

- **GIVEN** an application document from the current backend, **WHEN** its
  WebSocket reconnects to the same process, **THEN** no reload alert appears.
- **GIVEN** an open application document, **WHEN** the backend restarts and the
  WebSocket reconnects, **THEN** the reload alert appears before another user
  action.
- **GIVEN** a system information request fails, **WHEN** Kandev has no process
  identity proof, **THEN** only the existing connectivity behavior applies.
- **GIVEN** the reload alert is visible, **WHEN** the user changes application
  routes, **THEN** the same alert remains visible.
- **GIVEN** a profile save races with restart detection, **WHEN** the backend
  returns the stale-interlock code, **THEN** the same reload alert appears. The
  internal message stays hidden.
- **GIVEN** the reload alert is visible, **WHEN** the user selects **Reload
  page**, **THEN** the browser reloads the current location. The new document
  does not contain the alert.

## Out of scope

- Automatic reload after a backend restart.
- Persistence or restoration of unsaved form data.
- A restart alert based only on a network failure or WebSocket disconnect.
- Changes to the security claims, token transport, or route scope of the
  interim settings interlock.
- Backend process history or restart metrics.

## Related requirements

- [Agent Runtime Availability](agent-runtime-availability.md)
- [Session subscription recovery](session-subscription-recovery.md)
- [Semantic Notifications](notifications.md)
