---
id: "01-preserve-new-edit"
title: "Preserve new mobile Rename edits through old completion"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-EDIT-FOCUS-001
acceptance_criteria:
  - AC-UI-EDIT-FOCUS-001.4
  - AC-UI-EDIT-FOCUS-001.5
  - AC-UI-EDIT-FOCUS-001.7
system_design:
  - ../../specs/ui/system-design/inline-edit-focus-continuity.md
---

# Task 01: Preserve new edit

## Summary

An old Rename request may clear only its own edit. Keep captured routing,
normalization, immediate close, error feedback and subsequent explicit controls.

## In scope

- Completion ownership and real-provider/action/API/transport regression RED/GREEN.
- Shipped picker/dialog integration, phone touch and wider viewport compatibility.
- Owning requirement/design minimal extension and verification receipts.

## Out of scope

Backend/API/schema, queues, cancellation, cache/WS changes, persistent drafts,
menu/modal/navigation redesign, settings/sibling changes, push/PR before grant.

## Acceptance

1. Success and failure cannot clear a newer edit of either task, including same
   task reentry and out-of-order completions; current-owner settlement still clears.
2. Captured task/title, immediate close and legitimate failure toast are retained;
   the new draft can be saved or cancelled explicitly.
3. Focus/title normalization remains covered; source/seed and operator state stay
   untouched; all owned handles terminal before clean commit and parent handoff.

## ASCII UI preview

UI-01 from [plan](plan.md#ascii-ui-preview), AC-UI-EDIT-FOCUS-001.7:

```text
A Save -> close (PATCH pending)
Open A/B -> [Rename task] [ exact new draft ] [Cancel] [Save]
Old completion -> same dialog and draft remain
```

Existing picker/dialog composition, touch targets and scroll owner are retained.
Desktop/tablet paths retain current handlers; no geometry change.

## Verification

Run from the repository root with Node 24 on PATH and NODE_OPTIONS=--max-old-space-size=8192.
Use GOMAXPROCS=2 GOFLAGS=-p=2 for builds. Frozen install from `apps/` precedes commands.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/mobile/use-mobile-task-rename-ownership.test.tsx components/task/mobile/use-mobile-task-rename.test.ts components/task/task-rename-dialog.test.tsx)
(cd apps && GOMAXPROCS=2 GOFLAGS=-p=2 ../scripts/run-quiet e2e --summary -- pnpm --dir web e2e:run --host --project mobile-chrome e2e/tests/task/mobile-task-rename-ownership.spec.ts --retries=0)
(cd apps && GOMAXPROCS=2 GOFLAGS=-p=2 ../scripts/run-quiet e2e --summary -- pnpm --dir web e2e:run --host --no-build --project chromium e2e/tests/task/inline-edit-focus.spec.ts --retries=0)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/mobile/use-mobile-task-rename.ts components/task/mobile/use-mobile-task-rename-ownership.test.tsx e2e/tests/task/mobile-task-rename-ownership.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
python3 scripts/lint-spec-files.test.py
python3 scripts/list-docs.test.py
git diff --check edafc7900de0328a9997401f11a0d76cb8c548d3
```

Run `.github/scripts/pr-docs.cjs` validateCoverage against full base diff including
untracked owned docs, and normal precommit/commit-msg hooks. Refresh main, open
inventory and paginated target patches before delivery. Record actual outcomes
and any bounded browser exception rather than claim a skipped test passed.

## Files likely touched

- `apps/web/components/task/mobile/use-mobile-task-rename.ts`
- `apps/web/components/task/mobile/use-mobile-task-rename-ownership.test.tsx`
- `apps/web/e2e/tests/task/mobile-task-rename-ownership.spec.ts`
- `docs/specs/ui/requirements/inline-edit-focus-continuity.md`
- `docs/specs/ui/system-design/inline-edit-focus-continuity.md`
- This plan/work order.

## Dependencies

None. Existing parent causal proof is preserved outside the checkout.

## Risks

Task-ID comparison misses same-task reentry. Gating all feedback hides old errors.
Only this task's fixture/processes may be changed; keep every handle terminal.

## Parallelism

`sequential`; no agents or nested platform tasks/sessions authorized.

## Inputs

Parent attempt21-source receipts; owning requirement/design; actual Sheet,
TaskSwitcherDialogs, TaskRenameDialog, action/API/fetchJson.

## Results

- Frozen `pnpm install --frozen-lockfile` from `apps/`: passed.
- Causal component RED: 6 intended ownership failures, 2 ordinary controls passed;
  real providers/dialog/action/API, only fetch held. Final focused run: 11 passed
  in 3 files, including current-owner controls and retained caret/normalization.
- Actual mounted picker/browser RED: 2 intended missing-newer-draft failures
  after old response, including corrected explicit agent/worktree fixture.
  An earlier fixture run had unrelated missing-agent session notices; the
  corrected run reproduced the same ownership failures without those notices.
- Fresh production build + phone touch GREEN: 2 passed. Final browser run:
  3 passed (20.3s), covering different/same task and same-task edit after a
  900px tablet resize. Explicit Save/Cancel and persisted titles checked.
  Seed repository HEAD/status remained unchanged. The earlier fresh-build
  two-scenario run also verified all changed/untracked source bytes unchanged.
- Existing desktop top-bar, Office and sidebar dialog focus/edit compatibility:
  3 passed (9.6s), using the same fresh build. These are ordinary edit controls,
  not a claim to repair the separate desktop sidebar completion handler.
- Scoped ESLint: passed with zero warnings. Initial duplicate test-string
  warnings were corrected. Node 24 typecheck passed after removing unsupported
  Testing Library `exact` options from the new component test. i18n check passed.
- Documentation catalog validation, full specification lint, 36 spec-linter
  tests and 9 catalog tests: passed. Full-base documentation coverage preflight:
  covered by this one work order and the existing requirement/design pair.
- Public docs assessment: existing task Rename entry/instructions are unchanged;
  this restores an edit-lifetime guarantee and introduces no new workflow, copy,
  API, configuration or screenshot contract. Internal specs/plans updated only.
- Screenshot inspection confirms the retained newer draft and unchanged controls
  on phone and tablet. Browser device emulation and mocked agent/disposable real
  backend establish shipped transport/UI integration, not a physical phone,
  native OS keyboard, or operator-runtime incident claim.
- Normal active precommit/commit-msg hooks and clean committed HEAD are mandatory
  for the parent checkpoint. Exact SHA, hook results, terminal process ledger,
  full-base diff and logs are recorded outside checkout in the readiness receipt.
  No push or PR is authorized at this checkpoint.
