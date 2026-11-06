---
id: "01-cancel-unavailable-pickup"
title: "Cancel unavailable pickup and prove recovery"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-KANBAN-TASK-REORDERING-001
acceptance_criteria:
  - AC-TASKS-KANBAN-TASK-REORDERING-001.9
  - AC-TASKS-KANBAN-TASK-REORDERING-001.12
  - AC-TASKS-KANBAN-TASK-REORDERING-001.27
  - AC-TASKS-KANBAN-TASK-REORDERING-001.32
system_design:
  - ../../specs/tasks/system-design/kanban-task-reordering.md
---

# Task 01: Cancel unavailable pickup and prove recovery

## Summary

Retire a keyboard gesture when its picked task disappears from the current
board input. Keep the same board mounted and prove a remaining card can reorder,
while valid gestures and phone task navigation retain their behavior.

## In scope

- The hook availability reconciliation in the owning design's
  [Keyboard gesture availability](../../specs/tasks/system-design/kanban-task-reordering.md#keyboard-gesture-availability).
- Independently authored hook and real board/card regressions, including mixed
  A-removed/B-and-C-present input and virtual-row-only unmount retention.
- One desktop deletion/recovery browser scenario, existing desktop pointer and
  keyboard controls, existing phone real-card and browser controls.
- Update the package's pending results/status only after actual verification.

## Out of scope

Backend, WS/store/protocol changes, general stale response/save acknowledgement,
redefining still-present task step/band transitions, whole-band normalization,
workflow switching, focus restoration, new copy, layout/touch redesign, generic
review/QA/simplify or broad suite replay.

## Acceptance

1. With A picked up, replacing the board input A/B/C with B/C clears all pickup
   and draft fields, emits no reorder for A, and allows B to pick up, move down
   and commit exactly C/B on that same mounted board. Empty input also retires
   the stored gesture; A reappearing does not revive it. AC001.9/.12.
2. Equivalent arrays/objects, unrelated task updates and a virtual-row-only
   unmount retain the existing draft when A remains in the board input. Normal
   commit/Escape, descendant guards and pending/singleton pickup controls pass.
   AC001.12/.27/.32.
3. Real desktop cards complete the browser removal/recovery flow with correct
   request payload, rendered order and persisted positions. Phone cards retain
   Enter/Space navigation, touch scrolling and no-reorder affordances in the
   existing controls; UI-01/UI-02 composition remains unchanged.

## ASCII UI preview

Excerpt from [full UI-01/UI-02 preview](plan.md#ascii-ui-preview).

```text
UI-01 Desktop/tablet (AC001.9/.12)
Before: A removed, B/C remain -> A draft blocks B
After:  A removed, B/C remain -> A draft cancelled, no request
        Focus B; Space -> Down -> Enter -> C/B persisted

UI-02 Phone (existing phone exception)
[ Board / step ]
[ B            ]  Tap / Enter / Space -> task
[ C            ]  Swipe -> column scroll; no pickup/reorder
```

The column/card structure, scroll ownership, safe-area handling and touch
geometry are retained. Focus B is an explicit interaction, not automatic focus
transfer. Titles/spacing are illustrative; recovery behavior and phone
navigation are requirements. No new localized strings are required.

## Implementation and test order

1. Read the owner amendment, current hook, existing tests and production card
   wiring. Mark this work order `in_progress` only after the later explicit
   implementation request. Use the existing render-stability composition test
   as provider/plumbing reference; do not read or replay private qualifications.
2. Add the named hook and real composition regressions from the plan. Run the
   RED block before production changes. A fixture/import/prop failure is not
   product RED: obtain the unavailable-pickup failure with normal reorder and
   explicit Escape controls passing. Retain every original command handle and
   actual exit evidence under the external operational plan.
3. Before changing the hook, author the new desktop browser scenario using
   the existing task API, causal waits, real cards and a same-board identity
   assertion. After hook/component RED, run ONLY that new scenario against a
   fresh managed baseline build with the browser RED command below. It must
   fail because B cannot recover after A disappears while the board stays
   mounted. Setup failure, selector failure, missing delivery or remount is
   not product RED. If the real browser baseline does not exhibit the defect,
   checkpoint the discrepancy before any production change.
4. After qualified hook/component and browser RED, make the smallest correction
   to the hook's stored gesture lifecycle. The input predicate is whether at
   least one task has the picked ID. A row's DOM presence is irrelevant.
   Retain existing event guards and transport path.
5. Execute targeted GREEN/static commands, then the complete affected desktop
   spec against a fresh managed build. Run mobile separately, reusing those
   exact GREEN artifacts with `--no-build` only if relevant source/base and
   backend/web/plugin artifacts are unchanged. Record artifact reuse explicitly;
   do not perform duplicate full builds without an intervening relevant change.
   Record results and preview assessment, then synchronize both package statuses.
   Do not claim completion for unrun commands or substitute broad passing suites.

## Verification

All blocks start from the repository root. Fresh-worktree bootstrap, once,
before the first pnpm command:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

RED before the hook correction; GREEN repeats this block with the phone-card
and pure keyboard controls added in the following block:

```bash
(cd apps/web && pnpm exec vitest run --maxWorkers=1 hooks/domains/kanban/use-keyboard-reorder.test.ts components/kanban/swimlane-kanban-content.keyboard-reorder.test.tsx)
```

Browser RED after hook/component RED and before production changes. This
fresh managed build tests only the new removal scenario on original behavior:

```bash
(cd apps/web && pnpm e2e:run --project chromium tests/kanban/kanban-reorder.spec.ts -- --grep 'keyboard reorder recovers after the picked card is removed$' --retries=0)
```

GREEN and static checks after the fix:

```bash
(cd apps/web && pnpm exec vitest run --maxWorkers=1 hooks/domains/kanban/use-keyboard-reorder.test.ts components/kanban/swimlane-kanban-content.keyboard-reorder.test.tsx components/kanban-card-regression.test.tsx lib/kanban/keyboard-reorder.test.ts)
(cd apps/web && pnpm exec eslint --max-warnings 0 hooks/domains/kanban/use-keyboard-reorder.ts hooks/domains/kanban/use-keyboard-reorder.test.ts components/kanban/swimlane-kanban-content.keyboard-reorder.test.tsx e2e/tests/kanban/kanban-reorder.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/kanban/kanban-reorder.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/kanban/mobile-kanban-reorder.spec.ts -- --retries=0)
```

Run desktop and mobile managed browser commands serially: the runner accepts
only one project. Browser RED discovers exactly one new scenario. Desktop GREEN
discovers the two existing controls plus that scenario; mobile discovers its five
existing controls. Desktop GREEN rebuilds after the hook fix. The mobile command
above deliberately reuses that immediately preceding build: record the unchanged
source/base and backend/web/plugin artifacts and the reuse in Results. If any
relevant input changed, omit `--no-build` for mobile and state why rebuilding was
required. Record actual discovery and exits; no-tests or fixture failure is not
behavioral evidence. No product check in these blocks is authorized in design.

Cheap design/reference/diff checks, valid during design and after implementation:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
node - <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/specs/tasks/requirements/kanban-task-reordering.md',
  'docs/specs/tasks/system-design/kanban-task-reordering.md',
  'docs/plans/kanban-keyboard-removal-cancellation/plan.md',
  'docs/plans/kanban-keyboard-removal-cancellation/task-01-cancel-unavailable-pickup.md',
];
const fileContents = Object.fromEntries(paths.map(path => [path, fs.readFileSync(path, 'utf8')]));
// Design-only projection; this is not final changed-file delivery coverage.
const result = validateCoverage({ changedFiles: [...paths, 'apps/web/hooks/domains/kanban/use-keyboard-reorder.ts'], fileContents });
console.log(JSON.stringify({ ok: result.ok, status: result.status, errors: result.errors }));
if (!result.ok) process.exitCode = 1;
NODE
git status --short -- docs/specs/tasks/requirements/kanban-task-reordering.md docs/specs/tasks/system-design/kanban-task-reordering.md docs/plans/kanban-keyboard-removal-cancellation
```

The preceding projection is a design reference check only. Final implementation
coverage must use the FULL actual working-tree change set relative to this
package's base, including committed, staged and unstaged edits plus untracked
additions. This separate final command derives statuses rather than labeling
all files modified or checking only the four documents:

```bash
node - <<'NODE'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const base = '7bc6934829994e6282f7a2b2c77246a610dd93c0';
const git = args => execFileSync('git', args, { encoding: 'utf8' });
const fields = git(['diff', '--name-status', '-z', '--no-renames', base, '--']).split('\0');
if (fields.pop() !== '') throw new Error('Unterminated Git diff output');
if (fields.length % 2) throw new Error('Invalid Git name/status pairs');
const changedByPath = new Map();
const statuses = { A: 'added', M: 'modified', D: 'removed', T: 'modified' };
for (let index = 0; index < fields.length; index += 2) {
  const [code, filename] = fields.slice(index, index + 2);
  if (!statuses[code] || !filename) throw new Error('Unsupported Git change entry');
  changedByPath.set(filename, { filename, status: statuses[code] });
}
const untracked = git(['ls-files', '--others', '--exclude-standard', '-z']).split('\0');
if (untracked.pop() !== '') throw new Error('Unterminated Git untracked output');
for (const filename of untracked) {
  if (!filename || changedByPath.has(filename)) throw new Error('Invalid untracked entry');
  changedByPath.set(filename, { filename, status: 'added' });
}
const changedFiles = [...changedByPath.values()];
const references = [
  'docs/specs/tasks/requirements/kanban-task-reordering.md',
  'docs/specs/tasks/system-design/kanban-task-reordering.md',
  'docs/plans/kanban-keyboard-removal-cancellation/plan.md',
  'docs/plans/kanban-keyboard-removal-cancellation/task-01-cancel-unavailable-pickup.md',
];
const fileContents = Object.fromEntries(references.map(path => [path, fs.readFileSync(path, 'utf8')]));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({ base, changedFiles, ok: result.ok, status: result.status, errors: result.errors }));
if (!result.ok) process.exitCode = 1;
NODE
```

Validate its emitted paths/statuses against `git status --short` before recording
final coverage; hook, hook tests, composition tests and browser scenario must be
included when changed. A design-only `exempt` result is not final implementation
coverage. Reconcile any unexpected foreign changes without reverting them.

Use the session's external operating plan for runtime selection, leases,
command-handle tracking, log locations and delivery controls. Those procedures
are not repository product changes.

## Files likely touched

Production:

- `apps/web/hooks/domains/kanban/use-keyboard-reorder.ts`

Tests:

- `apps/web/hooks/domains/kanban/use-keyboard-reorder.test.ts`
- `apps/web/components/kanban/swimlane-kanban-content.keyboard-reorder.test.tsx` (new)
- `apps/web/e2e/tests/kanban/kanban-reorder.spec.ts`

Four design-package files carry this work order's scope/status/results. Existing
`kanban-card-regression.test.tsx`, `keyboard-reorder.test.ts` and
`mobile-kanban-reorder.spec.ts` are unchanged controls. Board composition/card
production files are inspection and rendered integration boundaries, not
planned production edits.

## Dependencies

None. One sequential work order; no delegates or parallel work packets.

## Risks

Cancellation on array identity or DOM absence would disrupt valid gestures.
Masking returned state alone can revive cancelled drafts. A mocked card or
remounted board can hide the failure. Browser deletion must arrive before
recovery keys and negative-request assertions must use a live observer plus the
sanctioned dwell helper.

## Parallelism

`sequential`

## Inputs

- [Requirement, AC001.9/.12/.27/.32](../../specs/tasks/requirements/kanban-task-reordering.md)
- [System design](../../specs/tasks/system-design/kanban-task-reordering.md)
- [Plan test and browser matrix](plan.md#tests)
- `apps/web/components/kanban/swimlane-kanban-content.render-stability.test.tsx`
- `apps/web/components/kanban-card-regression.test.tsx`
- `apps/web/e2e/tests/kanban/kanban-reorder.spec.ts`
- `apps/web/e2e/tests/kanban/mobile-kanban-reorder.spec.ts`

## Results

Completed 2026-10-19. Production changes are limited to the keyboard hook's
private pickup-state lifecycle. It cancels on picked-ID absence and leaves
existing handlers and transport unchanged.

- Hook RED: missing-card and empty-input regressions retained A instead of
  cancelling; existing normal/Escape/retention controls passed.
- Real composition RED: B could not acquire `aria-grabbed` after A disappeared;
  three normal/Escape/virtual-row controls passed. An initial plural-indicator
  query error was corrected before qualifying this failure.
- Fresh managed browser RED: only the new scenario ran, no retries. B pickup
  remained blocked after A deletion; the board mount sentinel survived and no
  cancellation request occurred. No production edit preceded this result.
- Reviewed four-file unit GREEN: 34 tests passed. Scoped lint then required a
  private pickup hook extraction and test grouping/literal cleanup. The two
  affected suites passed again, 16 tests; unchanged phone-card and pure-helper
  controls were not replayed.
- Final changed-path ESLint: passed for the production hook and three changed
  test files, with zero warnings. Full web typecheck: passed.
- Fresh managed desktop GREEN: all 3 scenarios passed (pointer persistence,
  normal keyboard persistence, same-board deletion recovery and persisted C/B).
- Separate mobile GREEN: all 5 existing scenarios passed with `--no-build`.
  The immediately preceding desktop GREEN backend binaries, web distribution,
  plugin package/identity, source diff and base/HEAD fingerprints were identical
  before and after reuse. No relevant input changed or duplicate build ran.
- UI-01 assessed in the real composition and desktop browser: A disappears,
  B recovers, C/B persists without remount. A fresh gated desktop screenshot
  shows the existing column/card structure with C above B. UI-02 is protected
  by the real phone-card Enter/Space controls and mobile swipe/hold/navigation/
  breakpoint browser checks. Phone reorder has no rendered surface to capture.
- Catalog, all-spec lint, local links, actual-file delivery coverage and
  `git diff --check` passed. The actual-input preflight includes all 8 changed
  files and their derived added/modified statuses; it is `covered` with no
  errors, not the earlier design projection/exemption.

The first browser launch stopped before building because Corepack selected the
wrong pnpm version from the repository root. Running the reviewed command from
`apps/web` resolved setup; that startup failure was not behavioral RED. The
workspace dependency install ran once. No backend, store, pointer or phone
navigation source changed. Public documentation impact remains unchanged.
