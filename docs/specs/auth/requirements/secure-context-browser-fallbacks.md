---
status: active
system: auth
created: 2026-08-02
owners:
  - kandev
---
# Repair secure-context browser fallbacks Requirements

## Overview

Preserve the observable behavior documented for Repair secure-context browser fallbacks.

## Requirements

### REQ-AUTH-SECURE-CONTEXT-BROWSER-FALLBACKS-001: Repair secure-context browser fallbacks

**Intent:** Preserve the observable behavior documented for Repair secure-context browser fallbacks.

#### Acceptance criteria

- **AC-AUTH-SECURE-CONTEXT-BROWSER-FALLBACKS-001.1:** Client-only identifiers used by workflows, workflow steps, and layout profiles are generated successfully whether or not `crypto.randomUUID()` is available. The secure Web Crypto implementation remains preferred; the fallback is only for non-security identifiers.
- **AC-AUTH-SECURE-CONTEXT-BROWSER-FALLBACKS-001.2:** Clipboard actions use the modern Clipboard API when it is available and fall back to the existing DOM copy mechanism when the API is unavailable or rejects the request. A missing clipboard capability never produces an uncaught browser API `TypeError`.
- **AC-AUTH-SECURE-CONTEXT-BROWSER-FALLBACKS-001.3:** Existing capability-gated APIs remain graceful in insecure contexts: `crypto.subtle` keeps its content-hash fallback, voice input reports that it needs HTTPS or localhost, and notification/audio features continue to no-op or report unsupported instead of throwing.
- **AC-AUTH-SECURE-CONTEXT-BROWSER-FALLBACKS-001.4:** These fallbacks preserve the current successful-origin behavior and do not change backend, WebSocket, or persistence contracts.
- **AC-AUTH-SECURE-CONTEXT-BROWSER-FALLBACKS-001.5:** **GIVEN** the page has no `crypto.randomUUID`, **WHEN** the user confirms Add Workflow, **THEN** a client-only workflow draft and its steps are created without an exception and receive unique UUID-shaped IDs.
- **AC-AUTH-SECURE-CONTEXT-BROWSER-FALLBACKS-001.6:** **GIVEN** the page has no `crypto.randomUUID`, **WHEN** the user adds a workflow step or creates a layout profile, **THEN** the client-only item is created without an exception and receives a unique ID.
- **AC-AUTH-SECURE-CONTEXT-BROWSER-FALLBACKS-001.7:** **GIVEN** `navigator.clipboard` is missing or `writeText` rejects, **WHEN** a user activates any copy action, **THEN** the DOM fallback is attempted and no uncaught promise or `TypeError` escapes the action.
- **AC-AUTH-SECURE-CONTEXT-BROWSER-FALLBACKS-001.8:** **GIVEN** `crypto.subtle` is missing, **WHEN** file content hashing is requested, **THEN** the existing non-cryptographic change-detection hash is returned.

## Migrated source detail

## Problem

Kandev advertises plain HTTP network URLs for remote browser access. Some Web
APIs used by the web app, including `crypto.randomUUID()` and
`navigator.clipboard`, are only exposed or usable in secure contexts. Direct
calls currently make otherwise valid workflow and copy actions throw when the
app is opened over a non-localhost HTTP origin.

## Desired behavior

- Client-only identifiers used by workflows, workflow steps, and layout
  profiles are generated successfully whether or not `crypto.randomUUID()` is
  available. The secure Web Crypto implementation remains preferred; the
  fallback is only for non-security identifiers.
- Clipboard actions use the modern Clipboard API when it is available and
  fall back to the existing DOM copy mechanism when the API is unavailable or
  rejects the request. A missing clipboard capability never produces an
  uncaught browser API `TypeError`.
- Existing capability-gated APIs remain graceful in insecure contexts:
  `crypto.subtle` keeps its content-hash fallback, voice input reports that it
  needs HTTPS or localhost, and notification/audio features continue to no-op
  or report unsupported instead of throwing.
- These fallbacks preserve the current successful-origin behavior and do not
  change backend, WebSocket, or persistence contracts.

## Regression scenarios

- **GIVEN** the page has no `crypto.randomUUID`, **WHEN** the user confirms Add
  Workflow, **THEN** a client-only workflow draft and its steps are created
  without an exception and receive unique UUID-shaped IDs.
- **GIVEN** the page has no `crypto.randomUUID`, **WHEN** the user adds a
  workflow step or creates a layout profile, **THEN** the client-only item is
  created without an exception and receives a unique ID.
- **GIVEN** `navigator.clipboard` is missing or `writeText` rejects, **WHEN** a
  user activates any copy action, **THEN** the DOM fallback is attempted and no
  uncaught promise or `TypeError` escapes the action.
- **GIVEN** `crypto.subtle` is missing, **WHEN** file content hashing is
  requested, **THEN** the existing non-cryptographic change-detection hash is
  returned.
- **GIVEN** the page is an insecure HTTP origin, **WHEN** voice, notification,
  or notification-sound capability checks run, **THEN** the UI reports or
  applies the existing unsupported/no-op behavior without throwing.

## Constraints

- `crypto.randomUUID()` remains the preferred source when present.
- Fallback UUIDs are for temporary/client identifiers only and MUST NOT be
  used for secrets, authentication tokens, or security decisions.
- The modern Clipboard API remains preferred; fallback copy is best effort and
  must preserve focus behavior for dialogs.
- Remote plain HTTP access remains supported; this repair does not require
  adding TLS or changing server binding.

## Out of scope

- Adding an HTTPS server or changing the advertised network URLs.
- Replacing secure randomness for security-sensitive values.
- Redesigning voice, notification, or sound settings beyond preventing
  capability-related exceptions.
