---
id: "01-match-accepted-chords"
title: "Match accepted chords"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001
  - REQ-TASKS-SUBTASK-DIALOG-PARENT-DEFAULTS-001
acceptance_criteria:
  - AC-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001.5
  - AC-TASKS-SUBTASK-DIALOG-PARENT-DEFAULTS-001.1
  - AC-TASKS-SUBTASK-DIALOG-PARENT-DEFAULTS-001.2
  - AC-TASKS-SUBTASK-DIALOG-PARENT-DEFAULTS-001.3
  - AC-TASKS-SUBTASK-DIALOG-PARENT-DEFAULTS-001.4
  - AC-TASKS-SUBTASK-DIALOG-PARENT-DEFAULTS-001.5
system_design:
  - ../../specs/plugins/system-design/plugin-shortcut-settings.md
  - ../../specs/tasks/system-design/subtask-dialog-parent-defaults.md
---

# Task 01: Match Accepted Chords

## Summary

Make existing conflict warnings and plugin core precedence agree with
`matchesShortcut` accepted events. Own pure overlap detection, event-specific
core reservation, and the existing conflict-label consumer in one sequential
implementation pass with independently authored regression evidence.

## In scope

- Replace `comboKey` and its callers with direct accepted-event overlap for
  warnings and actual-event core matching for dispatch.
- Keep direct two-entry groups; accumulate stable deduplicated peer labels in
  `buildConflictLabels` and reuse `useShortcutConflictLabels` on both cards.
- Preserve effective override reads, unbound behavior, fixed reservation
  defaults, and all unrelated dispatcher admission and handler behavior.
- Add focused tests to existing suites, with AC traceability annotations.

## Out of scope

Apply the plan's exclusions. Do not read, copy, replay, delete, or derive tests
from ROOT's private qualification source or `candidate.test.ts`/`source.json`.
No layout, copy, recorder, persistence, inventory, manifest, SDK, backend,
platform-matching, CodeMirror, or independent E2E changes are authorized.

## Acceptance

1. A fresh regression mounting real `usePluginShortcuts` and real
   `useKeyboardShortcut(SHORTCUTS.FIND_IN_PANEL)` fails before correction for
   the causal reason: Mac stub, plugin `ctrl+f`, cancelable ordinary Ctrl+F,
   core callback 0 and plugin callback 1. After correction core callback is 1,
   plugin callback 0, and default is prevented by core. Canonical Meta+F and
   noncolliding plugin Ctrl+Q controls pass before and after. Restore all
   platform stubs and unregister test handlers on cleanup.
2. Conflict and actual-label consumer tests show Mod overlaps Ctrl-only,
   Meta-only, and Ctrl+Meta but never make disjoint Ctrl-only and Meta-only
   actions label each other. Accumulate every direct peer once across groups.
   Cover mixed `ctrlOrCmd` with explicit `ctrl`/`cmd`, exact modifiers,
   key-case, different keys, Alt/Shift, and unbound entries using explicit
   behavioral expectations rather than tests mirroring the helper algorithm.
3. A core Ctrl-only override reserves Ctrl+Q while a Mod plugin still receives
   Meta+Q. Fresh override/removal, unbound, and fixed nonconfigurable core
   cases remain correct; a core-less reservation test observes no plugin
   `preventDefault`. Existing affected suites pass with no changes to
   matching compatibility, editor rules, registration order, or save flows.

## Implementation order

After a later explicit implementation release and one granted local-heavy
lease, mark this order `in_progress`. Ensure dependencies exist before pnpm
checks; only when missing, run `pnpm install --frozen-lockfile` from `apps/`.
Author the regression independently in the existing suite, run it RED with
controls, retain the causal failure receipt, then implement the minimum fix
and run GREEN. Add overlap, label-consumer, and partial-overlap regressions
before their production corrections. Run all product checks sequentially.
Record actual native handles, completion joins, exits, and current absence of
owned processes; retain failure evidence. Do not overlap heavy commands or
discard a native handle. Keep all work in this primary session.

