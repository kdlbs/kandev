---
spec: docs/specs/tasks/quick-chat-expiration.md
created: 2026-07-14
status: implemented
---

# Implementation Plan: Typed Quick Chat Sessions

## Overview

Unify ordinary and configuration utility chats in the existing Quick Chat modal. The backend first
restores both kinds with a metadata-derived discriminator; the frontend then replaces the separate
config tab store/popover with typed setup and session tabs, and the shared clarification region gains
a collapse control. Focused unit/integration tests precede production changes, followed by desktop
and mobile Playwright coverage and full verification.

## Backend

### Boot restoration contract

- Update `apps/backend/internal/backendapp/boot_state.go` so the boot query includes config-mode
  workflow-less ephemeral tasks while still excluding automation-run and workflow-bound tasks.
- Add `kind` to each mapped session. Derive `config` only from `task.Metadata[MetaKeyConfigMode]`;
  all other eligible tasks map to `chat`.
- Keep active-workspace filtering, primary-session hydration, newest-activity-first ordering, and
  closed/unset modal state.
- Extend `apps/backend/internal/backendapp/helpers_test.go` with ordinary/config kind, ordering,
  exclusion, primary-session hydration, and workspace-boundary assertions.

## Frontend

### Typed state and hydration

- Define `QuickChatSessionKind` and `QuickChatSession` in
  `apps/web/lib/state/slices/ui/types.ts`; remove the independent config-chat state/actions.
- Extend Quick Chat actions to open typed real/setup sessions and guard workspace activation.
- Normalize absent boot kinds to `chat` in `apps/web/lib/state/default-state.ts` and
  `apps/web/lib/state/hydration/hydrator.ts` for backward compatibility.
- Cover store actions and hydration in focused Vitest tests.

### Unified launch, setup, and lifecycle

- Refactor the config launcher under `apps/web/components/config-chat/` to open a `config` setup tab
  in Quick Chat; remove the fixed popover and second conversation/tab renderer.
- Move config setup presentation into the Quick Chat modal, preserving the configuration profile,
  introduction, suggestions, placeholder, default-profile update, config endpoint, task-session seed,
  passthrough detection, and send-after-subscription initial-prompt behavior.
- Extend `use-quick-chat-modal.ts`, `quick-chat-modal.tsx`, and tab components for typed sessions,
  accessible config indicators, kind-specific content/toolbar, one explicit delete lifecycle, blank
  setup cleanup, and workspace changes. Keep the existing `+` action creating an ordinary setup tab.
- Update Settings FAB and Command Palette launch paths to the same typed modal action.

### Clarification presentation

- Keep clarification inline in `QuickChatContent`, bounded and scrollable above the composer.
- Add an accessible collapse/expand affordance that preserves the pending question and restores the
  previous region without hiding message history or primary actions on mobile.
- Add focused tests for any extracted state/helper logic and rely on Playwright for rendered layout.

## Tests

- **Boot kind and ordering:** `apps/backend/internal/backendapp/helpers_test.go`; SQLite-backed boot
  payload test with ordinary/config tasks and different activity times.
- **Exclusions and workspace scope:** same file; automation-run, workflow-bound, missing-primary,
  archived, and foreign-workspace tasks do not restore.
- **Backward-compatible hydration:** `apps/web/lib/state/hydration/hydrator.test.ts` and/or
  `apps/web/lib/state/default-state.test.ts`; missing `kind` becomes `chat`.
- **Typed store actions:** existing/new UI slice tests; config/chat setup and real sessions coexist,
  cross-workspace activation is rejected, and blank close is client-only.
- **Launch/delete/initial prompt:** `apps/web/components/quick-chat/use-quick-chat-modal.test.ts` and
  config launch hook tests; config endpoint/session seeding, exactly-once prompt delivery, passthrough
  lookup, and confirmed task deletion.

## E2E Tests

- Replace the fixed-popover expectations in
  `apps/web/e2e/tests/settings/config-chat-popover.spec.ts` with a desktop unified-modal flow:
  Settings or Command Palette launch, config indicator, send/response, refresh/reopen, continue, and
  confirmed deletion.
- Add clarification coverage proving the message list remains visible and the bottom region can be
  collapsed, expanded, resized/scrolled, and answered.
- Add `apps/web/e2e/tests/settings/mobile-configuration-chat.spec.ts` for full-screen launch, config
  indicator, clarification controls, and unclipped primary actions.
- Re-run ordinary repository-backed Quick Chat desktop/mobile specs from PR #1679.

## Implementation Waves

Wave 1 (parallel after approval):

- [x] [Task 01: Backend boot contract](task-01-backend-boot-contract.md) (`done`)
- [x] [Task 03: Clarification responsive UX](task-03-clarification-responsive-ux.md) (`done`)

Wave 2:

- [x] [Task 02: Unified frontend sessions](task-02-unified-frontend-sessions.md) (`done`)

Wave 3:

- [x] [Task 04: E2E and verification](task-04-e2e-and-verification.md) (`done`)

## Verification

Targeted commands and results are recorded in each task. Final repository verification ran from the
repository root in the required order: `make fmt`, `make typecheck`, `make test`, `make lint`.
Affected E2E runs used the managed runner from `apps/web`, including desktop config/ordinary Quick
Chat and the `mobile-chrome` configuration/repository specs.

## Risks

- A blank tab is keyed by an empty session ID; typed setup actions must replace or close the existing
  placeholder deterministically instead of creating colliding React/store keys.
- Config initial prompts must remain component/session scoped so hydration or tab switching cannot
  replay them.
- Failed task deletion must remain recoverable on reload; client removal cannot become the only
  record of a persistent config task.
- Boot hydration and providers must always scope sessions to the active workspace.
