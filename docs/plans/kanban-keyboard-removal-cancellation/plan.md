---
created: 2026-10-19
status: complete
requirements:
  - REQ-TASKS-KANBAN-TASK-REORDERING-001
system_design:
  - ../../specs/tasks/system-design/kanban-task-reordering.md
legacy_specs: []
---

# Implementation Plan: Cancel unavailable Kanban keyboard pickups

## Overview

Release a keyboard reorder gesture when its picked card disappears from the
board input, so another visible card can reorder without remounting the board.
One sequential work order adds independently authored regressions, makes the
hook correction, and verifies desktop recovery and existing phone behavior.
The implementation and focused verification are complete; results are recorded below.

## Evidence and assumptions

The supplied qualification on `7bc6934829994e6282f7a2b2c77246a610dd93c0`
rendered the real board, card and hook with virtualizer/render plumbing and the
reorder transport boundary replaced. Picking A with Space, removing A while
the board stayed mounted, then sending Escape/Space/Down/Enter to B yielded
zero reorder calls. Normal reorder and explicit Escape on still-visible A
followed by B passed. This is supplied rendered-path evidence, not an
independently reproduced browser or persisted-backend result. The earlier
wrong-column-prop fixture was setup failure, not product RED. Future tests
must be authored from production and existing public tests.

Source inspection confirms the cause: `useKeyboardReorder` stores `pickedUp`
but never reconciles it with later `tasks`; `handleKeyDown` rejects every other
card before reaching Escape. `SwimlaneKanbanContent` keeps the hook mounted as
`displayTasks` changes. User intent settles cancellation, no request for A,
fresh reorder for B, and phone navigation preservation. No material question
blocks the package.

This is a conformance repair for AC-TASKS-KANBAN-TASK-REORDERING-001.12 using
AC-TASKS-KANBAN-TASK-REORDERING-001.9 cancellation semantics. The owner amendment
clarifies the previously implicit availability edge under those existing IDs;
it introduces no separate repair requirement or new backend contract.

## Scope

### In scope

- Cancel when no task in the current hook input matches the picked ID; clear
  draft fields so the composition removes the pickup/insertion state.
- Let a remaining eligible card start and commit a fresh keyboard reorder.
- Preserve a gesture across equivalent list/object refreshes, unrelated task
  updates and virtual-row unmounting when the picked ID remains present.
- Preserve normal commit/Escape, descendant guards, pending-band and singleton
  pickup constraints, pointer behavior and phone card navigation.

### Out of scope

Backend/WS/store changes, reorder transport/merge/conflict changes, general
stale-response or save-ack handling, whole-band draft normalization, redefining
step/band transitions of a still-present picked task, workflow-switch handling,
new keyboard controls, automatic focus recovery, layout/touch redesign and copy.

## Technical approach

Change only `apps/web/hooks/domains/kanban/use-keyboard-reorder.ts` in production.
Reconcile stored pickup availability against the current board input by ID.
Retire the stored gesture rather than only masking its returned fields, and
never submit the retired draft. Do not key cancellation on task-array identity,
whole-band equality, DOM mount membership or global persisted membership.
`displayTasks` is after board filtering/orphan remapping and before virtual rows.
A change affecting other tasks alone does not invalidate the picked ID. An empty
input cancels; later reappearance must require a new pickup.

No new reusable ownership boundary or architectural alternative requires an
ADR. The existing owner pair and completed
[original reorder package](../kanban-task-reordering/plan.md) remain authoritative;
its historical results and the linked
[phone removal package](../remove-mobile-kanban-drag/plan.md) are not rewritten.

## ASCII UI preview

### UI-01: Desktop/tablet board, picked card disappears

Entry: focus A in a column, Space; the board then stops including A.

```text
BEFORE (defect)             AFTER (proposed recovery)
+-------------------+      +-------------------+
| Step              |      | Step              |
| A removed         |      | A removed         |
| B                 |      | B [focused]       |
| C                 |      | C                 |
+-------------------+      +-------------------+
A's draft remains          A's draft cancelled, no request
B keys ignored             B: Space -> Down -> Enter
                           Commit order: C, B
```