## Verification

Run the RED selection before production edits, then repeat it for GREEN from
`apps/web`:

```bash
pnpm exec vitest run hooks/use-plugin-shortcuts.test.ts -t 'accepted-event core precedence' --maxWorkers=1
```

Name the new real-hook describe block `accepted-event core precedence` so this
selection includes the causal case and its two controls. Other RED selections
use the explicit newly added overlap/consumer case names in their owned suites;
record the exact commands and expected failures in Results.

Run these affected checks sequentially from `apps/web` after GREEN:

```bash
pnpm exec vitest run hooks/use-plugin-shortcuts.test.ts hooks/use-keyboard-shortcut.test.ts hooks/use-app-shortcuts.test.ts hooks/use-integration-shortcuts.test.ts lib/keyboard/shortcut-conflicts.test.ts lib/keyboard/utils.test.ts lib/keyboard/plugin-shortcuts.test.ts lib/keyboard/shortcut-overrides.test.ts components/settings/keyboard-shortcuts-card.test.tsx components/settings/plugins/plugin-shortcuts-card.test.tsx --maxWorkers=1
pnpm exec eslint --max-warnings 0 hooks/use-plugin-shortcuts.ts hooks/use-plugin-shortcuts.test.ts lib/keyboard/shortcut-conflicts.ts lib/keyboard/shortcut-conflicts.test.ts components/settings/keyboard-shortcuts-card.tsx components/settings/keyboard-shortcuts-card.test.tsx components/settings/plugins/plugin-shortcuts-card.test.tsx
pnpm run typecheck
```

After implementation is released, stage only task-owned changes for normal
delivery. Run `pnpm run i18n:ratchet` from `apps/web` with its normal active
selectors, including added/owned files through the index; no selector narrowing
or hook bypass. No new product copy or locale keys are planned. If actual owned
files differ, use their exact paths for ESLint and record that bounded change.

From repository root:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git diff --cached --check
git status --short
```

Use this read-only working-tree coverage API preflight from repository root
with the activated Node toolchain. After implementation, replace the projected
source list only if actual owned paths changed; require `ok: true` and no errors.

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const artifacts = [
  'docs/specs/plugins/requirements/plugin-shortcut-settings.md',
  'docs/specs/plugins/system-design/plugin-shortcut-settings.md',
  'docs/plans/shortcut-conflict-dispatch/plan.md',
  'docs/plans/shortcut-conflict-dispatch/task-01-match-accepted-chords.md',
  'docs/specs/tasks/requirements/subtask-dialog-parent-defaults.md',
  'docs/specs/tasks/system-design/subtask-dialog-parent-defaults.md',
];
const source = [
  'apps/web/lib/keyboard/shortcut-conflicts.ts',
  'apps/web/hooks/use-plugin-shortcuts.ts',
  'apps/web/components/settings/keyboard-shortcuts-card.tsx',
  'apps/web/lib/keyboard/shortcut-conflicts.test.ts',
  'apps/web/hooks/use-plugin-shortcuts.test.ts',
  'apps/web/components/settings/keyboard-shortcuts-card.test.tsx',
  'apps/web/components/settings/plugins/plugin-shortcuts-card.test.tsx',
  'apps/web/components/task/new-subtask-dialog.tsx',
  'apps/web/components/task/use-subtask-parent-defaults.ts',
  'apps/web/components/task/new-subtask-dialog.test.tsx',
  'apps/web/components/task/new-subtask-dialog-context.test.ts',
];
const changedFiles = [...artifacts, ...source].map(filename => ({
  filename,
  status: filename.startsWith('docs/plans/shortcut-conflict-dispatch/') || filename.endsWith('/subtask-dialog-parent-defaults.md') || filename.endsWith('/new-subtask-dialog.test.tsx') ? 'added' : 'modified',
}));
const fileContents = Object.fromEntries(artifacts.map(p => [p, fs.readFileSync(p, 'utf8')]));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.errors.length) process.exitCode = 1;
NODE
```

