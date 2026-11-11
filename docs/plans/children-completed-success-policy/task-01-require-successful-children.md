---
id: "01-require-successful-children"
title: "Require successful children before parent workflow actions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-SUBTASK-COMPLETION-TRIGGER-003
acceptance_criteria:
  - AC-TASKS-SUBTASK-COMPLETION-TRIGGER-003.1
  - AC-TASKS-SUBTASK-COMPLETION-TRIGGER-003.2
  - AC-TASKS-SUBTASK-COMPLETION-TRIGGER-003.3
  - AC-TASKS-SUBTASK-COMPLETION-TRIGGER-003.4
  - AC-TASKS-SUBTASK-COMPLETION-TRIGGER-003.5
  - AC-TASKS-SUBTASK-COMPLETION-TRIGGER-003.6
  - AC-TASKS-SUBTASK-COMPLETION-TRIGGER-003.7
  - AC-TASKS-SUBTASK-COMPLETION-TRIGGER-003.8
  - AC-TASKS-SUBTASK-COMPLETION-TRIGGER-003.9
  - AC-TASKS-SUBTASK-COMPLETION-TRIGGER-003.10
system_design:
  - ../../specs/tasks/system-design/subtask-completion-success-policy.md
---

# Task 01: Require successful child completion

## Summary

Strengthen existing readiness checks so the workflow trigger requires every
counted child to be `COMPLETED`. Preserve normal successful delivery and later
recovery. Update existing help and prove the behavior without a new setting.

## In scope

- Orchestrator readiness predicate and its terminal-event comment.
- The Office service helper shared by edge and recovery workflow producers.
- Failed, cancelled, mixed, unknown, empty, membership, recovery, and duplicate
  regression cases through real service paths.
- Existing help translations and desktop/phone rendered checks.
- Public workflow guidance and the intentional compatibility change.

## Out of scope

- New configuration fields, database migrations, SDK work, engine interfaces,
  trigger payloads, or new result types.
- Global terminal-state definitions, independent Office scheduler cascades,
  automatic recovery, parent rollback, or branch merging.
- New UI controls, layout changes, and broad local review or verification.

## Acceptance

1. Any counted child outside `COMPLETED` blocks dispatch before callbacks,
   parent movement, operation marks, or recovery receipts. All-success sets
   follow the existing lifecycle, and no extra configuration is required.
2. Membership, read-error, no-session, duplicate, busy-parent, and parent-commit
   behavior remains covered. Failed or cancelled children that later complete
   release one eligible reaction without a new retry mechanism.
3. Both Office workflow producers obey the rule. Existing help and public docs
   explain it, and focused backend plus desktop/phone checks pass.

## TDD sequence

1. Mark the work order `in_progress` and synchronize the plan.
2. Add `TestProcessOnChildrenCompleted_RequiresSuccessfulChildren` in a focused
   sibling test file. Use ordinary workflow configuration and the real handler.
3. Run `mixed_completed_failed` before production changes. Record the parent
   moving when the test expects it to wait as the behavioral RED evidence.
4. Replace `allChildrenTerminal` with `allChildrenCompleted` in the existing
   readiness path. Preserve the nonempty check and single repository snapshot.
5. Require successful wave members in the Office service helper before dispatch.
6. Add recovery and receipt regressions, then update help and rendered tests.
7. Run verification and record results before marking `done`.

Required RED command, from the repository root:

```bash
(cd apps/backend && GOMAXPROCS=2 go test -trimpath -tags fts5 -p 2 ./internal/orchestrator -run '^TestProcessOnChildrenCompleted_RequiresSuccessfulChildren/mixed_completed_failed$' -count=1 -v)
```

## Regression matrix

