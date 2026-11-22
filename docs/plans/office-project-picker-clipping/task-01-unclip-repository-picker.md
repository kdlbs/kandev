---
id: "01-unclip-repository-picker"
title: "Restore the floating repository picker"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-DIALOG-CONTENT-CONTAINMENT-001
acceptance_criteria:
  - AC-UI-DIALOG-CONTENT-CONTAINMENT-001.1
  - AC-UI-DIALOG-CONTENT-CONTAINMENT-001.2
  - AC-UI-DIALOG-CONTENT-CONTAINMENT-001.3
  - AC-UI-DIALOG-CONTENT-CONTAINMENT-001.4
  - AC-UI-DIALOG-CONTENT-CONTAINMENT-001.5
  - AC-UI-DIALOG-CONTENT-CONTAINMENT-001.6
  - AC-UI-DIALOG-CONTENT-CONTAINMENT-001.7
  - AC-UI-DIALOG-CONTENT-CONTAINMENT-001.8
system_design:
  - ../../specs/ui/system-design/dialog-content-containment.md
---

# Task 01: Restore the Floating Repository Picker

## Summary

Restore repository picker hit testing outside the New Project dialog shell.
Keep the contained form, persistent footer, project-detail picker, and current
repository selection semantics. Prove desktop and phone behavior through the
actual React portal.

## In scope

- Add the three named browser regressions in the plan before production edits.
- Capture the mounted project dialog shell and pass it to the picker through
  an optional container prop. Use the existing Popover portal support.
- Change the shell to visible overflow while retaining body containment.
- Bound picker width and available height, with a shrinking scrollable list.
- Keep existing desktop and phone long-chip cases and creation/detail flows.
- Record final results here and synchronize both linked containment packages.

## Out of scope

- Repository APIs, discovery state ownership, value normalization, and stores.
- Shared UI primitives, task-dialog context, new copy, and a phone drawer.
- Other dialogs or Office form fields.

## Acceptance

1. After one repository chip moves the trigger right, the full desktop picker
   remains hit-testable beyond the dialog. Wheel and keyboard selection work.
2. At phone and short-phone sizes, the picker fits the viewport and its touch
   actions remain reachable. Selection persists after project creation/reload.
3. Opening, selecting, scrolling, Escape, outside dismissal, and reopening
   preserve the form and focus. Existing detail and long-chip cases still pass.

## Implementation notes

Use the existing `DialogContent` callback-ref pattern from `TaskCreateDialog`.
Pass the mounted element through `ProjectFormBody` and `ReposField` to
`ProjectRepositoryPicker`. Remove the picker's `portal={false}` override.
Standalone detail use omits `portalContainer` and retains the default target.

Keep the shell's viewport caps and auto/minmax/auto rows. The body retains
`min-h-0 min-w-0 overflow-x-hidden overflow-y-auto overscroll-contain`.
The shell uses visible overflow and does not become a horizontal scroller.

Keep the preferred picker width at 420px. Apply a viewport cap with 16px side
insets and matching Radix `collisionPadding`. Use Radix's available-height
value to constrain the picker. Keep search reachable and allow the command
list to shrink and scroll. Preserve existing desktop and touch control sizing.

Use the real React portal. Do not implement DOM relocation from the diagnostic
prototype. Keep component functions within the scoped lint limits.

## ASCII UI preview

