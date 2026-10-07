---
status: active
system: ui
created: 2026-10-06
owners:
  - web
---

# Composer File Search Ownership Requirements

## Overview

The shared chat composer's `@` file suggestions belong to the session being
edited. Switching sessions while retaining the editor must not reuse another
session's completed file search or accept its delayed file candidates.

UI owns this reusable editor candidate/reply ownership contract. Workspaces
retains filesystem access and server-side search authority. This contract is
independent of [mention recency](composer-mention-recency.md), which ranks
eligible candidates, and [suggestion overlays](composer-suggestion-overlays.md),
which owns presentation geometry. Neither owns file-search request identity.

## Terminology

- **Owner lifetime:** One mounted composer with one committed session identity.
  A committed change to another session, to no session, or unmount retires that
  lifetime. Returning to the same session creates a new lifetime.
- **File query:** The exact text following the active `@` trigger, including
  empty text. No-session composers have no eligible file-search owner.
- **Retired settlement:** A reply to a file search from a retired owner lifetime
  or a request superseded by a later file-candidate lookup in that composer.

## Requirements

### REQ-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001: Session-owned file candidates

**Intent:** Keep file suggestions useful when a chat editor survives session
navigation and outstanding replies arrive in a different order.

#### Acceptance criteria

- **AC-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001.1:** When a composer completes an
  `@` file search and then commits a different session, its next identical query
  shall search the new session and show that session's returned matching files.
  The prior session's completed files shall not satisfy that new lookup.
- **AC-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001.2:** Within one owner lifetime,
  repeating the last successfully completed exact query shall reuse its files
  without another file-search request. Different query text, including empty
  text versus a literal query named `__empty__`, shall remain distinct. New
  searches shall retain the exact query and the existing limit of 20.
- **AC-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001.3:** A retired settlement shall
  neither replace the current reusable file results nor contribute its retired
  file paths to the returned candidate list. This applies to success, failure,
  and empty replies, including an older reply arriving before, during, or after
  the newer lookup settles. This guarantee concerns file candidates and cache
  publication; it does not define generic suggestion-popup cancellation.
- **AC-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001.4:** When session identity becomes
  null, the composer shall perform no file search and offer no session-owned
  files on the next candidate lookup. Null-to-session and session-A-to-B-to-A
  transitions shall require fresh file reads. Retirement shall not edit, clear,
  restore, or submit the draft and shall not itself issue a file-search request.
- **AC-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001.5:** Independent mounted composers
  shall not share completed file results or retire each other's reads. Ordinary
  rerenders in the same lifetime shall preserve completed-query reuse. Mount,
  unmount, and development effect replay shall leave current lookups usable and
  prevent retired work from publishing file results.
- **AC-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001.6:** Current successful, empty,
  failed, or unavailable-client lookups shall preserve the existing non-file
  candidate sources, error fallback, ranking/recency, and public input/insertion
  behavior. The same file ownership semantics shall apply on desktop and phone
  without changing menu layout, touch controls, scrolling, or navigation.

## Out of scope

- Changing backend search, WebSocket protocol, workspace authority, store shape,
  persistent storage, or introducing a shared cache/coordinator.
- General composer rewrite, draft serialization, submission/focus ownership,
  entity `#`, slash commands, plugin architecture, or reverse message search.
- Changing task, prompt, Plan sources, ranking, recency, copy, menu geometry,
  breakpoints, touch behavior, navigation, or layout.
- Extending the legacy inline-mention hook's file branch without a demonstrated
  session-bearing production consumer.