| Case | Assertion |
| --- | --- |
| `FAILED` + `FAILED` | Parent waits and no completion operation is consumed |
| `COMPLETED` + `FAILED` | Parent waits despite one successful child |
| `COMPLETED` + `CANCELLED` | Parent waits |
| Any active or unknown state, empty set, or failed read | No trigger effects |
| Every counted child `COMPLETED` | Normal parent reaction, entry, and admission |
| Failed archived, ephemeral, or automation-run child | Excluded from the counted snapshot |
| Active grandchild | Does not count as a direct child of this parent |
| Failed/cancelled child later `COMPLETED` | One eligible reaction after the last success |
| Duplicate delivery and reopen/complete cycle | Existing operation idempotency remains effective |
| Move action and side-effect-only declaration | No move, prompt, queued run, or decision mutation before readiness |
| Office edge and recovery paths with cancelled member | No engine dispatch or recovery receipt |
| Office all-success and ordinary no-action handling | Existing acceptance behavior remains |
| Missing parent session | No automatic session creation |

Use repository assertions and callback/dispatch spies. A transition flag alone
does not prove that earlier side effects stayed blocked. Join successful entry
with the existing completion barrier.

Office's successful-delivery fixtures currently use cancelled children.
Change those fixtures to successful children where delivery is the test's
purpose. Preserve their original receipt, identity, ordering, and starvation
assertions. Add explicit cancellation-blocking cases rather than removing
coverage or changing unrelated scheduler semantics.

## ASCII UI preview

UI-01, selected parent step, shared desktop/phone composition:

```text
When Child Tasks Complete                         [?]
[ Move to specific step                           v ]
[ Review                                          v ]

Help: Runs after every active direct child succeeds. At least one counted
      active direct child is required. Archived, ephemeral, and automation-run
      children are ignored; grandchildren do not count. Failed or cancelled
      children keep the parent waiting.
```

