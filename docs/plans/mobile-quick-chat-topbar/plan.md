---
spec: docs/specs/mobile-quick-chat-topbar/spec.md
created: 2026-07-17
status: complete
---

# Implementation Plan: Mobile Quick Chat Topbar

## Overview

Stack this follow-up on PR #1751 (`feature/quick-chat-on-mobile-wnn`), which already supplies
the mobile and tablet Quick Chat entry points, task-switcher access, and the mobile close
control. Use `PageTopbar`'s leading slot for a workspace-preserving Kandev home link, keep the
Home title out of the mobile root header while respecting existing page-title visibility, and add
focused component and mobile Playwright coverage for the remaining behavior.

## Frontend

- In `apps/web/components/kanban/kanban-header-mobile.tsx`, retain PR #1751's Quick Chat action,
  render the Kandev wordmark as an accessible link using `workspaceHomeHref`, and render the
  existing left-side title/context only for non-Home pages.
- Do not change PR #1751's Quick Chat modal, tablet header, task switcher, state, APIs, or
  responsive breakpoints.

## Backend

No backend changes. The existing Quick Chat launcher and modal contracts are reused.

## Tests

- **What:** the mobile Home header has a workspace-preserving Kandev link, omits the redundant
  Home label, and keeps Quick Chat immediately before Search.
  **File:** `apps/web/components/kanban/kanban-header-mobile.test.tsx`.
  **How:** render with `StateProvider`, expose `PageTopbar` slots through the existing mock, and
  assert accessible roles, link target, absence of Home text, conditional rendering without a
  workspace, and DOM order.
- **What:** a non-Home header retains its page title and workspace label.
  **File:** `apps/web/components/kanban/kanban-header-mobile.test.tsx`.
  **How:** retain and update the existing Tasks-header component test.

## E2E Tests

- **Scenario:** on mobile Tasks, tapping the Kandev wordmark returns to the active workspace's
  Home board.
  **File:** `apps/web/e2e/tests/chat/mobile-quick-chat-entry.spec.ts`.
  **What to verify:** accessible link, resulting `/` route with the active workspace preserved,
  the absence of a redundant Home label, and no horizontal overflow.

## Implementation Task

- [x] [task-01-mobile-topbar](task-01-mobile-topbar.md) (`done`)

## Verification

From `apps/web`:

```bash
pnpm test -- components/kanban/kanban-header-mobile.test.tsx
pnpm e2e:run --host --project mobile-chrome -- tests/chat/mobile-quick-chat-entry.spec.ts
```

From the repository root, after the focused checks pass:

```bash
make fmt
make typecheck
make test
make lint
```

## Risks

- Mobile topbar width is constrained when metrics are enabled; rendered E2E verification must
  confirm the combined stacked header does not overlap, wrap, or introduce horizontal scrolling.
- The home link must preserve the selected workspace rather than relying on a stale global
  workspace preference.