Required: retain the existing column/card hierarchy, remove obsolete pickup and
insertion state, and admit B's fresh gesture. B is focused explicitly by the
user/test; no automatic focus transfer is proposed. Existing live position
announcements apply to the fresh gesture. Spacing and titles are illustrative.
Maps to AC001.9/.12 and the rendered regression plus desktop browser scenario.

### UI-02: Phone, existing navigation composition

Entry: phone Kanban, current step selected.

```text
+-------------------+
| Board / step      |
| B                 |  Tap / Enter / Space -> task
| C                 |  Swipe cards -> column scroll
+-------------------+
No pickup, insertion marker or reorder request
```

Nearest shipped exemplar: `SwipeableColumns` and `KanbanCardShell` phone
navigation, consistent with the mobile UI language's Kanban precedent.
Retain the column's scroll ownership, horizontal step navigation, safe-area
handling and existing touch targets. There is no new overlay, action, hierarchy,
viewport geometry or responsive preference write. Shared hook availability
changes do not route phone keys into reorder. Existing real phone-card unit
coverage and `mobile-kanban-reorder.spec.ts` provide the phone assessment and
rendered control; no new mobile scenario or layout audit is needed.

## Tests

All paths below are relative to `apps/web`. Work-order commands own execution.

- `hooks/domains/kanban/use-keyboard-reorder.test.ts`: add
  `cancels a missing pickup and allows another card to reorder`; use A/B/C,
  rerender the same hook with B/C, assert all draft fields null and no request
  for A, then B Space/Down/Enter commits C/B. Also cover empty input followed by
  reappearance without draft revival. AC001.9/.12.
- Same file: `retains a pickup across equivalent and unrelated task updates`;
  preserve a moved draft while refreshing objects and changing/removing another
  task. Keep existing normal commit, Escape, descendant, pending and singleton
  controls. AC001.12/.27/.32.
- New `components/kanban/swimlane-kanban-content.keyboard-reorder.test.tsx`:
  `releases a removed real card and reorders a remaining real card` keeps the
  board mounted across tasks rerender. Keep real `SwimlaneKanbanContent`,
  `KanbanCard` and keyboard hook; replace only layout/virtualizer plumbing and
  `useStepReorder` transport boundary as necessary, using production prop names.
  Fire keys on actual card roots; assert no request for A, then one request for
  B with C/B order. Include normal reorder and explicit Escape controls.
  Assert obsolete draft/indicator props clear and B gains/loses `aria-grabbed`.
- In the same composition suite: `retains a pickup when only its virtual row
  unmounts` preserves tasks input while cycling the row's render presence,
  then resumes that card's gesture. AC001.12; protects the visibility boundary.
- Existing `components/kanban-card-regression.test.tsx`: phone Enter/Space opens
  once, repeated/descendant keys do not activate, and reorder handler stays
  uncalled; desktop retains keyboard wiring. Phone ACs remain owned by the
  linked mobile requirement, not a second requirement in this work order.

## E2E tests

Add one test to `e2e/tests/kanban/kanban-reorder.spec.ts`, `chromium`:
`keyboard reorder recovers after the picked card is removed`. Seed A/B/C in
one band; navigate once; pick A; delete A through existing `apiClient.deleteTask`
while remaining on the board; await A's disappearance; focus B; press
Escape/Space/ArrowDown/Enter. Arm a reorder request observer before removal,
assert no cancellation request using the sanctioned negative-assertion dwell,
then await B's single PUT, assert payload excludes A, UI order C/B and persisted
positions from the API. No reload is allowed between pickup/removal/recovery;
retain a board DOM identity sentinel to catch accidental remount. The existing
normal keyboard and pointer persistence cases run as GREEN controls. Author this
scenario before the hook fix: after hook/component RED, run only this scenario
against a fresh managed original-behavior build and require blocked recovery on
the same mounted board. If the browser baseline does not exhibit the defect,
checkpoint the discrepancy before production changes. Setup/remount failure is
not RED. AC001.9/.12.

