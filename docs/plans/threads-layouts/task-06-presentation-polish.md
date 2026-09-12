---
id: "06-presentation-polish"
title: "Polish Threads presentation"
status: done
wave: 6
depends_on:
  - "05-document-presentation"
plan: "plan.md"
requirements:
  - REQ-UI-THREADS-DECK-005
  - REQ-UI-THREADS-SAVED-VIEWS-005
acceptance_criteria:
  - AC-UI-THREADS-DECK-005.1
  - AC-UI-THREADS-DECK-005.3
  - AC-UI-THREADS-DECK-005.6
  - AC-UI-THREADS-DECK-005.7
  - AC-UI-THREADS-DECK-005.11
  - AC-UI-THREADS-SAVED-VIEWS-005.4
system_design:
  - ../../specs/ui/system-design/threads-conversation-deck.md
  - ../../specs/ui/system-design/threads-saved-views.md
---

# Task 06: Polish Threads presentation

## Summary

Implement the user's feedback on the running playground: keep layout choices
only in View settings and smooth the entire routine composer's entry/exit.
This revises the existing approved package; initial work-order results remain
historical evidence, not evidence for this refinement.

## In scope

- Remove the standalone layout selector and its editor-opening callback.
- Use one mounted grid-track disclosure for hints, status controls, queue,
  editor, and tools. Preserve one CI family outside the animated region.
- Use interruptible 200ms reveal/160ms collapse, subtle fade/6px movement,
  immediate inert-state updates, and an immediate reduced-motion path.
- Update scoped engineering/public guidance and the existing browser scenarios.

## Out of scope

Backend, persistence, new dependencies, new interaction timers, plugin APIs,
touch composition changes, task lifecycle changes, publishing, and the main
instance on :9998. Refresh only the already-authorized isolated playground.

## Acceptance

1. No standalone layout control is rendered. The configurator retains keyboard
   selection, draft preview, save/discard, synchronization, and error recovery.
2. Both disclosure directions animate real height and content, preserving CI,
   editor identity, focus/holds, sibling bounds, and transcript follow/history.
   Reduced motion is instant; rapid reversals do not snap or leave inert inputs.
3. Phone/coarse-pointer composers stay visible with the existing single-chat
   navigation and view drawer. The normal full task composer is unchanged.

## ASCII UI preview