Do not invoke hosted status publication for a local dry-run. Record the command
and result. No broad local suite, build, QA, audit, or new mobile E2E is required
unless an actual failure makes a focused additional check necessary.

## Files likely touched

- `apps/web/lib/keyboard/shortcut-conflicts.ts`
- `apps/web/hooks/use-plugin-shortcuts.ts`
- `apps/web/components/settings/keyboard-shortcuts-card.tsx`
- `apps/web/lib/keyboard/shortcut-conflicts.test.ts`
- `apps/web/hooks/use-plugin-shortcuts.test.ts`
- `apps/web/components/settings/keyboard-shortcuts-card.test.tsx`
- `apps/web/components/settings/plugins/plugin-shortcuts-card.test.tsx`
- The four design-package artifacts, for accurate delivery status/results.

`utils.ts`, core reservation IDs, overrides, manifests, SDK, backend, and
plugin-detail production markup are read-only inputs.

## Dependencies

None. Later explicit implementation request and local-heavy lease required.

## Mobile and rendered scope

Reuse the current settings cards and translated warnings. This repair changes
shared data/action logic only. Attached keyboards on phones use the same
dispatch path. Real warning-consumer component tests provide the narrow
mobile-parity evidence; no phone layout preview or geometry E2E is warranted.
See the [plan](plan.md#mobile-and-rendered-scope).

## Risks

Nontransitive overlap, overwritten/duplicate labels, whole-binding suppression,
and platform stubs leaking between tests. Physical OS shortcut delivery remains
outside simulated keydown evidence.

## Parallelism

`sequential`

## Inputs

- Existing AC-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001.5 and its owning design's
  [conflict behavior](../../specs/plugins/system-design/plugin-shortcut-settings.md#conflict-and-dispatch-behavior).
- Repository matcher, real hook and registry, current core reservations, and
  affected settings-card tests. The qualification receipts establish the
  defect, not a test-source template.
- Plan exclusions and docs-maintainer assessment. Existing public precedence
  wording and completed relocation results remain valid.

## Results

- Frozen installation from `apps/` was required because this worktree had no
  dependencies: `pnpm install --frozen-lockfile` passed with pnpm 9.15.9.
- Independent RED: the exact `accepted-event core precedence` selection above
  exited 1 with one causal failure (core 0/plugin 1/default prevented) and two
  passing controls. GREEN repeated that command and passed all three tests.
- Additional RED:
  `pnpm exec vitest run hooks/use-plugin-shortcuts.test.ts lib/keyboard/shortcut-conflicts.test.ts components/settings/keyboard-shortcuts-card.test.tsx components/settings/plugins/plugin-shortcuts-card.test.tsx -t 'accepted overlap|direct conflict labels|event-specific core reservations' --maxWorkers=1`
  exited 1 with 16 expected failures and nine passes. This directly exposed
  accepted-event, label accumulation, rendered warning, and reservation defects.
- The exact ten-file affected Vitest command above passed 178/178 tests.
- The exact seven-file ESLint command initially exited 1 for an unused `isMac`
  import, a six-parameter test callback, and a 104-line test describe callback.
  Remove the import, retain the tuple through a rest callback, and split the
  describe block without changing behavior or assertions. The repeated scoped
  ESLint command passed with zero warnings.
- After those mechanical lint corrections:
  `pnpm exec vitest run hooks/use-plugin-shortcuts.test.ts lib/keyboard/shortcut-conflicts.test.ts components/settings/keyboard-shortcuts-card.test.tsx --maxWorkers=1`
  passed 59/59 tests. The other seven previously passing suites were unchanged.
- `NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck` passed once before
  those mechanical lint corrections, including its normal generation and
  rich-output checks. No generated files changed.
- Normal `pnpm run i18n:ratchet` ran after staging all 11 owned files and passed
  (zero added and three modified production candidates; 643-entry allowlist).
  Test exclusions and all selectors remained unchanged.
- `python3 scripts/list-docs.py validate` passed (382 decisions, 1609 specs);
  `python3 scripts/lint-spec-files.py --all` passed; both working/index
  `git diff --check` commands passed. The then-current coverage API heredoc
  returned `ok: true`, `errors: []` for the actual 11 owned paths.
- Existing callback ordering, editor rules, fixed reservations, override
  behavior, matcher compatibility, save flows, and phone composition remain
  covered by the affected suites and unchanged production boundaries.
  Evidence uses simulated DOM keydown events; physical OS keys were not proven.
- All local native commands were joined and retained as external operational
  receipts with actual exits and current owned-group absence. Failed RED and
  lint receipts were kept. No private qualification source was read or replayed.


## Required-CI correction addendum

Original Results remain completed evidence. ROOT reviewed the precise Tasks pair
and this same work order, then explicitly released the required-CI selected-parent
correction and its scoped checks in the same primary session. Corrective Results
below record the completed implementation; merge remains separately authorized.
The original hosted observer has actually joined (native 38891, chunk c9e81b,
exit 1); its owned group 2392849 is absent. Retain its original clock/report,
not a replacement observer. Delivery/retry bookkeeping stays external.

### Source responsibility and boundaries

`apps/web/components/task/new-subtask-dialog.tsx` owns the correction.
Use the actual `parentTaskId` to read the canonical `taskOverview.byId` record,
following the existing canonical-first selector in `task-page-content.tsx`;
retain a compatibility lookup only if needed for existing board-only callers.
Do not change that shared selector, global store, task inventories, or fetch
unrelated lists. The active task/session must never substitute for the target.

Preserve primary selection: explicit `primarySessionId`, otherwise a selected
parent session marked `is_primary`. Secondary session ordering must not select
another default. Repository order/branch and primary-session fallback remain
as currently resolved. The existing `useTaskSessions(parentTaskId)` owns reads,
loaded/empty/error state, refresh, and session options; its implementation is
read-only. Existing active-parent behavior is a control.

Change mount-only seeding to local, one-time delayed initial seeding. Seed the
repository when known only while untouched (reuse `repositoriesDirty`, including
intentional removal, and respect existing/remote rows); seed profile only while
untouched; resolve initial workspace mode once primary context is available or
the existing session read settles loaded/error. Track deliberate profile/mode
choices locally through existing handlers, including clearing a choice. Later
refreshes must not reseed. The dialog caller's submission fallback must respect
explicit profile clearing. Source interactions (existing repository dirty/remote
mode/remote-row state) lock the initial new-workspace mode so late worktree
data cannot discard a selected or removed source. Verify the actual `createTask`
request for profile clearing, repository choice/removal, and remote paste, keeping the
shared submit hook and ordinary omitted-profile backend defaulting unchanged.
No generic revisions/coordinator policy.

This is smaller than full-context readiness gating: the existing form stays
mounted and editable during reads. No new remount/key, spinner, visible copy,
geometry, submission contract, prompt-copy policy, or forced active-parent
selection. Entered title/prompt and repo/profile/workspace choices survive delayed
success. Loaded no-session, error, and missing-parent cases remain usable with
existing empty/default controls and validation, never indefinitely loading.
`new-subtask-form-state.ts`, shared form parts/handlers, session hooks, backend,
and sidebar action callers remain read-only unless a demonstrated bounded
failure requires revising this design first.

### Independent regression and controls

Add `apps/web/components/task/new-subtask-dialog.test.tsx`, rendering the real
dialog, form state, and existing form controls against a reactive store. Stub
external data/transport boundaries only, not parent lookup or seeding. Before
production edits, prove RED for a selected non-active parent present only in
`taskOverview.byId`, with a different active kanban task and delayed successful
selected-parent sessions. Observe the actual repository chip, profile, workspace
mode, and retained entered title/prompt. Use explicit expected IDs/labels.

Minimal controls: already-loaded active parent; delayed primary plus differently
configured secondary (primary wins); loaded no-session and missing/error context
(form remains usable); deliberate repo removal/replacement, profile, and workspace
choice made before delayed success remain unchanged, as do title/prompt. A later
refresh does not overwrite initial/user values. Extend the existing
`new-subtask-dialog-context.test.ts` only for needed primary-selection controls.
Trace tests to AC-TASKS-SUBTASK-DIALOG-PARENT-DEFAULTS-001.1–.5:
canonical-only selected parent (.1), initial primary/repository/mode defaults
(.2), entered work/explicit choices/refresh (.3), missing/error/no-session (.4),
and existing desktop/phone controls (.5). Office AC .6 is not correction coverage.

### Sequential checks after corrective release

Use the existing toolchain/dependencies and one local-heavy lease. Retain actual
RED/GREEN exits and joins; then run only the four affected component/state suites:

```bash
pnpm exec vitest run components/task/new-subtask-dialog.test.tsx components/task/new-subtask-dialog-context.test.ts components/task/new-subtask-form-state.test.ts components/task/new-subtask-form-parts.test.tsx --maxWorkers=1
pnpm exec eslint --max-warnings 0 components/task/new-subtask-dialog.tsx components/task/new-subtask-dialog.test.tsx components/task/new-subtask-dialog-context.test.ts
NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck
pnpm e2e:run --host --shards 1 --project mobile-chrome tests/task/mobile-sidebar-task-actions.spec.ts -- --grep 'Mobile sidebar task actions.*uses the selected non-active parent defaults for mobile subtasks$' --retries=0 --workers=1
```

The grep omits a start anchor because Playwright includes project/file titles
before the suite name; its selected scenario and end anchor remain exact.
Run the mobile command once after GREEN under the normal managed build/run/cleanup
runner, preserving the original assertion and timeouts; no test-file edit is
planned. Do not replay the unchanged 178 keyboard tests. Use actual owned paths
for scoped lint, normal staged `i18n:ratchet`, active hooks, whitespace checks,
coverage API, catalog validation, and all-spec lint. Coverage dry-run includes
the existing four artifacts, this correction's related Tasks requirement/design
pair, and the three projected correction source/test paths.
The UI correction needs a bounded substantive assessment of that diff; completed
keyboard reports remain reusable. No routine full review requests or optional
polish gate. Append actual corrective Results only after execution.


### Initial addendum design checks

Catalog validation (382 decisions, 1609 specs), all-spec lint, working/index
whitespace checks, and projected owning coverage passed (`ok: true`, `errors: []`).
Native 93561 joined at 3a4cc9, exit 0; its current owned process group is absent.
No implementation, permanent test, heavy command, hook, commit, or push ran.


The precise Tasks-pair amendment passed catalog validation (382 decisions,
1611 specs), all-spec lint, whitespace checks, and the updated coverage dry-run
(`ok: true`, `errors: []`) against the final requirement/design references.
Native 42329 joined at ac6a5b, exit 0; current owned group absent. Only the
related Tasks pair and existing manifest/order changed; implementation remains
pending a later explicit corrective release and local-heavy lease.


## Corrective Results

- Independent rendered RED used the real dialog, form state, existing controls,
  and reactive canonical task inventory retained through the real store action.
  `vitest run components/task/new-subtask-dialog.test.tsx components/task/new-subtask-dialog-context.test.ts --maxWorkers=1`
  exposed four expected failures and ten passing controls; GREEN passed 14/14.
  Earlier fixture setup/hydration attempts are retained separately as setup
  evidence, not substituted for the qualified canonical-only parent regression.
- The actual create boundary exposed a cleared selector still submitting the
  parent profile (RED one failure plus one passing normal-default control).
  The dialog caller now removes that fallback after an explicit profile choice;
  the same selected tests passed GREEN 2/2. Shared submit logic stays unchanged.
- Actual selected and pasted source requests exposed late worktree data switching
  to inheritance and omitting repositories (RED three failures plus one untouched
  auto-inheritance control). Existing source-dirty/remote state now settles the
  initial new-workspace mode. Request regressions preserve selected, removed, and
  pasted sources, along with explicit mode and profile choices. The pasted GitHub
  request retains its existing `github_url` field; an incorrect test-field
  expectation was corrected without changing conversion or assertions on source.
- The final exact four-file Vitest command passed 22/22 tests. Scoped ESLint on
  the three actual owned TS/TSX paths passed with zero warnings. Initial duplicate
  test literals/nested ternary lint feedback was corrected within the same tests.
- Current `NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck` passed after
  the final source guard. An initial failure in owned test fixture types was
  corrected with existing branded-ID constructors, optional-map access, the sparse
  hydration cast, and the supported Testing Library options. Failed receipts were
  retained; necessary reruns followed those actual errors and the later source fix.
- The exact managed mobile E2E command above passed its original scenario once
  (1 test, one worker, zero retries, normal host build/run/cleanup). Assertions,
  timeouts, test file, and fixture were unchanged. It ran before the later manual
  source-interaction guard; that guard is inactive in this exact no-source-edit
  flow. Final rendered/request tests cover the material later diff, so no second
  runtime build or E2E replay was performed. The grep start anchor was removed
  because Playwright prepends project/file titles; the selected scenario is unchanged.
- Production ownership is only `new-subtask-dialog.tsx`; the two dialog test paths
  are owned. Shared form state/parts, submit/session hooks, sidebar callers, backend,
  keyboard changes, and the E2E test/fixture remain unchanged. No new product copy,
  layout, public API, profile eligibility, or persistence change was introduced.

- Normal staged `pnpm run i18n:ratchet` passed with all owned paths in the index
  and normal selectors/exclusions (four modified production candidates clean).
  Catalog validation passed (382 decisions, 1611 specs), all-spec lint and working/
  staged whitespace checks passed, and the updated six-artifact/ten-source coverage
  API preflight returned `ok: true`, `errors: []`. Normal hooks and publication
  receipts remain in the external operational checkpoint.


## Main extraction compatibility

### Evidence and intent

Current head `9ec419782599b8b1b0d5d185fec0d4cf8b120515` is published and its
corrective Results remain historical evidence. Fresh REST mergeability is false
(`dirty`), and the complete snapshot lists all six required checks as missing.
The recorded PR base is `71ada983`; the actual main target is `148c84f072737ba7bc089960a321944f9e9088cb`.
Main #4476 removes the two inline parent seed hooks and calls its new
`useSubtaskParentDefaults`. This is the sole overlapping correction path.
Main's form-state blob is unchanged; its helper's two existing tests cover
late defaults and prior edits. No inspection or repair of the other main changes
is needed for this conflict proposal.

### Exact ownership

- `apps/web/components/task/use-subtask-parent-defaults.ts`: retain this main
  owner and reconcile our repository seed-once/dirty/existing-or-remote-choice
  guard and profile seed-once/explicit-touch handling into it. Expose the local
  profile change handler and effective submission fallback to its caller;
  preserve existing helper callers/test inputs without a generic policy API.
- `apps/web/components/task/new-subtask-dialog.tsx`: retain canonical selected
  parent and primary-context resolution, stable form lifetime, initial workspace
  mode/source-choice guards, and the effective profile fallback passed to the
  unchanged submit hook. Wire the existing shared profile handler through the
  extracted helper; remove duplicate inline repository/profile seed hooks.
- Reuse `new-subtask-dialog.test.tsx`, `new-subtask-dialog-context.test.ts`, and
  main's `use-subtask-parent-defaults.test.ts` as regression/control owners.
  Change test inputs only if the minimal helper wiring requires them. No new
  test-only export or injection seam. The same Tasks design needs only its
  component-ownership wording synchronized during the released integration.

Preserve main's adopted-checkout behavior and all other main files unchanged.
Shared form state/parts, submit/session hooks, sidebar callers, backend adoption,
API/eligibility behavior, keyboard matching, copy/layout, and existing acceptance
criteria remain outside production ownership.

### Sequential integration after later release

Verify exact own branch/head, clean index, actual remote head, and fresh main
identity before the ordinary local base merge for this concrete conflict. Keep
main's extraction and port our invariants to its owner; do not choose a side
merely to remove markers. Use existing rendered/request cases for a meaningful
RED of lost explicit-profile behavior before implementing the helper guard,
then GREEN. Preserve all failed receipts. Normal merge/commit hooks stay active;
no bypass, dependency reinstall, or drift-only rebase.

Run the focused five-file selection sequentially from `apps/web`:

```bash
pnpm exec vitest run components/task/new-subtask-dialog.test.tsx components/task/new-subtask-dialog-context.test.ts components/task/use-subtask-parent-defaults.test.ts components/task/new-subtask-form-state.test.ts components/task/new-subtask-form-parts.test.tsx --maxWorkers=1
pnpm exec eslint --max-warnings 0 components/task/new-subtask-dialog.tsx components/task/use-subtask-parent-defaults.ts components/task/new-subtask-dialog.test.tsx components/task/new-subtask-dialog-context.test.ts components/task/use-subtask-parent-defaults.test.ts
NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck
```

Use normal staged i18n selectors, actual owned-path coverage/spec/diff checks,
and active hooks. Preserve incoming main changes rather than independently
replay backend/adoption or keyboard suites. No new mobile runtime build is
planned: the exact passed scenario's data/controls remain the contract and the
rendered regressions cover the extracted owner; a concrete integration failure
that changes that flow must be checkpointed before expanding validation.

Retain observer 27809 and its 18:24:21/19:54:21 UTC clock; any replacement must
follow its actual join/current owned absence, never an inferred completion.
ROOT must review this proposal and send a later explicit integration request
and local-heavy lease. No production mutation, base merge, rebase, or new observer
was performed at this checkpoint.


### Compatibility Results

- ROOT reviewed and later released the bounded main-helper integration. The
  ordinary `git merge --no-commit 148c84f072737ba7bc089960a321944f9e9088cb`
  reported the expected dialog conflict; no rebase or side selection dropped
  incoming behavior. Index blob comparison confirms the other 66 incoming
  paths equal main, with no unmerged paths after resolution.
- Main's helper is the sole repository/profile seed owner. Existing caller
  source-state and mode guards remain; the helper's explicit profile handler
  suppresses late reseeding and its effective fallback reaches the unchanged
  submit hook. Tests were reused without changing their assertions or inputs.
- The existing rendered cleared-profile case exposed late reseeding under
  main's original helper (one expected failure, active-parent control passing).
  GREEN passed both selected cases, including the actual omitted-profile
  create request. The exact five affected suites then passed all 24 tests.
- Scoped ESLint and the current 8 GiB typecheck passed. Failed RED and merge
  conflict receipts remain under the existing external receipt prefix. No
  keyboard/backend suite replay, dependency install, or second mobile build
  was performed. Normal staged i18n, owning coverage/spec/diff checks, active
  merge hooks, and bounded ROOT semantic assessment precede publication.
- Original corrective observer 27809 was gracefully cancelled for the actual
  merge conflict and joined at native chunk `93d3f7`, exit 143. Its owned group
  3108686 is absent. This is a no-verdict conflict receipt, not CI success or
  a spent retry. Its original clock and prior observer receipts are retained;
  no replacement observer overlaps it.

- Normal staged i18n passed with all five modified candidates clean and the
  643-entry guard allowlist intact. Catalog validation (383 decisions, 1613
  specifications), all-spec lint, working/index whitespace checks, and owning
  six-artifact/eleven-source coverage passed (`ok: true`, `errors: []`). ROOT
  accepted the complete integrated two-production-file diff and incoming-path
  preservation receipt before publication. Normal merge hooks and hosted
  current-head validation remain delivery gates, not inferred successes.
