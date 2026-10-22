---
created: 2026-10-22
status: done
requirements:
  - REQ-UI-MOBILE-TASK-NAVIGATION-001
system_design:
  - ../../specs/ui/system-design/mobile-menu-backdrops.md
legacy_specs: []
---

# Implementation Plan: Session menu opening gesture

## Overview

Prevent a native Sessions row's opening touch from selecting an action. One
sequential work order owns causal browser RED, consumer correction, and focused
validation. The user explicitly authorized bounded planning plus TDD without
a separate planning-approval checkpoint; stop ready for parent validation before
push or PR. No additional agents or persistent tasks.

## Scope

Own the local trigger interaction, faithful existing consumer tests, a dedicated
native-touch browser regression, and the smallest owning UI clarification.
Exclude backend/API, lifecycle policy, permissions, confirmations redesign,
shared Radix/Drawer primitives, Unicode rename, and arbitrary touch delays.

## Technical approach

Radix dropdown 2.1.16 opens on pointerdown. Preserved managed-phone evidence
shows pointerdown/up on the trigger followed by a click on newly rendered
Restart, sending session.stop and session.launch. Wait for the completed touch
click before opening, locally; preserve Radix mouse/pen/keyboard paths and the
existing action callbacks. PR4444 changes nested modal defaults independently;
this repair must not depend on either default. PR1512 changes selectors and
PR4013 adds bulk menu entries, both outside this repair.

## ASCII UI preview

UI-01: Native Sessions picker, selected secondary row, action menu.

```text
Phone: Sessions drawer           Touch completes -> inset menu
[Primary session          ...]   [Set as primary             ]
[Selected session         ...]   [Stop                       ]
                                [Restart session            ]
                                [Delete                     ]
                                [Handoff                  > ]
Tablet/desktop: existing session tabs and anchored menus (unchanged)
```

Before: opening touch can also select Restart. After: opening leaves the menu
inspectable; a separate item gesture chooses the action. Order above is
illustrative; eligibility and localized labels remain authoritative. Reuse
MobileSessionsPicker/MobilePickerSheet and shared menu-bottom-sheet styling:
one menu scroller, existing safe area and 70dvh cap, 44px phone/coarse-pointer
hit targets. Drawer selection remains the primary action. No layout changes.
This native picker is phone-only in task-layout; tablet and
desktop retain their existing session-tab composition. Real-Radix tests cover
mouse/pen at a wider viewport, and the phone browser covers hybrid mouse input.
Maps to AC-UI-MOBILE-TASK-NAVIGATION-001.3, .11, .12.

## Tests

Extend `mobile-sessions-section.test.tsx` through real Radix trigger/content to
prove touch opening phase separation, cancellation, explicit action once,
mouse/pen and keyboard behavior, and existing pending/delete handoff coverage.
Mocks may replace domain hooks, never menu coordination.

## E2E tests

`mobile-session-menu-gesture.spec.ts` in mobile-chrome uses native `.tap()`,
pre-navigation gateway capture with full payload/phase history and a stimulus
baseline. Only exact visible-session automatic focus launches from
`use-session-resumption.ts` are classified separately; explicit stop and
unclassified launches remain failures. Require zero menu mutation assertions
after opening and
dismissal, and a separate successful action with backend readback and exact
request target/count. Cover selected-row selection, repeated opening,
Escape/outside dismissal, phone hit targets/containment and hybrid
compatibility. Existing restart/deletion specs protect lifecycle integration.

## Work orders

- [x] [Task 01: Own the opening touch](task-01-own-opening-touch.md)

## Verification strategy

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/mobile/mobile-sessions-section.test.tsx)
(cd apps/web && GOMAXPROCS=2 GOFLAGS=-p=2 pnpm e2e:run --host --shards 1 --project mobile-chrome e2e/tests/session/mobile-session-menu-gesture.spec.ts e2e/tests/session/mobile-session-tab-restart.spec.ts e2e/tests/session/mobile-session-deletion.spec.ts --retries=0)
(cd apps/web && pnpm exec eslint components/task/mobile/mobile-sessions-section.tsx components/task/mobile/mobile-sessions-section.test.tsx components/task/mobile/use-session-menu-opening-gesture.ts e2e/tests/session/mobile-session-menu-gesture.spec.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Also run PR-documentation coverage preflight and assess public documentation.
Keep raw receipts outside the checkout and report every process terminal.

## Verification results

- Frozen install passed; no lockfile change. Real-Radix consumer RED: 4 failures,
  20 passes. Review regression RED: 3 failures/24 passes for unrelated keyboard
  propagation. Final consumer GREEN: 27/27 (3.80s).
- Native `.tap()` RED on original consumer, independently reconfirmed with full
  payload/phase assertions: trigger alone sent stop plus non-focus resume launch
  for the exact selected secondary. Final fresh managed phone package GREEN: 5/5 (53.1s),
  one worker/shard, retries 0, strict gateway accounting and bounded Go builds.
- Full gateway history retained with stimulus index: one legitimate primary
  `session_focus` before the stimulus; only the later explicit secondary
  `session.set_primary` mutation afterward. Backend readback/reload confirmed it.
  Opening, Escape, Enter, hybrid mouse, repeated touch and outside dismissal pass.
- Screenshot inspected against UI-01: inset menu, clear rows, dimmed backdrop,
  viewport containment, 44px targets and no document horizontal overflow.
- The combined package includes the native gesture test plus all four existing
  restart/deletion cases; retries 0 throughout. Scoped ESLint: zero
  errors/warnings. Typecheck/catalog check: passed with 8GB heap; initial default
  4GB attempt exhausted its heap (exit 134), preserved separately.
- Docs catalog validated 396 decisions/1697 specifications; full spec lint,
  documentation coverage (`covered`), and diff checks passed.
- Public docs assessment: `docs/public/sessions-and-review.md` already describes
  opening Sessions then choosing an action; no changed procedure/copy/API needs
  a public docs edit. Internal owning requirement/design clarified.
- Setup failures are preserved separately: startup focus baseline, lookup through
  a Radix-hidden dialog, and English Primary capitalization. No causal assertion
  weakened, lifecycle mocked, errors muted, arbitrary touch delay or retry added.
- Coverage is managed Chromium phone emulation, not physical-device/OS proof.
  Tablet/desktop use their existing separate tabs; wider mouse/pen are real-Radix
  consumer tests. PR4444 compatibility is source-reviewed, not a combined run.
- Remote main remains dff7433de5b7f5c42de5cd3e2bc0f61cd187f25e. Overlapping heads
  PR4444 f15b457, PR1512 a8655ad, PR4013 37bfa1ec remain unchanged; unrelated hunks
  and shared primitives are untouched. Detailed logs/artifacts are outside the
  checkout at the task workspace's `loop-receipts/attempt16/`.
- Review remediation limits trigger key propagation suppression to Enter, Space,
  and ArrowDown. Tab, ArrowRight, and unrelated keys reach surrounding handlers;
  existing opening-key tests still protect against selecting the enclosing row.
  The owning design is clarified; no new product/domain contract was introduced.
- Parent actual-diff and independent consumer validation completed, followed by
  separate same-session delivery authorization. Local checks above are complete;
  current-head CI/review, screenshot and merge evidence remain tracked externally
  until delivery finishes. No record-only commit is required for that state.

## Risks

- A simulated DOM event sequence cannot prove browser touch retargeting; actual phone
  regression is the causal gate.
- Keyboard events must not bubble into the selectable enclosing row.
- Preserve pending/deletion focus transfer and future nested non-modal behavior.
