---
created: 2026-11-03
status: complete
requirements:
  - REQ-UI-CLARIFICATION-CONTEXT-001
system_design:
  - ../../specs/ui/system-design/clarification-context.md
legacy_specs: []
---

# Implementation plan: Single clarification size control

## Overview

Replace the separate Expand and Reset buttons with one icon-only Expand/Restore toggle.
Restore returns to the previous content-sizing mode or manual height.
One work order delivers the state transitions, shared header, translations, public instructions, and focused tests together.

The user accepted this interaction and its ASCII preview in the primary conversation.
Task 01 delivered the interaction as one vertical slice.

## Ownership and reconciliation

UI owns the existing reusable panel-sizing contract across independent clarification hosts.
Update [the requirements](../../specs/ui/requirements/clarification-context.md) and
[the design](../../specs/ui/system-design/clarification-context.md), rather than create a parallel task-owned specification.
The amendment changes .10/.11 and adds .13-.16 under `REQ-UI-CLARIFICATION-CONTEXT-001`.
Both documents became active/current when this work order passed.

The completed [context usability package](../clarification-context-usability/plan.md) records delivery of the current two-button UI.
Its Task 02 Expand/Reset scenarios become Expand/Restore scenarios prospectively in this package.
Keep its completed status and historical test results unchanged.
The linked [context rendering](../clarification-context-rendering/plan.md),
[paragraph transport](../clarification-context-newlines/plan.md), and
[Threads scrolling](../threads-grid-clarification-overflow/plan.md) packages retain their delivered outcomes.
The [answer draft package](../clarification-answer-drafts/plan.md) retains application-owned answers and task-local sizing.
No companion work order needs reopening. Their current test files provide regression coverage here.

The [draft ownership ADR](../../decisions/2026-11-03-clarification-answer-draft-ownership.md)
and [current-turn ownership ADR](../../decisions/2026-08-14-current-turn-clarification-ownership.md) remain authoritative.
This local presentation change requires no new ADR, persistence tier, or backend contract.

## Scope

### In scope

- One icon-only Expand/Restore control across Task chat, Quick Chat, Needs You Inbox, Coordinator Needs You, and Threads.
- Separate desktop and phone mode/Restore state in the existing bounded bundle cache.
- Auto/manual restoration, host changes, keyboard and touch activation, mouse dragging, and grip double-click reset.
- Localized tooltips and accessible names, migrated test selectors, and focused rendered evidence.
- Update the existing clarification sizing instructions during implementation.

### Out of scope

- Changes to skip, collapse, question navigation, Submit, answer delivery, or the user-input barrier.
- A size menu, additional settings, cross-task size retention, reload persistence, or new touch-drag support.
- New host layout algorithms, screenshot recapture, unrelated header cleanup, or backend changes.

## Technical approach

### State and geometry

Extend `useClarificationResizeState` in `clarification-draft-context.tsx` and the fallback state in
`use-resizable-clarification-overlay.ts` with explicit auto/manual/expanded records for desktop and phone.
Expanded records save the normal state atomically, including an auto target with no pixel height.
Retain the provider's eight-bundle cache, task lifetime, bundle keys, and existing eviction boundary.

Use measured geometry for availability and effective height. Use explicit state for the next action.
Manual-at-maximum remains normal mode with disabled Expand, rather than implying a Restore target.
Restore remains enabled after host contraction. Expanded mode follows host growth as well as shrinkage.
The first real drag movement enters manual mode. Grip double-click clears the active presentation to auto mode.
Keep existing host observers, sibling reservations, initial caps, and Threads scroll fallback.

### Header and localization

Replace the paired size callbacks and flags through `ClarificationPanelSection`, `ClarificationInputOverlay`,
and `ClarificationOverlayTopBar` with one toggle action, explicit mode, and availability.
In `ClarificationHeaderActions`, use `IconArrowsMaximize` and `IconArrowsMinimize` in the same native Button.
The existing dockview group actions demonstrate this icon pair.
Use the existing Tooltip primitive, action-specific `aria-label`, and `aria-controls`.
Keep focus on the same button after either action.

Use `clarification-panel-size-toggle` as the stable test ID in both states.
Migrate all old expand/reset selectors in component and browser tests, including the Coordinator and Threads variants.
Use the English labels `Expand` and `Restore` through `t()`.
Update `chat.json` for en, pt-pt, zh-cn, ja, ko, zh-tw, zh-hk, and pseudo.
Generate Traditional Chinese and pseudo catalogs with the existing commands.
Remove old size-action keys only after a repository-wide caller search.

