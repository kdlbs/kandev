---
created: 2026-08-27
status: done
requirements:
  - REQ-AGENTS-PROFILE-RECENT-USE-001
  - REQ-AGENTS-PROFILE-RECENT-USE-002
  - REQ-AGENTS-PROFILE-RECENT-USE-003
system_design:
  - ../../specs/agents/system-design/profile-recent-use.md
legacy_specs: []
---

# Implementation Plan: Agent Profile Recent Use

## Overview

Add the bounded backend recency contract first, hydrate its compact projection
into independent frontend state, apply stable contextual ordering at operational
selectors, and finally record only successful effective launches. This order
keeps persistence and synchronization authoritative before UI consumers begin
writing or reading the new data.

## Scope

### In scope

- Four user-scoped operational contexts with ten distinct IDs each.
- Dedicated bounded persistence, focused HTTP contracts, boot hydration, and a
  compact user-routed WebSocket update.
- Stable recency ordering for task creation, subtask creation, task sessions,
  handoff, quick chat, and configuration chat.
- Success-only recording with effective-profile and supersession handling.
- Focused backend, frontend, and browser evidence.

### Out of scope

- Changes to default profile resolution or profile eligibility.
- Recency for settings, defaults, automation, or Office assignment selectors.
- Workspace- or task-scoped history, pinning, manual ordering, clearing UI, or
  browser persistence.
- Layout, overlay, touch, scrolling, breakpoint, or user-facing copy changes.

## Technical approach

### Bounded backend record

Add `user_agent_profile_recent_use` to the user repository base schema and
replay migrations. Store one ordered JSON array plus revision and timestamp per
user/context. Extend the repository, service, DTO, controller, and handlers with
list and move-to-front operations. Use bounded revision retries and suppress
writes/events when the requested profile is already first.

Add `GET /api/v1/user/agent-profile-recent-use` and
`PUT /api/v1/user/agent-profile-recent-use/:context`. Extend authenticated boot
state with the compact records and register
`user.agent_profile_recent_use.updated` through the existing user event
broadcaster. Do not change `models.UserSettings`, its JSON serializer, its CAS
revision, or `user.settings.updated`.

### Frontend state and ordering

Add typed recent-use records to the settings slice as a sibling of
`userSettings`. Hydrate boot data, add the focused API client, and accept only
newer per-context mutation responses or WebSocket events. Add a pure stable
ordering helper and let `useAgentProfileOptions` opt into a context. Preserve
the combobox's final selected-first behavior.

Pass `task_create` to standard task and subtask selectors, `task_session` to
new-session and handoff selectors, and `quick_chat` to the quick-chat selector.
Apply the same helper to the configuration-chat profile grid with
`config_chat`. Leave every caller without a context unchanged.

### Success-only recording

Add one best-effort recording entry point that updates the store from the
authoritative response. Invoke it after successful task, subtask, new-session,
handoff, quick-chat, and configuration-chat launch handling. Resolve the
effective response profile before recording, and place recording after the
latest-request check for ephemeral chats so superseded tasks are excluded.
Never await the recording request in primary navigation or launch completion.

## Tests

- `AC-AGENTS-PROFILE-RECENT-USE-003.1` through `.4`: repository and service
  tests in `apps/backend/internal/user/store/sqlite_test.go` and
  `apps/backend/internal/user/service/agent_profile_recent_use_test.go` cover
  user/context isolation, move-to-front, distinctness, cap, CAS retry, no-op
  suppression, and deletion cascade on SQLite and PostgreSQL paths.
- `AC-AGENTS-PROFILE-RECENT-USE-003.1`: handler, boot-state, and broadcaster
  tests cover authenticated scoping, compact hydration, revision fields, and
  user-routed events without `user.settings.updated`.
- `AC-AGENTS-PROFILE-RECENT-USE-001.1` through `.5` and `003.3` through `.5`:
  Vitest coverage for store hydration/event reconciliation and the pure option
  helper covers contextual ranks, stable unseen order, stale IDs, ineligible
  profiles, selected-first composition, and context-free callers.
- `AC-AGENTS-PROFILE-RECENT-USE-002.1` through `.4`: focused launcher tests
  assert that only successful, non-superseded, effective profiles trigger the
  best-effort record call.

## E2E tests

- `AC-AGENTS-PROFILE-RECENT-USE-001.1`, `001.3`, `002.1`, and `003.1`: extend
  `apps/web/e2e/tests/chat/quick-chat.spec.ts` or add
  `apps/web/e2e/tests/chat/agent-profile-recent-use.spec.ts` to launch a quick
  chat with a non-leading profile, reopen the selector, reload, and confirm
  that profile remains ahead of unseen eligible profiles.
- Context isolation, caps, and failure/supersession semantics remain focused
  unit/integration evidence because reproducing them through several complete
  browser launches would add slow duplicate coverage.
- No new mobile Playwright test is required. The change normalizes shared
  selector data only and does not alter the existing mobile composition,
  navigation, overlay, scrolling, safe-area, or touch behavior. Existing mobile
  quick-chat and handoff tests continue to exercise those shared surfaces.

## Work orders

- [x] [Task 01: Persist bounded profile recency](task-01-persist-profile-recency.md)
- [x] [Task 02: Apply contextual selector ordering](task-02-apply-selector-ordering.md)
- [x] [Task 03: Record successful profile launches](task-03-record-successful-launches.md)
- [x] [Task 04: Prove persisted selector recency](task-04-prove-selector-recency.md)

## Verification results

Backend, frontend, and browser verification passed:

- Backend: 1573 tests passed in the user, backend-app, and WebSocket packages.
- Frontend state and selector ordering: 70 tests passed.
- Frontend launch recording: 73 tests passed.
- Frontend typecheck passed.
- Focused Chromium E2E: 1 test passed in 8.1 seconds.

## Risks

- Revision conflict handling must behave identically on SQLite and PostgreSQL.
- A global change inside the shared options hook could accidentally reorder
  configuration selectors; context must remain explicit and optional.
- Quick-chat and configuration-chat supersession checks must precede recording
  to avoid remembering tasks that the frontend deletes.
- Boot-state loading must remain non-fatal and must not add the new records to
  the existing full user-settings payload.
