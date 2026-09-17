---
created: 2026-09-13
status: implemented
requirements:
  - REQ-UI-SIDEBAR-HOVER-001
  - REQ-UI-SIDEBAR-HOVER-002
system_design:
  - ../../specs/ui/system-design/sidebar-hover-reveal.md
legacy_specs: []
---

# Implementation Plan: Sidebar Hover Reveal

## Overview

The completed first work order delivers a temporary 500 ms hover reveal and desktop/phone
regression coverage. The implementation uses local hover state and preserves the saved sidebar preference.

## Scope

Global sidebar hover interaction, overlay geometry, explicit expansion and accessible
navigation. The settings follow-up extends existing user preference persistence. Review sidebars
and mobile navigation redesign remain excluded.

## Technical approach

Follow the [system design](../../specs/ui/system-design/sidebar-hover-reveal.md).
Keep persisted layout width separate from transient visual width. Add a small local
hook; compose it into `AppSidebar` and separate header presentation from toggle state.
No new architecture boundary or ADR is required for this local presentation state.

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

## Tests

New `hooks/domains/sidebar/use-sidebar-hover-reveal.test.ts` covers “reveals at 500 ms”,
“cancels early exit”, “requires fresh entry”, “cleans up pending callbacks”, and
“cancels on eligibility loss” (AC .1, .3-.5). Extend `app-sidebar.test.tsx` for
“preserves layout reservation and saved collapse”, “keeps owned portals usable”,
“retains focused reveal”, and “explicit expansion persists” (AC .2-.4).
Extend `app-sidebar-header.test.tsx` for the visible pin action (AC .4).

## E2E tests

Add `e2e/tests/layout/sidebar-hover-reveal.spec.ts` in chromium: short hover cancellation,
full reveal, task navigation, owned workspace/task menus, Escape, pinning, no layout
shift, breakpoint and touch eligibility (AC .1-.5). Use controlled browser clock for
the timer boundary; use causal assertions for navigation and settled geometry.
Add `e2e/tests/layout/mobile-sidebar-hover-reveal.spec.ts` in mobile-chrome: no hover
rail, existing task drawer opens by tap and navigates, contained scrolling and no
horizontal overflow (AC .5). Reuse `mobile-sidebar-read-recovery.spec.ts` fixture patterns.

## Work orders

- [x] [Task 01: Implement sidebar hover reveal](task-01-hover-reveal.md)
- [x] [Task 02: Configure sidebar hover activation](task-02-hover-settings.md)

## Verification

Run sequentially from repository root; build before browser verification.

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

## Verification results: original interaction

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

## Risks

Portaled menus can appear outside DOM containment and must retain the reveal.
Visual expansion must not overwrite persisted collapse or move Dockview/status geometry.

## Documentation impact

Public how-to guidance updated in `docs/public/tasks-and-workflows.md` to explain
temporary hover reveal, explicit expansion, and phone tap navigation.

## Settings follow-up

Task 02 follows the completed Task 01 as one vertical TDD slice: persistence and
wire mapping, appearance draft/save integration, then runtime cancellation and
rendered desktop/phone evidence. The original verification results above remain
historical evidence; the settings work is verified separately below. Exact new commands and likely
files are in [Task 02](task-02-hover-settings.md).

### ASCII UI preview

UI-03: Preferences > Appearance > Sidebar, enabled (AC-UI-SIDEBAR-HOVER-002.1-.5).

```text
Desktop                           Phone
Sidebar                           Sidebar
Show sidebar on hover    [on]     Show sidebar on hover  [on]
Hover delay (ms)         [500]     Hover delay (ms)
Mouse or trackpad only.            [500                  ]
                                  Mouse or trackpad only.
[Discard] [Save changes]           [Discard] [Save changes]
```

UI-04: Disabled/invalid states at the same entry point (.2-.3).

```text
Show sidebar on hover   [off]      Hover delay (ms) [-1]
Hover delay (ms) [500 disabled]    Enter a whole number from 0 to 5000.
                                  [Save changes disabled]
```

Grouping, units, explicit save and retained disabled value are structural; spacing
is illustrative. The page owns vertical scrolling; the existing floating save
surface remains reachable above phone safe-area insets. Touch controls are at least
44 px, desktop numeric input 28 px. All copy uses locale keys. Screenshots and
geometry assertions in Task 02 compare the rendered controls to these previews.

### Tests and E2E coverage

Task 02 maps .1-.3 to backend `sidebar_hover_settings_test.go` files in user
store/service/handlers, mapper and appearance draft tests; .4 to fake-timer hook
and AppSidebar tests; .1-.4 to chromium `sidebar-hover-settings.spec.ts`; .5 to
mobile-chrome `mobile-sidebar-hover-settings.spec.ts`. Browser tests cover save,
reload, custom delay and disabled hover; mobile tests cover both controls and
continued tap navigation. Settings discovery is checked through its existing tests.

### Risks and documentation

False and zero must not be mistaken for omission. Numeric editing must not save
an empty field as zero. Remote saves must cancel timers without overwriting unsaved
settings drafts or losing focus. Update `docs/public/tasks-and-workflows.md` after
implementation to identify the appearance controls and defaults.

### Settings verification results

Completed on 2026-09-14. Task 02 records the exact verification commands and results.
223 tests across 17 frontend suites, backend user/settings-catalog suites, five
desktop browser tests and two phone browser tests passed. Typecheck, targeted lint
(no warnings), i18n, builds, spec and public-doc validation and whitespace checks
passed. Desktop and phone settings screenshots were inspected. The isolated test
instance at `100.105.155.17:48439` was refreshed with its data retained; the main
`:9998` process was untouched.