### Phone composition

Entry remains the pending-question panel inside the active conversation.
The nearest shipped exemplar is `ClarificationPanelSection` with `mobile-clarification.spec.ts`.
Keep its inline question, shared context, one active scroller, multi-question stepper, and answer actions.
This temporary decision needs no added drawer or route.
The size control stays secondary to answering the question.

Fine-pointer desktop uses a 28px square button. Phone widths below 768px and coarse pointers use at least 44px square targets.
Use phone layout independently from pointer precision. Preserve existing dynamic viewport and safe-area handling.
Separate presentation records preserve a desktop draft and Restore target through desktop-phone-desktop transitions.

### Public documentation

During implementation, update `docs/public/sessions-and-review.md`, under **Answer clarification questions**.
Replace its Expand/Reset instructions with the single Expand/Restore action and previous-height behavior.
This is a how-to section. Do not publish the planned behavior during this design-only turn.

## ASCII UI preview

### UI-01: Desktop pending question, normal and expanded

Entry: any shared pending-question host. State: one active question, normal auto/manual sizing.

```text
Current, verified from ClarificationHeaderActions:
[Question] [Expand to available space] [Reset panel size] [x] [v]

Proposed normal header:
[Question]                              [E] [x] [v]
  context, question, choices, and answer input

Proposed expanded header, same button position:
[Question]                              [R] [x] [v]
  context, question, choices, and answer input
  more available vertical space
```

`[E]` and `[R]` represent maximize/minimize icons. Their tooltips and accessible names are Expand and Restore.
They are one button, with no persistent text label. `[x]` remains Skip and `[v]` remains Collapse.
The existing conditional Submit action retains its position between the size button and Skip.
The existing stepper and progress remain in the leading header group for multiple questions.

### UI-02: Phone pending question, same inline surface

Entry: conversation's pending-question panel. State: normal or expanded.

```text
One question:
[Question]          [E/R] [x] [v]
[context and question text     ]
[choice A                      ]
[choice B                      ]
[answer input                  ]

Multiple questions, existing header wrap:
[Question] [< 2/3 >] [answered]
[E/R]          [Submit] [x] [v]
[context and active question   ]
[choices and answer input      ]
```

The same size control toggles between E and R. Touch targets measure at least 44px in both dimensions.
There is no second size button or menu. Existing narrow-header wrapping remains available.
Header controls stay in the existing clarification scroll region. This change does not make the header sticky.
The normal panel uses its existing bounded scroller. In exhausted Threads hosts, the footer remains the single active vertical scroller.
Safe-area and software-keyboard containment remain host responsibilities.

### UI-03: Constrained host

```text
Normal mode, no measured headroom:
[Question]               [E disabled] [x] [v]

Expanded mode, host shrinks:
[Question]                       [R] [x] [v]
```

Restore remains enabled even when the host has no spare space.
One size control, stable placement, no persistent labels, and action-specific names are structural requirements.
Spacing, the word Question, and ASCII icon letters are illustrative and add no product copy.
UI-01/UI-02/UI-03 map to .9-.11 and .13-.16 and the focused component and browser scenarios below.

## Tests

The following case names describe additions or updates within existing suites:

- `hooks/use-resizable-clarification-overlay.test.ts`: auto/manual Expand-Restore cycles (.14), repeated cycles and manual-at-maximum (.15).
- The same hook suite: host shrink/grow, unavailable measurement, enabled Restore (.9/.15), drag and double-click transitions (.16).
- `components/task/chat/clarification-draft-context.test.tsx`: expanded-auto remount retention, presentation separation, bundle/task isolation, and eight-bundle eviction (.10/.14).
- `components/task/chat/clarification-panel-section.test.tsx`: one icon-only toggle, accessible names, focus continuity, disabled Expand, preserved draft, collapse/reopen, and no response calls (.10/.11/.13-.16).
- `components/task/chat/clarification-input-overlay.test.tsx`: shared-header propagation and existing answer/submission regression coverage (.11/.13/.14).

Use TDD. Start with behavioral failures for two controls and missing manual-height restoration.
A missing new test ID alone does not establish behavioral RED.

## E2E tests

