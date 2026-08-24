---
spec: docs/specs/ui/requirements/settings-manual-save.md
decision: docs/decisions/0046-settings-route-save-coordinator.md
created: 2026-08-09
status: complete
---

# Implementation Plan: Centered settings save surface

## Overview

Refine the existing route-scoped settings save action into a compact, centered
single-row save surface inspired by the supplied reference. The coordinator
remains the owner of draft persistence and navigation protection; the change
adds a shared local Reset action and changes only the presentation and
responsive placement.
The surface is centered in the settings content pane when Configuration Chat
is closed and uses the existing chat action host above the popover when it is
open.

No backend, API, schema, or persistence changes are required.

---

## Backend

No backend changes. Existing contributor save and discard callbacks remain the
persistence boundary.

---

## Frontend

### Coordinator reset contract

- Update `apps/web/components/settings/settings-save-provider.tsx` to factor the
  existing contributor-discard loop into a reusable route-local reset callback.
- Keep navigation's `Discard and leave` behavior unchanged, while allowing the
  save surface to invoke the same contributors without a pending navigation.
- Disable Reset and Save while a reset or save is in flight. A reset failure
  keeps the route dirty and reports a dedicated localized reset error.
- Preserve stable contributor order, partial-save behavior, revision safety,
  validation disabling, and the 1.5-second successful-save confirmation.

### Centered save surface

- Update `apps/web/components/settings/settings-floating-save.tsx` to render a
  compact neutral card/pill with the translated `Unsaved changes` label, a
  secondary `Reset` action, and the existing `Save changes`/`Retry save` action.
- Remove the oversized right-anchored green rectangle. Keep success-green
  emphasis on the primary action and use the existing status icon/copy for
  saving, saved, invalid, and error states.
- Center the standalone surface against the settings content pane, with a
  roughly 20px desktop bottom inset plus safe-area clearance. On phones, lift
  the surface above the Configuration Chat FAB while preserving the same
  centered composition. Keep the surface keyboard accessible, preserve
  `data-testid="settings-floating-save"`, and retain the navigation dialog as a
  separate modal flow.
- Keep the desktop surface near 40px tall by using compact controls and minimal
  outer padding; retain 44px control hitboxes on phone widths.
- Keep the mobile composition intentional: the surface uses an intrinsic-width
  single-row card on phone widths, keeps the message flexible without
  horizontal overflow, and keeps every primary/secondary control at least 44px
  in its active dimension. Adjust the settings scroll container's bottom
  padding to the compact surface height while preserving safe-area clearance.

### Configuration Chat collision handling

- Update `apps/web/components/config-chat/config-chat-panel.tsx` so the existing
  floating-actions host spans the chat popover width and centers the portaled
  save surface above it. Preserve the host portal rather than allowing the
  viewport-centered surface to intersect the open chat.

### Localization and design-system boundaries

- Reuse existing localized keys (`common:unsavedChanges`, `settings:reset`,
  `settings:saveChanges`, `settings:retrySave`, and current status keys); do not
  add hardcoded UI copy or new backend contracts.
- Reuse `@kandev/ui/button`, existing success/destructive tokens, safe-area
  utilities, and `useResponsiveBreakpoint`/the canonical `md` boundary where a
  responsive branch is required.

### Mobile parity contract

- Desktop outcome: a dirty settings route exposes one centered, content-pane-
  reachable save surface; Save persists all dirty contributors and Reset
  restores their baselines.
- Mobile entry point: the same bottom-inset surface remains visible at the lower
  center of a 390px viewport, with an intrinsic-width composition and no
  hover-only interaction.
- Nearest shipped exemplars: `apps/web/components/kanban/mobile-fab.tsx` for
  safe-area-aware fixed action geometry and 44px touch ergonomics, and
  `apps/web/components/config-chat/config-chat-panel.tsx` for the existing
  chat-hosted collision escape hatch. These contribute geometry/placement only;
  settings state and actions remain shared in the coordinator.
- Scroll owner: `settings-scroll-container` remains the single vertical scroll
  owner. The content shell reserves the reduced space needed for the
  Configuration Chat FAB and phone save clearance, while the absolute save
  surface uses the settings main content pane and a roughly 20px desktop bottom
  inset.

---

## Tests

- **What:** Reset invokes every dirty contributor, makes no persistence request,
  hides the surface after successful discard, disables duplicate reset/save
  submissions, and retains dirty state on discard failure.
  **File:** `apps/web/components/settings/settings-save-provider.test.tsx`.
  **How:** focused React Testing Library tests with synchronous and deferred
  contributor callbacks.