This is the [combined preview](plan.md#ascii-ui-preview), mapped to
`AC-TASKS-SUBTASK-COMPLETION-TRIGGER-003.8`. Controls and geometry remain
unchanged. Existing desktop hover/keyboard help and phone touch drawer show the
revised text. Preserve wrapping, scroll ownership, and existing touch targets.

## Rendered and documentation proof

- Extend `workflow-children-completed.spec.ts` with an ordinary parent workflow,
  real waiting session, failed batches, a mixed failed set, and cancellation.
  Later complete the child and assert the persisted and rendered parent move.
- Keep the existing successful workflow scenario. No flag appears in setup,
  saved workflow data, or import/export fixtures.
- In desktop and mobile workflow settings scenarios, open existing help and
  assert the revised meaning. Use `.tap()` for the phone help drawer.
- For negative asynchronous outcomes, use a causal event or the sanctioned
  negative-observation helper. An immediate absence assertion is insufficient.
- Update `childrenCompletedHelp` in all seven real locales. Generate the
  Traditional Chinese pair and pseudo catalog through existing commands.
- Update `docs/public/coordination.md` (how-to),
  `docs/public/workflow-import-export.md` (reference), and
  `docs/public/workflow-tips.md`. Explain the unconditional success rule and
  the effect on workflows that previously accepted failure or cancellation.
  Keep the existing event and portable version.

## Verification

Run from the repository root, sequentially:

```bash
(cd apps/backend && GOMAXPROCS=2 go test -trimpath -tags fts5 -p 2 -race -count=1 ./internal/orchestrator -run 'ChildrenCompleted|TerminalStepMove|AutoStartAgentCallback')
(cd apps/backend && GOMAXPROCS=2 go test -trimpath -tags fts5 -p 2 -race -count=1 ./internal/office/service -run 'Children|ParentWake|WaveIdentity|ResolveWave')
```

Run `bash scripts/lint-go-changed` with the actual changed Go paths. Record its
concrete arguments and result. No engine, storage, SDK, or provider migration
verification is required because those contracts do not change.

Before the first pnpm command in a fresh worktree, run
`(cd apps && pnpm install --frozen-lockfile)`. Then run:

```bash
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/workflow/workflow-children-completed.spec.ts tests/workflow/workflow-settings.spec.ts -- --grep '[Cc]hild')
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/workflow/mobile-workflow-settings.spec.ts -- --grep '[Cc]hild')
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use managed builds, preserve worker limits, and record discovered test counts
for both Playwright projects. Compare rendered help with UI-01. Run the plan's
documentation-coverage preflight before closing the package.

## Files changed

- Orchestrator readiness and regressions:
  `apps/backend/internal/orchestrator/event_handlers_children_completed.go`,
  `event_handlers_children_completed_test.go`,
  `event_handlers_children_completed_success_test.go` (new), and
  `event_handlers_children_completed_wave_test.go`.
- Office wave readiness and regressions:
  `apps/backend/internal/office/service/wave_identity.go`,
  `wave_identity_internal_test.go`,
  `children_completed_wave_identity_test.go`, and
  `scheduler_wake_reconciler_test.go`.
- Existing workflow help in the English, Portuguese, Simplified Chinese,
  Traditional Chinese, Japanese, and Korean catalogs, plus generated
  pseudo-locale catalogs.
- Desktop and mobile workflow E2E scenarios in
  `apps/web/e2e/tests/workflow/`.
- Public guidance in `docs/public/coordination.md`,
  `docs/public/workflow-import-export.md`, and `docs/public/workflow-tips.md`.
- The task requirements and design, this plan and work order, and the existing
  `task-dependencies.md` and `model-unification.md` design descriptions.

## Dependencies

None. This is one vertical fix. Use existing source and tests, `/tdd`, `/e2e`,
`/mobile-parity`, `/docs-maintainer`, and scoped backend/Office/web guidance.
The UI change is content-only inside the existing help surface.

## Risks

- The similarly named Office scheduler helper is outside this change.
- A late gate can allow callbacks before deciding that children are unready.
- Successful-delivery fixtures need real success to retain their original value.
- Existing failure/cancellation-driven workflows intentionally change behavior.

## Parallelism

`sequential`

## Inputs

- [Requirement 003](../../specs/tasks/requirements/subtask-completion-trigger.md).
- [Successful child completion design](../../specs/tasks/system-design/subtask-completion-success-policy.md).
- [Existing parent commit design](../../specs/tasks/system-design/subtask-completion-parent-commit.md).
- [Existing busy parent design](../../specs/tasks/system-design/subtask-completion-busy-parent-wake.md).
- [Investigation evidence](plan.md#evidence-and-root-cause).

## Results

### RED

- The required orchestrator regression failed before the production change:
  `mixed_completed_failed` observed the parent move while a child remained
  failed.
- Office regressions for cancelled wave members failed before the Office gate:
  the resolver accepted the member, edge delivery dispatched, and recovery
  recorded a receipt.
- The managed desktop E2E regression failed before the production change when
  the failed batch advanced the parent.

### GREEN

- Orchestrator race suite passed:
  `GOMAXPROCS=2 go test -trimpath -tags fts5 -p 2 -race -count=1 ./internal/orchestrator -run 'ChildrenCompleted|TerminalStepMove|AutoStartAgentCallback'`.
- Office service race suite passed:
  `GOMAXPROCS=2 go test -trimpath -tags fts5 -p 2 -race -count=1 ./internal/office/service -run 'Children|ParentWake|WaveIdentity|ResolveWave'`.
- `bash scripts/lint-go-changed apps/backend/internal/orchestrator/event_handlers_children_completed.go apps/backend/internal/orchestrator/event_handlers_children_completed_test.go apps/backend/internal/orchestrator/event_handlers_children_completed_success_test.go apps/backend/internal/orchestrator/event_handlers_children_completed_wave_test.go apps/backend/internal/office/service/wave_identity.go apps/backend/internal/office/service/wave_identity_internal_test.go apps/backend/internal/office/service/children_completed_wave_identity_test.go apps/backend/internal/office/service/scheduler_wake_reconciler_test.go` passed with zero issues.
- `i18n:zh-hant`, `i18n:pseudo`, `i18n:check`, and `i18n:ratchet` passed.
- Managed desktop Chromium E2E passed: 2 tests. Managed mobile Chromium E2E
  passed: 1 test. Each used one worker and built production artifacts.
- Public documentation tests passed: 63 tests. The public docs validator
  validated 55 pages.
- Specification catalog validation and full specification lint passed.
- Documentation coverage preflight, whitespace, and local-link checks passed.
- Review follow-up preserved the success and failure help assertions and added
  rendered checks for the nonempty counted-child requirement, ignored child
  types, and grandchildren. Desktop Chromium passed: 1 test. Mobile Chromium
  passed: 1 test. The updated Traditional Chinese and pseudo catalogs were
  generated; localization checks passed.

No commit, pull request, or release publication was performed.
