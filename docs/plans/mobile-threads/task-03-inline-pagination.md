---
id: "03-inline-pagination"
title: "Move pagination into the topbar"
status: done
wave: 3
depends_on:
  - 02-header-and-swipe-cue
plan: "plan.md"
requirements:
  - REQ-UI-THREADS-DECK-003
acceptance_criteria:
  - AC-UI-THREADS-DECK-003.9
  - AC-UI-THREADS-DECK-003.10
  - AC-UI-THREADS-DECK-003.12
system_design:
  - ../../specs/ui/system-design/threads-conversation-deck.md
---

# Task 03: Move pagination into the topbar

## Scope and acceptance

Apply the user's next evaluation feedback: keep dots and position, remove the
instruction text, and move the indicator inline beside the view control without
increasing the topbar height. Remove the extra task-header row. Preserve native
title/session pickers, full-width chat, tablet/desktop behavior, and demo data.

## Design and ownership

Reuse `PageTopbar` and the `MobileColumnTabs` current-context pattern. The board
keeps horizontal scrolling and viewport activation; its header render slot
exposes the active mobile task ID. The page derives position from stable order.
No extra observer, copied selection state, persisted preference, or new gesture
handler. Numeric dots are noninteractive; existing actions retain 44px targets.
The inline surface keeps orientation visible while restoring vertical chat space.

Owned files: Threads page, board, mobile column header, mobile pagination,
Threads-only topbar test ID, localized label removal, and mobile E2E coverage.

## Validation

First fail the mobile regression on the missing topbar indicator. Then verify
same-row geometry, unchanged 56px topbar, reclaimed task-header space, active
dots after swipes/picker navigation, and no indicator for empty/single-thread or
tablet states. Run:

```bash
make build-web
cd apps/web
pnpm test components/threads/ lib/threads/ app/threads/threads-page-client.test.tsx
pnpm run typecheck
pnpm run i18n:check
pnpm e2e:raw tests/task/mobile-threads-view.spec.ts tests/kanban/mobile-kanban-topbar.spec.ts tests/plugins/mobile-plugin-topbar.spec.ts --project=mobile-chrome --retries=0
pnpm e2e:raw tests/task/threads-view.spec.ts --project=chromium --retries=0
```

Run scoped ESLint, formatting, spec lint, public docs validation, and a dark
360px demo browser check. Refresh the existing private Tailscale demo; do not
reseed it. Risks: stale position, title pressure, or hidden header in empty state.

## Parallelism

`sequential`

## Results

- RED: the existing build failed the new regression because the page topbar
  had no pagination indicator.
- GREEN: 162 focused unit tests and all 22 browser regressions passed with
  one worker and no retries. Six mobile Threads tests prove live position after
  picker selection and real swipes, inline control alignment, reclaimed header
  height, and phone/tablet transitions. Four shared mobile/plugin tests and all
  12 desktop Threads tests passed.
- An initial geometry assertion included the topbar's bottom border, placing
  its center half a pixel below every control. The final assertion compares
  actual control centers and separately checks containment and topbar height.
- Build, typecheck, scoped zero-warning lint, formatting, translations, spec
  lint, and public documentation checks passed. No backend rebuild or schema
  change was needed.
- Dark 360px demo inspection measured the unchanged 56px topbar, a task-header
  reduction from 139 to 111px, and a 360px chat/document width. Multi-agent
  controls remain contained. Screenshots: `mobile-inline-pagination-dark.png`
  and `mobile-inline-pagination-agents.png` under the demo runtime directory.
- The existing demo database and private Tailscale route remain intact. HTTPS
  health and a TLS-verified WebSocket upgrade (101) passed. The temporary
  inspection browser was closed; the demo remains running. No commit or
  publication was made. Physical-device keyboard and Safari remain untested.