- **What:** The coordinator-rendered surface exposes the unsaved label, Reset,
  Save changes, Retry save, invalid, saving, and saved states with the new
  neutral/centered layout hooks and accessible names.
  **File:** `apps/web/components/settings/settings-save-provider.test.tsx`.
  **How:** focused React Testing Library tests through `SettingsSaveProvider`;
  rendered geometry and the Configuration Chat-hosted branch are covered by
  Playwright.
- **What:** Existing navigation protection, partial failure, retry, in-flight
  revision, and safe-area layout tests remain green.
  **Files:** existing settings save/layout tests.
  **How:** run the focused settings component test set.

---

## E2E Tests

- **Scenario:** GIVEN a dirty desktop settings route, WHEN the action appears,
  THEN the surface is centered in the settings content pane, uses a restrained
  neutral container near 40px tall, exposes Reset and Save changes, and Reset
  returns the form to its baseline without an API write.
  **File:** `apps/web/e2e/tests/settings/settings-manual-save.spec.ts`.
- **Scenario:** GIVEN a dirty 390px settings route, WHEN the user scrolls to the
  final field, THEN the centered bottom-inset surface stays within the viewport, controls
  have touch-sized hitboxes, the last field is not covered, and document
  horizontal overflow remains zero.
  **File:** `apps/web/e2e/tests/settings/mobile-general-settings.spec.ts`.
- **Scenario:** GIVEN Configuration Chat is open on a dirty settings route,
  THEN the portaled surface is centered over the chat-safe host, sits above the
  popover, and does not intersect it on desktop or mobile.
  **Files:** `apps/web/e2e/tests/settings/config-chat-popover.spec.ts` and
  `apps/web/e2e/tests/settings/mobile-config-chat-popover.spec.ts`.
- Capture refreshed desktop and mobile screenshots for visual review while
  keeping the existing deterministic API cleanup in each test.

---

## Risks

- The message plus two actions may become too wide in pseudo-locale or on a
  narrow phone; flexible message sizing and compact-surface geometry assertions
  must cover both.
- The Configuration Chat host is portaled into a positioned popover; changing
  its width/alignment can regress the existing above-popover collision contract.
- Reset must not replace saved baselines or issue API calls, and must not clear a
  newer revision that changes while a save is in flight.
- Existing settings routes can have several contributors; reset and save must
  use the same stable contributor ordering and error semantics.

---

## Out of Scope

- Backend settings schemas, HTTP contracts, persistence semantics, or
  multi-contributor atomicity.
- Replacing named operational commands or dialog/sheet-local Create, Save, or
  Delete actions.
- Redesigning Configuration Chat itself or changing the settings navigation
  guard contract.

---

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Centered save surface and reset](task-01-centered-save-surface.md) (done)

Wave 2:

- [x] [Task 02: Rendered desktop and mobile coverage](task-02-save-surface-e2e.md) (done)

Tasks execute sequentially by default. Task 02 depends on Task 01 because its
selectors and geometry assertions target the completed surface.

---

## Verification Results

- `cd apps && pnpm --filter @kandev/web test -- --run components/settings/settings-save-provider.test.tsx` — 16 tests passed.
- `cd apps/web && pnpm run typecheck` — passed.
- `cd apps/web && pnpm run lint` — passed.
- `cd apps/web && pnpm run i18n:check` — passed; retained the repository's existing advisory locale parity findings.
- `cd apps/web && pnpm run i18n:ratchet` — passed with zero new-code violations.
- `cd apps/web && pnpm e2e:run --host --no-build tests/settings/settings-manual-save.spec.ts tests/settings/config-chat-popover.spec.ts` — 11 desktop tests passed.
- `cd apps/web && pnpm e2e:run --project mobile-chrome --no-build tests/settings/mobile-general-settings.spec.ts tests/settings/mobile-config-chat-popover.spec.ts` — 6 mobile tests passed after the final frontend build.
- `cd apps/web && CAPTURE_PR_ASSETS=true pnpm e2e:raw ...` — 17 desktop/mobile tests passed; `.pr-assets/manifest.json` contains refreshed desktop and mobile screenshots.
- `git diff --check` — passed.

## Open Questions

None. The reference's Reset action maps to Kandev's route-local contributor
discard semantics, while the primary label remains the existing localized Save
changes copy.
