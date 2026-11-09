---
id: "01-selected-workflow-metadata"
title: "Repair selected workflow metadata"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-AGENT-OVERRIDES-001
  - REQ-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001
  - REQ-TASKS-TASK-CREATE-LAUNCH-PREVIEW-002
acceptance_criteria:
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.1
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.6
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.7
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.8
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.10
  - AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001.2
  - AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001.3
  - AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001.4
  - AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-002.3
  - AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-002.5
system_design:
  - ../../specs/tasks/system-design/workflow-agent-overrides.md
  - ../../specs/tasks/system-design/task-create-launch-preview.md
---

# Task 01: Repair selected workflow metadata

## Summary

Give the task dialog explicit demand for complete selected-workflow steps through
the existing shared metadata owner. Use the result for replacement validation and
launch preview, with recoverable failure and current-scope publication.

## In scope

- Apply `/tdd`. First add `loads selected workflow metadata without a snapshot or pending board read`
  against the dialog adapter. Observe missing rows or permanent loading before the repair.
- Add `use-task-create-workflow-steps` with explicit state, targeted Retry, full mapping,
  scope masking, and shared-read cancellation. Cover missing and placeholder snapshots
  with an idle board reader, plus complete loaded and successful-empty controls.
- Migrate `useWorkflowStepsEffect` and its callers to one selected metadata observer.
  Thread the result and Retry through state, setup, validation, and prop builders.
  Preserve current form/reset/submission semantics and launch precedence.
- Cover delayed responses, workflow/workspace/auth changes, open cycles, failure/retry,
  shared consumers, complete fields, and preview/group consistency with focused tests.
- Add desktop/phone `cold workflow metadata` cases for both entry variants in the plan.
  Keep the loaded-board controls. Prove B metadata loads without B task snapshots.
- If new copy is necessary, translate all seven languages and generate Traditional Chinese.

## Out of scope

Backend/routing/schema changes, new caches, runtime flags, all-board loading,
form redesign, and existing-task override editing.

## Acceptance

1. A fresh A-board page can create a task in unopened B with the selected replacement.
   Explicit and remembered B selection both work on desktop and phone.
2. Selected metadata success, empty, failure, Retry, and stale-result handling match
   the design. Grouping and launch preview consume the same complete current result.
3. The exact checks below pass. Results include failed-before/passed-after evidence,
   B-snapshot absence, request observations, and phone interaction evidence.

## ASCII UI preview

UI-01, desktop, after successful selected metadata:

```text
Workflow agents                       [Reset]
  Changes apply only to this task.
  Source profile: A  Implement, PR     [Use workflow profile v]
```

UI-02, phone, same result:

```text
Workflow agents               [Reset]
Changes apply only to this task.
Source profile: A
Implement, PR
[ Use workflow profile             v ]
```

