---
created: 2026-11-02
status: done
requirements:
  - REQ-TASKS-WORKFLOW-AGENT-OVERRIDES-001
  - REQ-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001
  - REQ-TASKS-TASK-CREATE-LAUNCH-PREVIEW-002
system_design:
  - ../../specs/tasks/system-design/workflow-agent-overrides.md
  - ../../specs/tasks/system-design/task-create-launch-preview.md
legacy_specs: []
---

# Selected workflow agent metadata repair

## Overview

[Issue #4999](https://github.com/kdlbs/kandev/issues/4999) reports permanent loading
in New Task > Advanced settings > Workflow agents for an unopened workflow.
One sequential work order repairs the metadata consumer and proves the complete
task creation flow on desktop and phone. Implementation is complete, with unit,
rendered, and browser evidence recorded below.

Tasks owns this repair because task creation owns profile replacement choices.
Platform retains the existing shared read owner and bounded board loading contract.

## Evidence and root cause

Source inspection and a temporary diagnostic ran at
`f648e79f9ecc7d5db0db301f20d0b23c7cb44d97` on 2026-11-02.
The authenticated GitHub user is `carlosflorencio`. Issue assignment was verified
after the source trace and diagnostic established a reliable repair path.

`resolveSnapshotState` in
`apps/web/components/task-create-dialog-workflow-agent-override-validation.ts`
treats missing snapshots and placeholders as loading. No snapshot request is
admitted there. `useTaskCreateDialogData` collects a pending flag, but validation
does not consume it. The independent `useWorkflowStepsEffect` fetch succeeds
without creating a snapshot. Its mapper drops fixed profiles and session targets.
Retry currently invokes `requestWorkspaceContextRefresh`, which owns different data.

The temporary `components/issue-4999-diagnostic.test.ts` exercised production
validation and the production fallback effect. All four checks passed:

- An absent Feature snapshot and idle board reader produce loading with no rows.
- Complete placeholder steps still produce loading, despite an Implement/PR group.
- The same steps in a loaded snapshot produce a settled Implement/PR group.
- A successful fallback step read drops `agent_profile_id` and `session_target`.

Command: `(cd apps/web && pnpm exec vitest run components/issue-4999-diagnostic.test.ts)`.
Result: one file, four diagnostic assertions passed. The temporary file was removed.
These assertions prove current failure behavior, not a corrected implementation.
The canonical issue contains prior live/API evidence and no image attachments.
No browser reproduction or live-instance mutation occurred in this planning turn.

The smallest reproduction uses board A as the only loaded snapshot, workflow B
with one fixed Implement profile, and B selected only in New Task. The current
desktop and phone E2E fixtures select their target as the board filter first.
That setup supplies the snapshot and misses the failure.

## Requirement conformance and assumptions

The active [override requirement](../../specs/tasks/requirements/workflow-agent-overrides.md)
already defines rows, scope reset, parity, recovery, and summary consistency.
This is an implementation violation of criteria .1, .6, .7, .8, and .10.
No new requirement ID or changed routing contract is necessary.

The current design's snapshot dependency is incomplete. Its selected metadata
section now specifies the repair. Board demand remains bounded under
[journey loading](../../specs/platform/system-design/journey-data-loading.md#homepage).
The existing shared read owner resolves the technical choice. No material product
choice remains open. No ADR is necessary for this local consumer correction.

## Scope

### In scope

- Selected-workflow metadata demand with explicit state and targeted Retry.
- Complete profile/recipient fields and one result for overrides and launch preview.
- Missing/placeholder snapshot recovery and current-scope result publication.
- Cold explicit selection and remembered workflow creation on desktop and phone.
- Existing no-override submission behavior and task-only replacement persistence.

### Out of scope

- Backend APIs, database migrations, execution routing, or override schema changes.
- Loading destination board tasks, all-workspace snapshots, or adding another cache.
- Redesigning the form, workflow picker, or agent selection surface.
- Editing existing task overrides, new preferences, runtime flags, or delegation.

## Technical approach

Implement the selected observer described in
[Selected workflow metadata](../../specs/tasks/system-design/workflow-agent-overrides.md#selected-workflow-metadata).
Use `readJourneyWorkflowSteps` through the existing store-scoped owner.
Keep `workflowStepToState` as the complete mapper and add workflow identity.

Wire the observer through `useTaskCreateDialogData`, setup, validation, and prop
builders. Remove the independent request in `useWorkflowStepsEffect`. Replace
snapshot-reader error and Retry wiring with selected-metadata state and Retry.
Keep other uses of snapshots intact. Do not add a global loading gate.

A complete loaded snapshot can seed current-workspace metadata. Missing or
placeholder snapshots require the lightweight read, even when the route workflow
equals the dialog workflow. An empty response settles successfully. Scope keys
mask obsolete data before effects and guard all late publications. Preserve
`fetchedSteps` only as a current-scope bridge if default-step resolution needs it.

Keep eligibility, grouping, launch precedence, and submission normalization.
Retry preserves the draft and valid selections. Workflow/workspace changes clear
replacements through the existing form lifecycle. Update tests and mocks that
previously encoded snapshot absence as a pending request.

## ASCII UI preview

UI-01: New Task > Advanced settings > Workflow agents, desktop.

```text
Before, unopened workflow B:
Workflow agents                       [Reset disabled]
  Loading workflow steps...           (never settles)

After, metadata success:
Workflow agents                       [Reset]
  Changes apply only to this task.
  Source profile: A  Implement, PR     [Use workflow profile v]

After, request failure:
Workflow agents                       [Reset]
  Unable to load workflow steps.      [Retry]
```

UI-02: Same entry point, phone success and failure.

```text
Advanced settings
Workflow agents               [Reset]
Changes apply only to this task.
Source profile: A
Implement, PR
[ Use workflow profile             v ]

Failure replaces the row:
Unable to load workflow steps.
[ Retry ]
```

The source label, affected steps, and full-width phone selector are structural.
Copy and spacing are illustrative. Reuse existing localized labels and controls.
Loading appears only during an admitted read. Successful no-fixed-profile results
hide the region. Error retains Retry. No separate mobile flow is introduced.

The nearest shipped exemplar is the current mobile workflow override form.
Its full-screen form retains one body scroll owner and a safe-area footer.
The primary action remains task creation. Agent choice is a short temporary
selection in the existing picker. Phone controls retain 44px targets, focus return,
and contained scrolling. Desktop retains its current row composition.
These views cover .1, .7, and .8 through the rendered checks below.

## Tests

All override acceptance IDs in the table below use
`AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001`; the preserved launch-preview criteria
are listed after the table.

| Criteria | Permanent regression evidence to add or update |
| --- | --- |
| .1, .8 | `hooks/use-task-create-workflow-steps.test.ts`: `loads selected workflow metadata without a snapshot or pending board read`; absent, placeholder, and failed inputs; loaded control; empty response; failure and targeted Retry; create-only demand |
| .1, .10 | Mapper and dialog tests retain fixed profiles, initial/step recipients, prompt/events/start/order/color; grouping includes Implement and PR; preview and overrides use the same workflow result |
| .6, .8 | Hook/adapter tests reject delayed workflow A after selecting B, workspace/auth changes, close/reopen and unmount; revisiting a workflow after a snapshot-backed selection or prior failure starts loading and publishes only the new result; retry retains draft/choice; stable rerenders do not repeat demand |
| .8, .10 | Validation tests distinguish idle/loading/success/error, keep ordinary creation available, and block unresolved selected replacements independently of board-reader state |
| .1, .8 | `hooks/journey-metadata-resources.test.ts`: concurrent picker/dialog reads share a flight; releasing one observer leaves the other valid; explicit retry uses the existing refresh path |
| .1, .6, .7, .10 | Desktop/phone rendered integration and browser regressions verify grouped rows, reset/switch, selectors, persisted create payload and consistent launch preview |

Preserve `AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001.2`, `.3`, and `.4` through
the launch-preview and prop-builder suites. Preserve
`AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-002.3` and `.5` through prompt composition
and no-prompt/empty-result checks. These existing requirements constrain the
loader migration. Their [design](../../specs/tasks/system-design/task-create-launch-preview.md)
now delegates selected step transport to the shared dialog observer.

## E2E tests

Extend the existing desktop and phone override specs. Name new cases with
`cold workflow metadata` so their focused runner commands remain stable.
Use `seedWorkflowAgentOverrideFixture`, `useRegularMode`, and existing page objects.
Do not select B as the board filter or navigate to B before opening the dialog.

Each project covers both explicit B selection and remembered B selection while
board A remains selected. The remembered case opens Advanced settings without
opening the workflow picker first. Capture boot state to prove B's snapshot is
absent. During the interaction, record workflow snapshot reads and reject any
new B task-snapshot request. Step reads are permitted and shared when concurrent.
Use request identities, rather than total API counts, for these assertions.

For each entry variant, select profile B, verify Implement/PR grouping and launch
preview, create without launch, and read back persisted step-bound replacements.
Include a selected-step failure followed by Retry and successful recovery with
draft retention. Hook/component tests own the exhaustive late-response matrix.
Phone checks include touch interaction, selector/Retry reachability, 44px targets,
focus return, viewport containment, and no document horizontal overflow.
Existing loaded-board tests remain controls and continue to pass.

## Work orders

- [x] [Task 01: Repair selected workflow metadata](task-01-selected-workflow-metadata.md)

Run Task 01 sequentially. No delegation is authorized.

## Verification results

Planning validation on 2026-11-02:

- Temporary Vitest diagnostic: one file, four current-behavior checks passed.
- `python3 scripts/list-docs.py validate`: 425 decisions and 1,885 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage` local preflight: `covered`, no errors.
  Inputs were this package, its two requirement/design pairs, and a proposed
  validator source edit. This validates planned traceability, not implementation.
- `git diff --check`: passed. New work-order paths were inspected separately.

Implementation validation on 2026-11-02:

- Focused Vitest: 13 files and 191 tests passed. The new hook suite covers absent,
  placeholder, and failed snapshots, complete loaded and successful-empty controls,
  complete profile/recipient mapping, targeted Retry, create-only demand, stable
  rerenders, stale workflow and generation results, activation identity reuse after
  visiting a snapshot-backed workflow, retired snapshot seeds, retired-error
  masking, workspace changes, close/reopen, and unmount. The affected edit-mode
  save/retry suite also passed 11 tests.
- Desktop browser suite: four tests passed, including both `cold workflow metadata`
  entries and the existing loaded-board controls. No workflow B task-snapshot request
  occurred; the explicit and remembered entry variants persisted the step-bound
  replacement.
- Phone browser suite: three tests passed, including both `cold workflow metadata`
  entries. Selector and Retry targets met 44px, Retry recovered the failed draft, and
  no document horizontal overflow occurred.
- `pnpm run typecheck`, focused ESLint, `pnpm run i18n:check`, and `pnpm run i18n:ratchet`
  passed.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.test.py`,
  `python3 scripts/lint-spec-files.py --all`, the traceability preflight (`covered`),
  and `git diff --check` passed.

## Documentation impact

Internal docs only. The public how-to in `docs/public/tasks-and-workflows.md`
already promises task-specific fixed-profile replacement in Advanced settings.
This repair restores that behavior without changing controls, terminology, or API.
The original delivery package retains its historical completed status and results.
This package owns the repaired cold-loading regressions and their evidence.

## Risks

- Fixing only the mapper leaves validation dependent on the absent snapshot.
- Keeping two step loaders can duplicate reads or disagree about launch and grouping.
- A stale bridge can show another workflow's launch preview during a scope switch.
- A browser fixture can hide the bug by opening the picker or warming board B first.
- Broad Retry can load unrelated task data and defeat the bounded loading contract.