Revises [UI-01 through UI-05](plan.md#ascii-ui-preview). Control order and one
non-CI animation group are structural; spacing is illustrative.

```text
UI-01/02 desktop: [Saved view v] [View settings] [Listing views]
UI-03 collapsed: [Transcript                 ]
                 [CI]*
      expanded:  [Transcript                 ]
                 [CI]*
                 [Status / queue            ]
                 [Editor                    ]
                 [Tools / Send / Collapse   ]
                 <one short height/fade transition>
UI-04 settings:  Display: Layout [Grid v]  Auto-hide [On]
UI-05 phone:     [Threads / View v] [Position] [Menu]
                 [One conversation          ]
                 [Visible editor and tools  ]
```

CI is never faded or duplicated. Phone entry remains the shipped Threads view
drawer and title/session pickers. The existing transcript/footer scroll owners,
dynamic-height shell, safe areas, and touch targets are retained. No new mobile
surface is introduced. Tests below cover .005.11 and saved-view .005.4 directly,
with existing desktop/phone tests retaining the other cited criteria.

## Verification

Commands from repository root. Existing dependencies and backend fixtures are
already built in this worktree; no backend source changes belong to this order.

```bash
(cd apps/web && pnpm exec vitest run components/threads/threads-view-controls.test.tsx components/threads/threads-view-controls-recovery.test.tsx components/task/chat/composer-disclosure.test.tsx components/task/chat/use-composer-disclosure.test.ts components/task/chat/transcript-viewport-resize.test.ts components/task/chat/use-chat-input-container.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/threads/threads-view-controls.tsx components/threads/threads-view-display.tsx components/task/chat/composer-disclosure.tsx components/task/chat/chat-input-area.tsx components/task/chat/chat-status-bar.tsx components/task/chat/queued-ghost-list.tsx e2e/tests/task/threads-display-settings.spec.ts e2e/tests/task/threads-composer-disclosure.spec.ts)
(cd apps/web && pnpm run i18n:check)
make build-web-e2e
(cd apps/web && pnpm e2e:run --no-build --project chromium tests/task/threads-display-settings.spec.ts tests/task/threads-composer-disclosure.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-threads-display-settings.spec.ts tests/task/mobile-threads-composer-disclosure.spec.ts -- --retries=0)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/threads-layouts
```

Queue wrapper compatibility is additionally checked with:

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/queued-ghost-list.test.tsx components/task/chat/queued-ghost-pin.test.tsx)
(cd apps/web && pnpm exec vitest run components/task/chat/chat-input-area.test.ts components/task/chat/chat-input-area.test.tsx)
```

## Files likely touched

- `apps/web/components/threads/threads-view-{controls,display}.tsx` and controls tests.
- `apps/web/components/task/chat/composer-disclosure.{tsx,css}`, `chat-input-area.tsx`,
  `chat-status-bar.tsx`, and `queued-ghost-list.tsx`.
- Existing desktop display/disclosure E2E and mobile companion specs.
- Owning Threads requirements/designs, this manifest/order, scoped Threads
  `AGENTS.md`, and `docs/public/sessions-and-review.md`.

## Dependencies

Tasks 01 through 05 are implemented. No external branch dependency.

## Risks

Nested disclosures can double-animate; use one region. Immediate display:none
or padding changes would still snap. Keep CI outside the region, avoid cloning
its providers, and retain the native transcript resize owner.

## Parallelism

sequential

## Inputs

The user's two polish requests, paired requirement/design files, and the
shipped `AnchoredLastPromptBar` grid-track transition pattern.

## Results

PR #3626 follow-up: deep-link scrolling now honors reduced motion alongside
composer disclosure. Overflow assertions have descriptive labels, the mobile
Open task locator identifies a known tile, and Chinese hidden counts refer to
chats. Final commands and results are in the
[review remediation record](plan.md#pr-review-remediation-2026-09-12).
The original polish evidence below remains historical.

Completed 2026-09-12. Initial package evidence remains in Tasks 01 through 05.

RED: the controls regression expected no standalone selector and received the
existing button (two failing cases). The browser regression expected one real
height transition and observed none. Both failed before their production edits.

GREEN: layout selection is confined to the configurator. One CSS grid-track
region smoothly reveals/collapses the entire routine footer, including its
padding. Its content fades with 6px movement; CI is outside the animated/inert
region and retains its mounted instance across disclosure. No new animation
timers, dependencies, disclosure state, or competing scroll owner were added.

Every command in Verification passed:

| Check | Result |
| --- | --- |
| Six specified controls/disclosure/scroll/input unit files | 50 passed |
| Queue list/pin compatibility files | 58 passed |
| Input-area helper/component files | 29 passed; full-module mock updated for the CI export |
| `pnpm run typecheck` | Passed |
| Scoped `pnpm exec eslint` command | Passed; changed E2E files checked again after harness corrections |
| `pnpm run i18n:check` | Passed; existing orphan-catalog warnings remain nonblocking |
| `make build-web-e2e` | Passed with the pseudo QA catalog; normal production build also passed |
| Desktop display/disclosure browser command | 10 passed, retries 0, strict WS |
| Mobile display/disclosure browser command | 4 passed, retries 0, strict WS |
| Public-doc validator test and validator | 1 test and 46 pages passed |
| Spec-linter tests and complete spec lint | 36 tests and all files passed |
| `git diff --check`; scoped plan status inventory | Passed; all six orders present |

The final browser checks sample real intermediate footer heights in both
directions while CI remains visible/non-inert, plus history/bottom anchoring,
unchanged sibling bounds, required actions, native menus/cancellation, drafts,
the full task page, layout save/discard/sync/recovery, and reduced motion.
The initial reduced-motion assertion counted the tile's unrelated focus-ring
transition; it now targets the composer itself. A normal Vite production build
intentionally omits the pseudo catalog; the translated-surface checks use the
repository's `build-web-e2e` target. Neither harness correction required a
production workaround.

Inspected desktop grid/revealed draft and phone Display screenshots against
UI-01 through UI-05. Existing mobile coverage verifies the always-visible
composer, single-chat navigation, touch controls, short viewport, tablet,
native drawer, and translated narrow-screen containment. This is emulation,
not physical-device keyboard evidence.

Public docs updated: `docs/public/sessions-and-review.md` (how-to). Owning
requirements/designs and scoped Threads engineering guidance are synchronized.

The already-authorized :48431 playground received an asset-only refresh from
this verified build. Index SHA-256:
`7a13f79de5155e844bdc261ff6b8c20477c52ba52cae3b25296c75985d343ace`.
Tailscale HTTP served byte-identical JS/CSS. Its backend stayed PID 1174378;
main :9998 stayed PID 2754878. No data or saved settings were reset and no
backend was restarted. Old hashed chunks and the prior index are retained.
The existing shutdown command is unchanged. No commit, push, PR, delegation,
or new Kandev task/session was performed.
