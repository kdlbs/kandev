---
id: "01-hover-reveal"
title: "Implement sidebar hover reveal"
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-UI-SIDEBAR-HOVER-001
acceptance_criteria:
  - AC-UI-SIDEBAR-HOVER-001.1
  - AC-UI-SIDEBAR-HOVER-001.2
  - AC-UI-SIDEBAR-HOVER-001.3
  - AC-UI-SIDEBAR-HOVER-001.4
  - AC-UI-SIDEBAR-HOVER-001.5
system_design:
  - ../../specs/ui/system-design/sidebar-hover-reveal.md
---

# Task 01: Implement Sidebar Hover Reveal

## Summary

Implement the delayed overlay as one vertical slice using TDD. Preserve saved collapse,
existing navigation actions, explicit controls, and touch access.

## In scope

Hook lifecycle, sidebar/header composition, owned portal interaction and focused tests.

## Out of scope

Backend changes, new preferences, mobile redesign, resize during temporary reveal.

## Acceptance

- Delayed reveal and cancellation satisfy AC .1/.3/.5, including owned portals and focus.
- Visual expansion preserves saved layout and explicit toggle semantics (AC .2/.4).
- Desktop and mobile rendered tests prove usable navigation and stable geometry (AC .1-.5).

## ASCII UI preview

UI-01: Desktop, pointer enters collapsed rail (AC .1-.4).

```text
Before 500 ms             After 500 ms
[rail] [page content]     [full sidebar  ] over page
                         [workspace    ]
                         [navigation   ]
                         [Expand button]
```

The 56 px layout reservation and page/status position stay fixed. The sidebar's
existing task/navigation scroll regions remain scroll owners; header/footer stay
fixed. Exit closes the overlay; explicit Expand keeps it open. Full navigation
and unchanged page geometry are structural; label positions here are illustrative.

UI-02: Phone, existing menu trigger (AC .5).

```text
[Menu] [current page]
        tap Menu
+---------------------+
| Navigation / Tasks  |
| destinations        | <- internal scrolling
| select destination  |
+---------------------+
    safe-area space
```

Use the existing inset drawer and localized copy. No collapsed rail or hover trigger
appears on phone. Desktop checks target UI-01; mobile checks target UI-02.

Full preview: [plan](plan.md#ascii-ui-preview).

## Verification

Run sequentially from repository root. New tests listed here are implementation outputs.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/domains/sidebar/use-sidebar-hover-reveal.test.ts components/app-sidebar/app-sidebar.test.tsx components/app-sidebar/app-sidebar-header.test.tsx components/app-sidebar/app-sidebar-workspace-picker.test.tsx lib/state/slices/ui/app-sidebar-actions.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint hooks/domains/sidebar/use-sidebar-hover-reveal.ts components/app-sidebar/app-sidebar.tsx components/app-sidebar/app-sidebar-header.tsx)
(cd apps/web && pnpm run i18n:check)
GOCACHE=/tmp/kandev-sidebar-go-cache make -C apps/backend build
make build-web-e2e
(cd apps/web && pnpm e2e:run --host --no-build --project chromium e2e/tests/layout/sidebar-hover-reveal.spec.ts e2e/tests/layout/toggle-sidebar-shortcut.spec.ts e2e/tests/layout/sidebar-resize-handle.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/layout/mobile-sidebar-hover-reveal.spec.ts -- --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

- `apps/web/hooks/domains/sidebar/use-sidebar-hover-reveal.ts` and `.test.ts` (new).
- `apps/web/components/app-sidebar/app-sidebar.tsx` and `.test.tsx`.
- `apps/web/components/app-sidebar/app-sidebar-header.tsx` and `.test.tsx`.
- Sidebar-owned popover/menu components only as required for scoped interaction ownership.
- `apps/web/e2e/tests/layout/sidebar-hover-reveal.spec.ts` and
  `mobile-sidebar-hover-reveal.spec.ts` (new).

## Dependencies

None. Install workspace dependencies before package commands if absent.

## Risks

Portals and focus can outlive pointer containment. Suppress re-entry caused by the
same collapse interaction. Do not broadly capture unrelated application overlays.

## Parallelism

Sequential. No subagents authorized.

## Inputs

[Requirements](../../specs/ui/requirements/sidebar-hover-reveal.md),
[design](../../specs/ui/system-design/sidebar-hover-reveal.md), existing sidebar unit
fixtures and layout E2E tests. Apply repository TDD and mobile-parity skills.

## Results

Completed on 2026-09-14.

| Verification | Result |
| --- | --- |
| Workspace frozen-lockfile install | Passed |
| Five targeted Vitest files | 58 tests passed |
| Typecheck, targeted ESLint, i18n check | Passed; no lint warnings |
| Backend build and `make build-web-e2e` | Passed |
| Chromium hover, shortcut and resize specs | 9 tests passed, retries disabled |
| Mobile navigation spec | 1 test passed, retries disabled |
| Specification catalog and full specification lint | Passed |
| Public-doc validator tests and published-page validation | Passed (46 pages) |
| `git diff --check` | Passed |

Browser RED was confirmed against the original frontend: the panel stayed 56 px
instead of revealing to 320 px. Final browser checks prove stable page geometry,
owned menu interaction, nested Escape/focus return, persistent expansion, task
navigation, responsive cancellation and phone tap navigation. Desktop and phone
screenshots were inspected and matched the planned compositions.

The fresh-worktree build used `GOCACHE=/tmp/kandev-sidebar-go-cache` because the
default cache was read-only. Browser runs required permission to bind the isolated
backend's local port. Backend artifacts were reused after frontend-only edits;
frontend assets were rebuilt before each affected browser run. Build output retained
existing chunk-size and dynamic-import warnings.
