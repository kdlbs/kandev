---
spec: docs/specs/ui/requirements/clarification-context.md
created: 2026-08-13
status: shipped
---

# Implementation Plan: Clarification Context Rendering

## Overview

Render the shared `ask_user_question_kandev.context` value that already reaches
every clarification message but is currently ignored by the pending overlay.
The confirmed root cause is confined to
`ClarificationInputOverlay`: the MCP handler, clarification service, persisted
message metadata, WebSocket delivery, and frontend metadata type all preserve
the field, while the component renders only question-level fields.

The repair derives a trimmed bundle-level context from the first sorted message
and renders it once between the overlay header and question carousel. It does
not change backend contracts, persistence, state, answer submission, or
resolved transcript rendering.

## Backend

No backend change is planned. The existing path already forwards context from
`apps/backend/internal/mcp/server/handlers.go` through
`apps/backend/internal/mcp/handlers/handlers.go` and persists it in each
clarification message from `apps/backend/internal/backendapp/adapters.go`.

## Frontend

### Pending clarification overlay

- Update `apps/web/components/task/chat/clarification-input-overlay.tsx` to
  read the shared context from the first sorted message.
- Trim only for the empty-value decision. Render non-empty content once at the
  bundle level, outside `ClarificationCard`, so carousel navigation cannot
  duplicate or remove it.
- Present the context as normal text with top spacing and no bordered, padded,
  or filled container.
- Preserve authored line breaks and wrap long words inside the overlay. Add a
  stable `clarification-context` test ID.
- Do not translate the value. It is agent-authored domain content, not UI copy.

### E2E page object

- Add a `clarificationContext()` locator to
  `apps/web/e2e/pages/session-page.ts`, scoped to the active clarification
  overlay.

### Mobile design contract

This is a content-only addition to the existing clarification overlay. The
nearest shipped phone exemplar is
`apps/web/e2e/tests/chat/mobile-clarification.spec.ts`, which exercises the
same shared overlay on the `mobile-chrome` Pixel 5 project. Composition,
navigation, actions, scroll ownership, safe areas, and touch targets remain
unchanged. The context stays above the carousel, wraps within the current
single scroll owner, and must not create document-level horizontal overflow.

## Tests

No new pure function or backend behavior is introduced. A browser test is the
smallest faithful regression because it proves the existing MCP-to-message
transport and the missing React rendering boundary together.

- **What:** shared context is visible exactly once and remains visible across
  multi-question carousel navigation.
  **File:** `apps/web/e2e/tests/chat/clarification.spec.ts`.
  **How:** use the existing `clarification-multi` mock-agent scenario, which
  already sends distinctive context, assert one context region before and
  after moving to the next question, and prove it is outside the question card.
- **What:** omitted context does not create an empty context region.
  **File:** `apps/web/e2e/tests/chat/clarification.spec.ts`.
  **How:** extend the existing single-question `clarification` happy path,
  whose mock call omits context, with a zero-count assertion.

## E2E Tests

- **Scenario:** **GIVEN** a multi-question MCP clarification with shared
  context, **WHEN** the task chat renders the pending bundle, **THEN** context
  appears exactly once above the carousel and persists across navigation.
  **File:** `apps/web/e2e/tests/chat/clarification.spec.ts`.
- **Scenario:** **GIVEN** the same bundle on a phone viewport, **WHEN** the
  operator views the pending clarification, **THEN** context remains visible,
  contained, and does not add horizontal page overflow.
  **File:** `apps/web/e2e/tests/chat/mobile-clarification.spec.ts`.

## Verification Results

- RED desktop: `cd apps/web && pnpm e2e:run tests/chat/clarification.spec.ts -- --grep
"shared context"` discovered 1 test and failed because the new context locator
  resolved to 0 elements before the renderer change. The managed runner built the
  production backend/web assets and completed teardown.
- RED mobile: `cd apps/web && pnpm e2e:run --project mobile-chrome
tests/chat/mobile-clarification.spec.ts -- --grep "shared context"` discovered 1
  test and failed at the same missing-render assertion. The managed runner built
  the production assets and completed teardown.
- GREEN desktop: the exact desktop command passed 1 targeted test against a fresh
  production build.
- GREEN mobile: the exact mobile command passed 1 targeted test against a fresh
  production build.
- `pnpm run typecheck` — passed.
- `pnpm exec eslint --max-warnings 0 components/task/chat/clarification-input-overlay.tsx
e2e/pages/session-page.ts e2e/tests/chat/clarification.spec.ts
e2e/tests/chat/mobile-clarification.spec.ts` — passed.
- `pnpm run i18n:ratchet` — passed with 0 added and 1 modified file clean.
- `git diff --check` — passed.
- PR asset capture: a disposable production-build E2E captured and visually
  inspected the desktop and phone states; both screenshots were non-empty and
  compressed with `pngquant-bin` for PR media.
- PR fixup: after the requested 15-minute review window, the active-chat page
  object scope and independent task verification commands were corrected in
  response to valid minor review comments. The focused desktop and mobile E2E
  commands passed again against fresh production builds.
- Visual refinement follow-up: the context now has 12px top spacing and normal
  text presentation with no padding, border, or background. The updated desktop
  and mobile focused tests passed against fresh production builds.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Render clarification context](task-01-render-clarification-context.md) - done

The task is sequential because the E2E assertions, page object locator, and UI
change form one Red-Green-Refactor slice. No subagent is planned or authorized.

## Risks

- Every message in a bundle currently carries the same context. Reading the
  first sorted message preserves the existing contract and avoids duplication;
  malformed bundles with inconsistent values remain outside this repair.
- Long agent-authored content can increase overlay height. The existing chat
  overlay owns vertical scrolling, while wrapping and the mobile E2E check
  protect narrow layouts from horizontal overflow.

## Out of Scope

- Backend, MCP schema, WebSocket, or database changes.
- Rendering context in resolved clarification history.
- Refactoring the clarification carousel or answer state.