All paths below are under `apps/web/e2e/tests/`. Use managed runners sequentially with fresh builds and one shard.

- Chromium `chat/clarification-resize.spec.ts`: pointer and keyboard cycles from auto/manual heights, focus/tooltip behavior, no-headroom state, grip reset, and host shrink/grow (.9/.11/.13-.16).
- Chromium `chat/clarification-context-hosts.spec.ts`: Quick Chat and Needs You expansion/restoration, 767px/768px fine-pointer sizing, and coarse-pointer tablet target sizing (.9/.11/.13-.15).
- Chromium `coordinator/needs-you.spec.ts` and `task/threads-composer-disclosure.spec.ts`: migrate both old selectors and prove Restore in the existing bounded hosts (.9/.13/.14).
- Mobile Chrome `chat/mobile-clarification.spec.ts`: tap the same button twice, measure both target dimensions, preserve a draft and separate presentation records through remounts (.10/.11/.13-.15).
- Mobile Chrome `task/mobile-threads-composer-disclosure.spec.ts`: migrate old controls and retain short-host answer reachability with one active scroller (.9/.11/.13-.15).

Assert one size control before interaction. Capture normal/expanded desktop and phone screenshots and compare them with UI-01/UI-02.
Assert no document horizontal overflow, control containment, and reachable answer actions in constrained hosts.
Keep synthetic context and existing mock-agent fixtures. Do not change response semantics to simplify test setup.
Physical keyboard behavior remains a device-check limitation beyond viewport emulation.

## Work orders

- [x] [Task 01: Replace size actions with one toggle](task-01-single-size-toggle.md)

Execute Task 01 as one vertical slice. No implementation subagents are authorized.
Its exact verification commands cover state, presentation, all affected browser selectors, localization, and public instructions.

## Verification results

Design-package validation passed on 2026-11-03:

- `python3 scripts/list-docs.py validate`: 428 decisions and 1895 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specifications passed.
- Catalog discovery includes the updated UI requirement/design pair.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: the actual four-file documentation diff is exempt, with no errors.
- The same preflight with five planned runtime paths reports covered and selects Task 01, with no reference errors.
- `git diff --check -- docs/specs docs/plans/clarification-size-toggle`: passed.
- `git status --short`: two modified specifications and the new package directory. All files remain unstaged and uncommitted.

Implementation verification passed on 2026-11-03:

- Focused Vitest: 4 files and 105 tests passed across the resize hook, provider cache, panel section, and input overlay.
- `pnpm run typecheck` and `pnpm run build` passed.
- Changed-file ESLint with `--max-warnings 0` passed for the four source files, four unit files, header, and six browser specs.
- `pnpm run i18n:check` passed with all six translated catalogs and pseudo complete; `pnpm run i18n:ratchet` reported 0 added and 5 modified files clean.
- Managed Chromium: 27 tests passed in `clarification-resize`, `clarification-context-hosts`, `coordinator/needs-you`, and `task/threads-composer-disclosure`.
- Managed Mobile Chrome: 24 tests passed in `chat/mobile-clarification` and `task/mobile-threads-composer-disclosure`.
- The browser runs covered auto/manual Restore, repeated cycles, keyboard focus continuity through both Expand and Restore, tooltips, no-headroom disabled Expand, grip reset, host contraction/growth, 767px/768px and coarse-pointer sizing, and desktop/phone record separation with a preserved draft.
- Review remediation kept one stable native button using `aria-disabled` with guarded activation, so focus survives Restore while panel measurements catch up or the contracted host has no headroom. Expand accepts only fresh panel and host measurements at click time. Drag movement continues to update the manual height after the pointer returns to its starting Y.
- Physical-device software keyboard behavior is not available in this environment. It is recorded as a platform-level hardware check, not a closure gate for this presentation-only control; the host owns keyboard containment, matching the completed host-bounded expansion work.
- Public instructions were updated in `docs/public/sessions-and-review.md`.

## Risks

- Inferring mode from rendered height creates a false Restore state after manual dragging or host contraction.
- Caching only pixel heights loses an expanded panel's auto Restore target during responsive remounts.
- A partial selector migration leaves Coordinator or mobile Threads coverage on removed controls.
- Width-only responsive checks can miss a clipped 44px touch target. Measure both dimensions and ancestor containment.
- Public instructions must change with implementation, rather than describe unshipped controls during planning.
