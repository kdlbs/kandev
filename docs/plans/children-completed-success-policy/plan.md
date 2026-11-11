---
created: 2026-11-11
status: implemented
requirements:
  - REQ-TASKS-SUBTASK-COMPLETION-TRIGGER-003
system_design:
  - ../../specs/tasks/system-design/subtask-completion-success-policy.md
legacy_specs: []
---

# Implementation plan: Require successful children

## Overview

Use one rule: `on_children_completed` fires only when every counted direct
child is `COMPLETED`. The user explicitly requested no additional setting.
This changes existing workflows automatically.

The [task requirement](../../specs/tasks/requirements/subtask-completion-trigger.md)
and [design](../../specs/tasks/system-design/subtask-completion-success-policy.md)
own this behavior. One vertical work order covers readiness, regression
proof, existing help, and public guidance.

## Evidence and root cause

Investigation used checkout `bceeaf1c1d30ad05b953fd800b56ae46ebf5f2bf` and
[issue #5258](https://github.com/kdlbs/kandev/issues/5258). The handler's
`allChildrenTerminal` check accepts failure and cancellation. It therefore
emits the parent trigger even when all child work failed.

A temporary SQLite-backed test drove the real handler and persisted parent
move. `FAILED/FAILED`, `COMPLETED/FAILED`, and `COMPLETED/CANCELLED` each moved
the parent from waiting to review. All three reproduction cases passed.
The test joined destination entry and was removed afterward. Its minimal
fixture also logged an independent review-state reconciliation error.

The reproduction command, from `apps/backend`, was:

```bash
GOMAXPROCS=2 go test -trimpath -tags fts5 -p 2 ./internal/orchestrator -run '^TestIssue5258Repro_ParentAdvancesForFailedChildren$' -count=1 -v
```

This proved the downstream readiness problem without reproducing an OOM.
Issue #5258 remains assigned to `carlosflorencio`. Session-recovery issue #5256
is separate.

## Scope

### In scope

- Replace terminal readiness with successful readiness before trigger dispatch.
- Apply the same rule to both Office engine-routed producers through their
  existing shared helper.
- Preserve unconsumed operations while children are failed or cancelled.
- Update existing translated help and public guidance.
- Prove failed batches, mixed states, cancellation, later success, and duplicates.

### Out of scope

- New settings, storage columns, migrations, API fields, SDK changes, or
  engine result types.
- Crash classification, automatic recovery, branch merging, or parent rollback.
- Global terminal-state changes and independent Office scheduler cascades.
- Commits, pull requests, or release publication.

## Technical approach

`readyChildCompletionRows` keeps its one repository read and nonempty check.
Rename the private predicate to `allChildrenCompleted` and require
`TaskStateCompleted` on every row. This check precedes engine actions and
operation marks, so blocked reactions need no new engine result.

In `office/service/wave_identity.go`, the helper shared by edge and recovery
workflow producers must also require `COMPLETED` on every wave member. Its
existing `ok=false` path skips dispatch and recovery receipts. Keep signatures,
payloads, membership filters, wave identity, and operation keys unchanged.

The similarly named scheduler helper belongs to independent Office wakes and
keeps its current contract. Clarify the Office requirement's terminology so
identity and workflow eligibility remain separate concepts.

Successful retry, duplicate suppression, parent commit, and busy-parent prompt
delivery retain their current paths. A child becoming `COMPLETED` supplies the
next ordinary evaluation opportunity.

## ASCII UI preview

UI-01: Settings, Workflows, selected parent step. Existing controls stay in place
on desktop and phone. Only help copy changes.

```text
When Child Tasks Complete                         [?]
[ Move to specific step                           v ]
[ Review                                          v ]

Help: Runs after every active direct child succeeds. At least one counted
      active direct child is required. Archived, ephemeral, and automation-run
      children are ignored; grandchildren do not count. Failed or cancelled
      children keep the parent waiting.
```

The same copy appears in desktop hover/keyboard help and the existing phone
help drawer. Text wraps in the current surface. No new control, geometry,
scroll owner, navigation, or save action is introduced.

This maps to `AC-TASKS-SUBTASK-COMPLETION-TRIGGER-003.8`. The shipped exemplar is
`EventTransitionSelect` and its touch-aware `HelpTip`.

## Tests

| Criteria under requirement 003 | Evidence |
| --- | --- |
| `.1`, `.2`, `.3`, `.6` | Real-handler regression with failed, cancelled, mixed, and successful children |
| `.4`, `.9` | Membership exclusions, empty sets, unknown states, and read-error cases |
| `.5`, `.10` | Later successful completion, duplicates, no-session behavior, and existing ledger regressions |
| `.7` | Ordinary existing workflow configuration, without any additional field |
| `.8` | Existing help component and desktop/phone rendered help checks |

The required RED test is
`TestProcessOnChildrenCompleted_RequiresSuccessfulChildren/mixed_completed_failed`.
It must fail on the current code because the parent moves instead of waiting.
The work order contains exact commands and affected test files.

## E2E tests

Extend `tests/workflow/workflow-children-completed.spec.ts` in `chromium` to
keep an ordinary parent waiting after child failure or cancellation. Complete
the child later and observe the normal parent move. No new configuration is
part of the setup.

Extend the existing child-completion scenario in
`tests/workflow/mobile-workflow-settings.spec.ts` in `mobile-chrome` to verify
revised touch help. Keep the existing desktop settings coverage and add the
same help expectation there. Preserve successful workflow tests.

Use isolated fixtures, a real parent session, and causal observations or the
sanctioned negative-observation helper. Immediate absence assertions cannot
prove that asynchronous parent dispatch stayed blocked.

## Compatibility

Every workflow uses the stricter rule. Existing reactions that relied on failed
or cancelled children now wait. `docs/public/coordination.md`,
`docs/public/workflow-import-export.md`, and `docs/public/workflow-tips.md`
explain the changed behavior.

Global task terminal states and consumed operations remain unchanged. Existing
parents already advanced are not rewound. `COMPLETED` remains a persisted task
outcome, not a Git merge guarantee.

Earlier completed trigger, parent-commit, and busy-parent packages remain
historical implementation records. Their validation results are not evidence
for this changed readiness rule.

## Work orders

- [x] [Task 01: Require successful children and prove the trigger](task-01-require-successful-children.md)

## Verification results

Implementation and package checks passed on 2026-11-11:

- Specification catalog validation and full specification lint.
- All 36 specification-linter tests.
- Documentation coverage for the single work order, with no errors.
- Backend orchestrator and Office service race suites for the focused regressions.
- Changed-Go lint with all eight changed Go paths, zero issues.
- i18n Traditional Chinese generation, pseudo generation, catalog check, and
  ratchet.
- Desktop Chromium E2E: 2 passed. Mobile Chromium E2E: 1 passed. Both used the
  managed one-worker runner, and both included production builds.
- Public documentation tests: 63 passed; 55 published pages validated.
- Tracked and new-file whitespace and local links.
- Review follow-up: desktop and mobile rendered-help checks each passed after
  asserting the nonempty counted-child requirement and membership exclusions.

The required orchestrator RED case failed before the readiness change because
the parent moved while a child remained failed. Three Office RED cases also
failed before the Office gate because a cancelled member dispatched. A managed
desktop E2E RED run showed the failed child batch moving its parent. Their
corresponding final green runs are recorded in the work order.

## Documentation coverage preflight

Run from the repository root:

```bash
node <<'JS'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/specs/tasks/requirements/subtask-completion-trigger.md',
  'docs/specs/tasks/system-design/subtask-completion-success-policy.md',
  'docs/plans/children-completed-success-policy/plan.md',
  'docs/plans/children-completed-success-policy/task-01-require-successful-children.md',
];
const result = validateCoverage({
  changedFiles: [
    'apps/backend/internal/orchestrator/event_handlers_children_completed.go',
    'apps/backend/internal/office/service/wave_identity.go',
    'apps/web/src/locales/en/workflows.json',
    ...paths,
  ],
  fileContents: Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
JS
```

## Risks

- Cancellation no longer releases this workflow trigger. This is an intentional
  compatibility change, not an opt-in mode.
- Office successful-delivery fixtures currently contain cancelled children.
  Update those fixtures and preserve separate cancellation-blocking assertions.
- A readiness check after engine dispatch can already be too late for side effects.
- A stale production build or premature negative assertion can hide the regression.

## Design handoff

The implementation and permanent regressions are complete. The package remains
unstaged and uncommitted; no commit, pull request, or release publication was
requested.