Run existing `e2e/tests/kanban/mobile-kanban-reorder.spec.ts`, `mobile-chrome`,
unchanged. It covers swipe/hold scroll without move/reorder, navigation and
767/768 boundary affordances. Phone keyboard navigation remains covered by the
real-card unit control. Desktop RED builds the original behavior and desktop
GREEN rebuilds after the correction. Run mobile separately, using `--no-build`
immediately after desktop GREEN only when relevant source/base and backend/web/
plugin artifacts are unchanged; record that reuse explicitly. Do not duplicate
full builds without an intervening relevant change. Compare rendered structure with UI-01/UI-02 in these focused checks; no screenshot redesign.

## Work orders

- [x] [Task 01: Cancel unavailable pickup and prove recovery](task-01-cancel-unavailable-pickup.md)

## Public documentation impact

Internal docs updated: the owning requirement/design and this delivery package.
Search of `docs/public/**`, root README and screenshot catalog found no affected
Kanban keyboard reorder instructions. This repairs the existing interaction;
it adds no command, configuration, public API, label or screenshot composition.
No public-doc edit is needed. The original package's phone follow-up stays valid.

## Verification results

Design checks passed on 2026-10-19:

- `python3 scripts/list-docs.py validate`: 388 decisions, 1656 specifications.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
  The initial amendments exceeded the existing owner-file limits; concise
  wording keeps the final requirement at 20,268 bytes and design at 32,630 bytes.
- Repository `validateCoverage` preflight: `covered`, no errors, using the four
  documents plus the projected sole production hook path. Docs-only input was
  correctly `exempt`; the projection checks document references, not final
  implementation change coverage. The work order also defines a final preflight
  deriving actual statuses and all changed paths from the task base plus
  untracked additions, including hook, tests and browser scenario.
  The first attempt could not find `node` on PATH; the successful check used
  the existing Node 24.21.0 binary without installation.
- `git diff --check`: passed. Status contains only the four intended uncommitted
  artifacts (two modified owner files, two new package files).

Implementation completed 2026-10-19:

- Independent hook RED and real board/card RED exposed retained A state and
  blocked B pickup with normal/Escape/virtual-row controls passing.
- Fresh browser RED ran only the new scenario against original behavior. B
  remained blocked after API deletion, with the board mount marker retained.
- Four-file unit GREEN: 34 tests passed. After lint-required private hook/test
  extraction, only the two affected suites reran: 16 tests passed.
- Final changed-path ESLint and full web typecheck passed.
- Fresh desktop GREEN: 3 scenarios passed. Separate unchanged mobile controls:
  5 passed with `--no-build`, reusing the immediately preceding desktop GREEN
  backend/web/plugin artifacts. Exact artifact/source/base fingerprints matched
  before and after reuse, with no relevant intervening changes.
- UI-01 recovery and existing composition verified by real cards and browser;
  UI-02 phone navigation, scrolling and no-reorder behavior retained in existing
  real-card and mobile browser controls. Fresh desktop capture shows C/B order.
- Final catalog, specification lint, local links, diff and full actual-status
  delivery preflight passed (`covered`, all 8 changed paths, no errors).

The sole production file is the keyboard hook. The completed work order records
exact commands, causal failure details, setup corrections and final outcomes.
No public documentation change is required.

## Risks

- Clearing on every new array or virtual row disappearance would cancel valid
  gestures. The retention and row-cycle controls must prevent this.
- Returning null while retaining stored state could revive A after it reappears.
- A synthetic key-handler-only test can miss real-card guards; retain real card
  roots in composition coverage and use a fresh-build browser scenario.
- Deletion delivery must be observed on the mounted board before recovery keys;
  a navigation/reload would hide the defect. No broader lifecycle repair is part
  of this work order.
