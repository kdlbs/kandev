---
status: active
system: architecture-lint
created: 2026-10-09
owners:
  - kandev
---

# Root-state slice typing requirements

## Overview

The architecture-lint system owns this internal contract because it governs
the existing frontend root-state-cast boundary and its shrink-only baseline.
The contract covers compile-time slice composition and evidence tracking. It
does not define Jira or Linear provider behavior, APIs, credentials, or UI.

## Terminology

- **Recipe setter:** A Zustand setter that accepts an Immer recipe for a slice
  draft.
- **Root composition:** Adding a slice creator to the application store.
- **Current inventory:** Counts measured from a named main commit, separate
  from counts retained for an older dated snapshot.

## Requirements

### REQ-ARCHITECTURE-LINT-ROOT-STATE-SLICE-TYPING-001: Keep issue-watch slice composition typed

**Intent:** Remove the Jira and Linear issue-watch root-store escapes while
preserving their existing state contract and making the architecture record
reliable.

#### Acceptance criteria

- **AC-ARCHITECTURE-LINT-ROOT-STATE-SLICE-TYPING-001.1:** When the application
  store composes the Jira or Linear issue-watch slice, TypeScript shall accept
  the slice's Immer recipe-setter capability without a type assertion or an
  unused getter or store API argument.
- **AC-ARCHITECTURE-LINT-ROOT-STATE-SLICE-TYPING-001.2:** When any existing
  issue-watch action runs, its signature, item ordering, loaded/loading flags,
  reset behavior, existing initial-state and hydration results, isolation
  between separate stores, and unrelated root-state references shall remain
  unchanged.
- **AC-ARCHITECTURE-LINT-ROOT-STATE-SLICE-TYPING-001.3:** When one of these
  slice casts is removed, the same change shall remove exactly that slice's
  three matching root-state-cast baseline entries and shall not remove or add
  an unrelated baseline identity.
- **AC-ARCHITECTURE-LINT-ROOT-STATE-SLICE-TYPING-001.4:** When the architecture
  maintenance records summarize completed work, they shall link completed
  outcomes to verified merged evidence, label any open Jira/Linear delivery as
  implementation complete and delivery pending, and distinguish historical
  counts from measurements tied to a current-main commit.

## Out of scope

- Jira or Linear provider behavior, credentials, transport, API contracts,
  synchronization, caching, or user-facing UI.
- A generic root-store typing framework, broad `AppState` creator types, or
  cleanup of unrelated slices or baseline entries.
- New Query migrations, alias retirements, architecture rules, or runtime
  behavior.