Failure replaces rows with the localized error and reachable Retry. Loading ends
when the admitted read settles. See the [full state previews](plan.md#ascii-ui-preview).
Preserve the existing phone form, body scroll owner, safe-area footer, and picker.
These views map to criteria .1, .7, and .8.

## Verification

Run from the repository root. In a fresh worktree without dependencies, first run
`(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/web && pnpm exec vitest run hooks/use-task-create-workflow-steps.test.ts hooks/journey-metadata-resources.test.ts components/task-create-dialog-effects.test.ts components/task-create-dialog-state.test.ts components/task-create-dialog-setup.test.ts components/task-create-dialog-workflow-agent-override-validation.test.ts components/task-create-dialog-workflow-agent-overrides.test.ts components/task-create-dialog-launch-preview.test.ts components/task-create-dialog-advanced-settings.test.tsx components/task-create-dialog-composition.test.tsx components/task-create-dialog-prop-builders.test.ts components/task-create-dialog-submit.test.tsx components/task-create-dialog.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint hooks/use-task-create-workflow-steps.ts hooks/use-task-create-workflow-steps.test.ts components/task-create-dialog-effects.ts components/task-create-dialog-state.ts components/task-create-dialog-setup.ts components/task-create-dialog-types.ts components/task-create-dialog-prop-builders.ts components/task-create-dialog-workflow-agent-override-validation.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/task/create-task-workflow-agent-overrides.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-create-task-workflow-agent-overrides.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run this local traceability preflight from the repository root:

```bash
node <<'NODE'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/plans/task-create-workflow-agent-metadata/plan.md',
  'docs/plans/task-create-workflow-agent-metadata/task-01-selected-workflow-metadata.md',
  'docs/specs/tasks/requirements/workflow-agent-overrides.md',
  'docs/specs/tasks/system-design/workflow-agent-overrides.md',
  'docs/specs/tasks/requirements/task-create-launch-preview.md',
  'docs/specs/tasks/system-design/task-create-launch-preview.md',
];
const base = execFileSync('git', ['merge-base', 'HEAD', 'origin/main'], { encoding: 'utf8' }).trim();
const committed = execFileSync('git', ['diff', '--name-only', base, 'HEAD', '-z'], { encoding: 'utf8' }).split('\0').filter(Boolean);
const working = execFileSync('git', ['diff', '--name-only', 'HEAD', '-z'], { encoding: 'utf8' }).split('\0').filter(Boolean);
const untracked = execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], { encoding: 'utf8' }).split('\0').filter(Boolean);
const changedFiles = [...new Set([...committed, ...working, ...untracked])].map(filename => ({ filename, status: fs.existsSync(filename) ? 'modified' : 'removed', additions: 1, changes: 1 }));
const fileContents = Object.fromEntries(paths.map(path => [path, fs.readFileSync(path, 'utf8')]));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({ ok: result.ok, status: result.status, errors: result.errors }, null, 2));
process.exitCode = result.ok ? 0 : 1;
NODE
```

Use the managed E2E builds and isolated fixtures. Run the two projects sequentially.
Record discovered/executed cases and results. A browser blocker leaves browser
acceptance pending. The preflight must report `covered` once runtime changes exist.

## Files likely touched

Owned frontend files, under `apps/web/`:

- `hooks/use-task-create-workflow-steps.ts` and `.test.ts` (new).
- `hooks/journey-metadata-resources.test.ts` (existing shared owner regression).
- `components/task-create-dialog-effects.ts` and `.test.ts`.
- `components/task-create-dialog-state.ts` and `.test.ts`.
- `components/task-create-dialog-setup.ts` and `.test.ts`.
- `components/task-create-dialog-types.ts`.
- `components/task-create-dialog-prop-builders.ts` and `.test.ts`.
- `components/task-create-dialog-workflow-agent-override-validation.ts` and `.test.ts`.
- `components/task-create-dialog-composition.test.tsx`.
- `components/task-create-dialog-workflow-agent-overrides.test.ts`.
- `components/task-create-dialog-launch-preview.test.ts`.
- `components/task-create-dialog-advanced-settings.test.tsx`.
- `components/task-create-dialog-submit.test.tsx`.
- `e2e/tests/task/create-task-workflow-agent-overrides.spec.ts`.
- `e2e/tests/task/mobile-create-task-workflow-agent-overrides.spec.ts`.
- `e2e/tests/task/task-workflow-agent-overrides-helpers.ts` if shared fixture support is needed.

Update this work order and `plan.md` statuses/results. Additional mock consumers
identified by typecheck belong to this same wiring repair and need focused checks.
The canonical mapper and shared reader are existing inputs, not new cache owners.

## Dependencies

None. The original override delivery package is complete. Read its existing
fixtures and preserve their runtime routing coverage.

## Risks

An effect-only fix can still expose old results for one render. Mask by identity
before effects. A remembered workflow test must not warm B by opening its picker.
The form's fallback/default resolution must not become another source of stale steps.

## Parallelism

`sequential`

## Inputs

- [Plan, root cause, test matrix, and scope](plan.md).
- [Requirements](../../specs/tasks/requirements/workflow-agent-overrides.md), criteria .1, .6, .7, .8, .10.
- [Design](../../specs/tasks/system-design/workflow-agent-overrides.md#selected-workflow-metadata).
- [Launch requirements](../../specs/tasks/requirements/task-create-launch-preview.md)
  and [design](../../specs/tasks/system-design/task-create-launch-preview.md).
- [Shared loading contract](../../specs/platform/system-design/journey-data-loading.md#shared-read-ownership).
- Existing `useWorkflowOptionPreviews` for shared-reader and lifecycle patterns.
- Existing mapper `workflowStepToState` and override E2E fixture/page objects.

## Results

Implemented `useTaskCreateWorkflowSteps` as the dialog's selected-workflow
metadata owner. It seeds complete current-workspace snapshots, otherwise reads
complete step definitions through `readJourneyWorkflowSteps`, masks stale
workspace, workflow, auth, open-cycle, retry, and selected-workflow activation
identities, aborts on close or unmount, and exposes targeted Retry with shared
refresh demand. Each activation, including a return to an earlier workflow or a
previously failed workflow, starts from loading instead of publishing retired
metadata. Failed snapshots that retain stale steps are rejected so the
lightweight read runs, and edit/session dialogs establish no selected-metadata
demand. The result now feeds grouping, launch preview, validation, and the
existing default-step bridge. `useWorkflowStepsEffect` and the snapshot-error
Retry wiring were removed.

Added focused hook, validation, mapper, bridge, and shared-owner regressions.
Extended the desktop and phone override specs with explicit and remembered
`cold workflow metadata` cases. Both projects proved profile B loads from an
unopened workflow while board A remains selected, no B task snapshot was
requested, Retry recovered a failed draft, and the created task persisted the
step-bound replacement.

Verification:

- Focused Vitest: 13 files, 191 tests passed. The affected edit-mode save/retry
  suite passed 11 additional tests.
- `pnpm run typecheck`: passed.
- Focused ESLint: passed with no warnings.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet`: passed.
- Desktop `create-task-workflow-agent-overrides.spec.ts`: 4 tests passed.
- Phone `mobile-create-task-workflow-agent-overrides.spec.ts`: 3 tests passed.
  Phone checks used managed Playwright mobile-chrome emulation; no physical
  device was available in this environment.
- Documentation validation, traceability preflight (`covered`), and
  `git diff --check`: passed.
