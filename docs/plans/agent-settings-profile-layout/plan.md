---
spec: docs/specs/agents/requirements/settings-profile-layout.md
created: 2026-08-13
status: building
---

# Implementation Plan: Simplify the agent settings profile layout

## Overview

Move profile creation into each installed-agent card header and remove the
redundant profile-count/action row from the card body. Reorder the existing
installed-agent toolbar so refresh/rescan is immediately before the rightmost
agent-creation action. Preserve the current profile rows and agent-level
controls, then prove the layout and navigation on desktop and mobile.

The change is frontend-only. No API, store, persistence, or locale contract is
needed because the existing translated labels and routes are reused.

## Frontend

### Agent profile sub-list

- `apps/web/components/settings/agents/agent-profiles-section.tsx`
  - Remove the profile-count/empty-state header row and its creation link.
  - Keep the existing profile rows and their actions in the card body.
  - Avoid rendering an empty profile body when no saved profiles exist; the
    card header supplies the setup action.

### Installed-agent card

- `apps/web/components/settings/installed-agent-card.tsx`
  - Add the existing `New profile` link to the configured agent's header,
    alongside agent-level controls, using the existing `?mode=create` route.
  - Keep `Setup profile` as the only header creation action for an agent with
    no saved record.
  - Preserve touch-sized header actions on mobile and the existing status,
    authentication, and runtime-update controls.

### Installed-agent toolbar

- `apps/web/app/settings/agents/page.tsx`
  - Reorder the current header controls so the refresh/rescan button is
    immediately before the existing Add TUI Agent action and the latter is
    rightmost.
  - Add stable test IDs for the toolbar and the two ordered controls if the
    existing markup does not provide a reliable scoped selector.
  - Keep the terminal action available and keep the current agent-creation
    behavior and label.

## Tests

- **What:** A configured agent shows profile creation in the card header,
  profile rows have no count/action sub-header, and an unconfigured agent has
  only its setup action.
  **File:** `apps/web/components/settings/agents/agent-profiles-section.test.tsx`
  and `apps/web/components/settings/installed-agent-card.test.tsx`.
  **How:** React Testing Library assertions against the rendered card/list,
  including the creation link target and absence of count/empty-state copy.
- **What:** Existing profile-row duplicate/delete behavior remains intact.
  **File:** `apps/web/components/settings/agents/agent-profiles-section.test.tsx`
  **How:** Keep and rerun the current store read-after-write tests.
- **What:** The installed-agent toolbar keeps terminal available and orders
  refresh/rescan directly before the rightmost agent-creation action.
  **File:** `apps/web/e2e/tests/settings/agent-profile-layout.spec.ts`
  **How:** Navigate through the real settings page, scope the toolbar by its
  test ID, assert the ordered controls, and verify the configured card's
  **New profile** link route.

## E2E Tests

- **Scenario:** GIVEN a configured agent with one or more profiles, WHEN the
  user opens `/settings/agents`, THEN the card header contains **New profile**,
  profile rows are directly below it, and no profile-count/action row exists.
  **File:** `apps/web/e2e/tests/settings/agent-profile-layout.spec.ts`.
- **Scenario:** GIVEN a phone viewport below 768px, WHEN the user opens the
  agents settings page, THEN the same creation action and ordered toolbar are
  touch-reachable and the document has no horizontal overflow.
  **File:** `apps/web/e2e/tests/settings/mobile-agent-profile-layout.spec.ts`.
  **What to verify:** Use the configured `mobile-chrome` project, assert the
  header action hitbox is at least 44px, use `.tap()` for the creation action,
  and assert the settings document remains contained.

## Mobile design contract

- Desktop outcome: manage an agent's saved profiles and create another profile
  from the same agent card.
- Mobile entry point: the installed-agent card on `/settings/agents`; no new
  drawer, route, or mobile-only control is needed because the action is a
  direct link and the existing settings scroll container remains the sole
  vertical scroll owner.
- Nearest shipped exemplar: the managed runtime update controls on the same
  agent card, whose mobile E2E coverage verifies 44px controls and no document
  overflow. Reuse its touch-sized action treatment and existing card geometry.
- Mobile hierarchy: agent identity/status first, creation and agent-level
  controls in the header, then profile rows in one vertical list.
- Presentation: inline card-header action; no drawer is justified because
  profile creation is a direct destination rather than a temporary choice.
- Shared state and logic: reuse the existing agent/profile store data and route
  handlers; only the placement and responsive classes change.

## Verification Results

Passed.

- Component regression coverage passed: `cd apps/web && pnpm test -- components/settings/agents/agent-profiles-section.test.tsx components/settings/installed-agent-card.test.tsx` (8 tests).
- Static checks passed: `cd apps/web && pnpm run typecheck`, the focused ESLint command from task 01, and `cd apps/web && pnpm run i18n:check` (with the repository's existing advisory catalog warnings).
- Desktop managed E2E passed: `cd apps/web && pnpm e2e:run --project chromium tests/settings/agent-profile-layout.spec.ts` (2 tests).
- Mobile managed E2E passed: `cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-agent-profile-layout.spec.ts` (1 test).
- Review follow-up coverage passed for both setup routes, the saved-empty profile body guard, and the toolbar ordering invariant.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [task-01-agent-settings-layout](task-01-agent-settings-layout.md)

Wave 2:

- [x] [task-02-agent-settings-e2e](task-02-agent-settings-e2e.md)

The tasks are sequential because the E2E selectors and layout assertions
depend on the component markup from task 01.

## Open Questions

- None. The existing **Add TUI Agent** control is treated as the requested
  agent-creation action; its semantics and translated label remain unchanged.
