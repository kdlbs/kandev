---
spec: docs/specs/ui/requirements/task-layout-profiles.md
created: 2026-07-28
status: completed
---

# Implementation Plan: Responsive Dockview Right-Column Width

## Overview

Separate the right column's automatically calculated default from an explicit
desktop sash resize. Existing environment layouts contain only serialized
Dockview geometry, so their right width must be treated as responsive by
default. A new environment-scoped manual-width marker will preserve a raw
user-requested right width across reloads and display changes.

## Confirmed Root Cause

Every persisted right-column size is currently forwarded from the serialized
environment layout to `resolveRightTarget`. That value wins over the ratio
default, so a 450px automatic large-display width is restored unchanged on a
laptop. The live-width synchronizers also place automatic widths in
`pinnedWidths`, making later layout operations treat them as overrides.

## Frontend

### Manual-width persistence

- Add a versioned, session-scoped companion key in
  `apps/web/lib/local-storage.ts` for a raw manual right-column width per
  environment. Legacy layouts have no marker and therefore remain responsive.
- Record the marker only after the existing genuine right-sash mouseup path in
  `apps/web/components/task/dockview-layout-setup.ts`. Do not record it for
  `ResizeObserver`, `api.layout`, restore, or programmatic layout changes.

### Restore and resize resolution

- Update `apps/web/lib/state/dockview-layout-builders.ts`,
  `apps/web/lib/state/dockview-env-switch.ts`, and
  `apps/web/lib/state/dockview-store.ts` so the right target is the manual
  marker when present; otherwise it is `getPinnedWidth` for the current
  measured Dockview width.
- On container resize, recalculate the automatic target before enforcing it.
  Keep a manual raw width stored even when it is temporarily clamped by the
  laptop cap, so it returns on a larger display.
- Keep task-specific panel structure, user layout profiles, the global left
  sidebar preference, and the existing runtime caps unchanged.

## Tests

- **What:** a serialized legacy/automatic 450px right column resolves to the
  current ratio rather than 450px. **Files:**
  `apps/web/lib/state/dockview-layout-builders-fixups.test.ts` and
  `apps/web/lib/state/dockview-env-switch-pinned.test.ts`.
- **What:** a genuine right-sash resize creates a manual marker and that marker
  wins across restore and fast environment switches. **Files:**
  `apps/web/components/task/dockview-layout-setup.test.ts` (or a focused new
  sibling test) plus `apps/web/lib/local-storage.test.ts`.
- **What:** automatic layout events do not become manual overrides. **Files:**
  `apps/web/lib/state/dockview-store.test.ts`.

## E2E Tests

- **Scenario:** a fresh desktop task moves large → laptop → large and the
  right Files/Changes/Terminal column follows the default ratio at each width.
  **File:** `apps/web/e2e/tests/layout/pane-resize-right.spec.ts`.
- **Scenario:** a manually resized right column moves large → laptop → large;
  it is clamped only on the laptop and returns to the requested width on the
  large display. **File:** `apps/web/e2e/tests/layout/pane-resize-right.spec.ts`.

## Mobile and Tablet

`TaskLayout` selects `SessionMobileLayout` and `SessionTabletLayout` before
the desktop Dockview workbench. No phone or tablet geometry or interaction
changes. The nearest mobile exemplar is
`apps/web/components/task/mobile/session-mobile-layout.tsx`; it keeps its
existing single-panel navigation and scroll ownership. The desktop browser
tests are the rendered verification for this desktop-only path.

## Implementation Waves

Wave 1 (sequential):

- [x] [task-01-responsive-right-width](task-01-responsive-right-width.md)

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run \
  lib/state/dockview-layout-builders-fixups.test.ts \
  lib/state/dockview-env-switch-pinned.test.ts \
  lib/state/dockview-store.test.ts
cd apps/web && pnpm e2e:run tests/layout/pane-resize-right.spec.ts
cd apps/web && pnpm run typecheck
```

## Risks

- Automatic layout changes must never create a manual marker, or a monitor
  switch will become sticky again.
- The raw manual width must survive a temporary cap without persisting the
  capped value as the user preference.
- Legacy persisted layouts lack intent metadata; treating them as automatic is
  required to repair the reported issue, but it deliberately changes their
  prior width-restoration behavior.

## Documentation Impact

No public documentation changes are required. The affected internal product
spec is updated in `docs/specs/ui/requirements/task-layout-profiles.md`. This is a local
layout-persistence repair, so no ADR is warranted.