Relevant excerpts from [UI-01 and UI-02 in the plan](plan.md#ascii-ui-preview):

```text
UI-01 Desktop: picker extends beyond the dialog border
| [repo chip] [Add repository] |
|             +----------------------+
|             | Search or paste ...  |
|             | Options (scroll)     |
| [Cancel]    +----------------------+
+-----------------------------+

UI-02 Phone: picker fits the viewport
| [Add repository]            |
| +-------------------------+ |
| | Search or paste ...      | |
| | Options (scroll)         | |
| +-------------------------+ |
| [Cancel]                    |
| [Create Project]            |
```

The plan defines the full before/after views. Search precedes the scrolling
list, and form actions retain their persistent row. These structural rules map
to `.1` through `.8`. Sample spacing and popover placement are illustrative.

## TDD sequence

1. Add shared test setup with 18 deterministic discovery suggestions. Use the
   real dialog and picker. Set the desktop reproduction to 1280 x 720.
2. Add one repository chip and reopen the picker. Assert an option's right-side
   point beyond the dialog resolves to that option, then select there.
3. Record behavioral RED: the current point resolves to `dialog-overlay`.
   The phone width regression separately fails at 393 x 851 and 320 x 568.
4. Implement the local portal and geometry correction.
5. Prove wheel delivery over the picker outside the shell. Use real keyboard
   navigation from the visible opener. Do not programmatically focus search.
6. Verify ArrowDown/Enter selects without submitting the project. Verify Escape
   dismisses only the picker and restores its trigger. Cover outside dismissal.
7. Use real phone taps, check actual target bounds, and create/reload the
   project to verify repository persistence. Check short-viewport list scrolling.
8. Run both complete affected spec files sequentially. Retain all earlier
   scenarios and record actual final results.

Wait for finite animations before geometry reads. Use hit testing and actual
input in addition to visibility. Resolve the picker from its trigger's
`aria-controls` or a unique visible picker selector. Parent-dialog queries use
the stable `create-project-dialog` seam. Do not use forced clicks or synthetic
DOM relocation to make permanent regressions pass.

## Verification

Run this block from the repository root. The install is required once per
fresh worktree. Managed E2E commands build production assets and clean up.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run --host --project chromium -- tests/office/project-repository-picker.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome -- tests/office/mobile-project-create-dialog.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint app/office/projects/create-project-dialog.tsx app/office/projects/project-repository-picker.tsx e2e/helpers/project-repository-picker.ts e2e/tests/office/project-repository-picker.spec.ts e2e/tests/office/mobile-project-create-dialog.spec.ts)
(cd apps/web && pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/office-project-picker-clipping
```

Run this local coverage preflight from the repository root. The two planned
source paths also validate references before implementation exists.

```bash
node <<'JS'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [...new Set([
  ...execFileSync('git', ['diff', '--name-only', 'HEAD', '-z'], {encoding:'utf8'}).split('\0'),
  ...execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], {encoding:'utf8'}).split('\0'),
  'apps/web/app/office/projects/create-project-dialog.tsx',
  'apps/web/app/office/projects/project-repository-picker.tsx',
])].filter(Boolean);
const documents = [
  'docs/specs/ui/requirements/dialog-content-containment.md',
  'docs/specs/ui/system-design/dialog-content-containment.md',
  'docs/plans/dialog-content-containment/plan.md',
  'docs/plans/dialog-content-containment/task-05-create-project.md',
  'docs/plans/office-project-picker-clipping/plan.md',
  'docs/plans/office-project-picker-clipping/task-01-unclip-repository-picker.md',
];
const result = validateCoverage({
  changedFiles: paths.map(filename => ({filename, status:'modified'})),
  fileContents: Object.fromEntries(documents.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
JS
```

## Files likely touched

- `apps/web/app/office/projects/create-project-dialog.tsx`.
- `apps/web/app/office/projects/project-repository-picker.tsx`.
- `apps/web/e2e/helpers/project-repository-picker.ts` (new shared helper).
- `apps/web/e2e/tests/office/project-repository-picker.spec.ts`.
- `apps/web/e2e/tests/office/mobile-project-create-dialog.spec.ts`.
- `docs/plans/office-project-picker-clipping/plan.md` and this work order.
- `docs/plans/dialog-content-containment/plan.md` and `task-05-create-project.md`
  for follow-up links and accurate final results, without replacing historical
  evidence.

## Dependencies

None. The existing requirement, revised design, and portal support are present.

## Risks

- A modal can block wheel or focus events from an incorrectly placed portal.
- Width containment alone does not prove that an ancestor permits hit testing.
- Height clipping can hide search or final options on short viewports.
- A stale production build can hide the actual result. Use managed builds.

## Parallelism

`sequential`. One focused RED/GREEN cycle owns both viewports.

## Inputs

- [Plan, evidence, and test matrix](plan.md).
- [Requirement](../../specs/ui/requirements/dialog-content-containment.md).
- [Office Create Project design](../../specs/ui/system-design/dialog-content-containment.md#office-create-project).
- `apps/web/components/task-create-dialog.tsx` and
  `apps/web/components/task-create-dialog-repository-options.tsx`.
- Existing picker and long-chip E2E cases and
  `apps/web/e2e/helpers/layout-assertions.ts`.
- Scoped web guidance and `/tdd`, `/e2e`, `/mobile-parity` skills.

## Results

### RED (production unchanged)

The shared helper seeds 18 deterministic on-disk discovery suggestions; the
seeded workspace also contributes its own registered repository, so tests
filter suggestions by their `/srv/repositories/` prefix instead of asserting a
total option count.

| Case | Observed failure |
| --- | --- |
| `create-project dialog: repository picker escapes the dialog without clipping` | The 420-pixel picker rendered at x=700..1120 while the dialog ended at x=896. The first suggestion's right-side point (1119.0, 148.2) resolved to `dialog-overlay`, so the clipped area was not interactive. |
| `create-project dialog: repository picker scrolling and keyboard dismissal preserve the form` | Wheel input over the painted picker left the command list `scrollTop` at 0. |
| Retained `create-project dialog: long repository chips keep the footer reachable` | Clicking the centre of the custom-entry row hit `dialog-overlay` once chips moved the trigger right, confirming the pre-existing local left-side click workaround. |

The desktop run recorded 4 passed and 3 failed. The phone width regression was
the same fixed 420-pixel popover inside a 361-pixel dialog with no viewport cap.

### GREEN

- `create-project-dialog.tsx` captures the mounted `DialogContent` with a
  callback ref, passes it through `ProjectFormBody` and `ReposField`, and the
  shell uses `overflow-visible` instead of `overflow-clip`. The height cap,
  auto/minmax/auto rows, and body containment are unchanged.
- `project-repository-picker.tsx` uses `PopoverContent`'s existing portal with
  the optional `portalContainer`. The project-detail consumer omits the prop and
  keeps the document-body target. The picker keeps its 420-pixel preferred
  width, adds `max-w-[calc(100vw-2rem)]`, 16-pixel collision padding, and an
  available-height cap. The command root and list can shrink, so the search
  control stays fixed while the option list scrolls.
- Selection callbacks, discovery timing, exclusions, query reset, and project
  submission are unchanged.

### Verification

| Check | Result |
| --- | --- |
| `pnpm e2e:run --host --project chromium -- tests/office/project-repository-picker.spec.ts` | 7 passed (5 retained, 2 new) in 32.3 s. |
| `pnpm e2e:run --host --project mobile-chrome -- tests/office/mobile-project-create-dialog.spec.ts` | 2 passed (long-chip retained, picker new) in 20.7 s. |
| `pnpm run typecheck` | Clean. |
| Targeted `eslint` on both source files, the helper, and both specs | Clean. |
| `pnpm run i18n:ratchet` | 0 added and 2 modified files clean; guard allowlist intact. |
| `python3 scripts/list-docs.py validate` | 415 decisions and 1824 specifications. |
| `python3 scripts/lint-spec-files.test.py` | 36 tests passed. |
| `python3 scripts/lint-spec-files.py --all` | All specification files passed. |
| Coverage preflight and `git diff --check` | Passed. |

The new browser cases prove the desktop picker paints past the dialog border
with an actual right-side hit and pointer selection, wheel delivery to the list,
search focus, ArrowDown/Enter selection without submitting, Escape and outside
dismissal with focus return, and the retained footer/body behavior. The phone
case proves 393 x 851 and 320 x 568 containment, 44-pixel trigger and search
targets, a shrinking scrolling list, real touch selection, and repository
persistence after project creation and reload. The result matches UI-01
(desktop picker beyond the shell, footer retained) and UI-02 (phone picker inside
the viewport, search first, list scrolling).

### Cleanup and residual risks

The managed E2E runs tore down their backends, agentctl processes, temporary
repositories, and Vite assets. No diagnostic artifacts remain. Residual risk:
collision placement above versus below the trigger is Radix-owned and not
asserted, and a body-level portal for the project-detail picker relies on the
existing default target rather than a new container.
