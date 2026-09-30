---
id: "02-browser-coverage-and-docs"
title: "Prove desktop and phone approval visibility"
status: done
wave: 2
depends_on:
  - "01-project-and-render-approval-badge"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.1
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.2
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.3
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.4
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.5
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.6
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.7
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.8
system_design:
  - ../../specs/integrations/system-design/github-workflow-attention.md
---

# Task 02: Prove desktop and phone approval visibility

## Summary

Prove that maintainers see the badge before disclosure and reach the explanation through desktop and phone controls.
Use existing provider fixtures and document the badge in the public GitHub status explanation.

## In scope

- Extend the plan's desktop and phone scenario matrix using mock workflow feedback, not browser-state injection.
- Assert compact status through API convergence, then rendered badge visibility before hover/tap.
- Cover conflict priority, automation dots, multiple PRs, reload, authoritative clearing, and existing mobile drawer navigation/focus.
- Capture normal-density desktop light/dark and phone states; verify containment, overflow, and narrow fine-pointer behavior.
- Add a short explanation to `docs/public/integrations.md` through `/docs-maintainer` and pragmatic `/simple-english`.
- Record exact checks and captures. Mark the plan implemented only after both work orders pass and the contracts match.
- Assess shared specification lifecycle against every requirement in those files. Keep draft status while another linked capability remains unresolved.

## Out of scope

- Production eligibility or glyph redesign beyond a defect necessary to satisfy the reviewed package.
- Live workflow approval, new media seeding systems, and publication actions.

## Acceptance

1. Provider-backed desktop and phone E2E prove badge visibility before disclosure, correct explanations, and reconciliation after reload and clearing.
2. Rendered checks prove conflict priority, existing automation dots, light/dark colors, phone containment, and drawer dismissal without task navigation.
3. Public documentation explains the amber lock and conflict priority; all exact commands and artifact paths appear in Results.

## ASCII UI preview

UI-01 through UI-03 excerpts from the [combined previews](plan.md#ascii-ui-preview):

```text
Desktop row: Review Contributor PR #4082 [PR+lock]
Hover/focus: CI  Awaiting maintainer approval

Phone Tasks: Review Contributor PR #4082 [PR+lock]
Tap PR -> existing drawer
  Pull request #4082
  CI     Awaiting maintainer approval
  Merge  Conflicts                    (if present)
Dismiss -> focus returns; task route stays unchanged
```

The lock uses amber in the upper-right slot; conflicts replace it with a red triangle.
Both conditions remain in text. Verify all 003 criteria with the scenario matrix and unit evidence from Task 01.
Use the shipped `PRTaskIconDrawer` exemplar; do not introduce another disclosure surface.

## Verification

Run from the repository root. Task 01 completes the workspace dependency installation.
Run managed E2E commands sequentially; each builds fresh production assets.

```bash
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/pr/pr-status-badge.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/pr/mobile-pr-sidebar-automation-indicators.spec.ts tests/pr/mobile-pr-ci-chip.spec.ts -- --retries=0)
(cd apps/web && pnpm exec vitest run components/github/pr-task-icon.workflow-approval.test.tsx)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Existing component RED from Task 01 proves the eligibility gap; browser tests prove integrated behavior after that fix.
Use `prCapture` for screenshots within the same focused E2E runs.
Record light/dark computed colors and narrow fine-pointer containment in the desktop scenario.
Record the phone drawer, text, focus, and overflow results in the mobile scenario.
Run additional changed suites by exact path. Restore fixture settings in cleanup even when assertions fail.

## Files likely touched

- `apps/web/e2e/tests/pr/pr-status-badge.spec.ts`
- `apps/web/e2e/tests/pr/mobile-pr-sidebar-automation-indicators.spec.ts`
- `apps/web/e2e/tests/pr/mobile-pr-ci-chip.spec.ts` only when an added assertion is necessary
- Existing PR E2E helpers when shared provider setup needs extraction
- `docs/public/integrations.md`
- This plan and current work order, plus linked specification frontmatter after completed delivery

## Dependencies

Task 01 supplies the compact projection and rendered badge.

## Risks

Opening an active task can hide compact-data gaps; seed a separate target task in the phone picker.
Aggregate-only fixtures can lose approval after refresh; seed workflow evidence and wait for stored convergence.
Provider refresh retains existing cache latency. Use explicit refresh for authoritative clearing rather than arbitrary sleeps.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/github-workflow-attention.md)
- [Badge design](../../specs/integrations/system-design/github-workflow-attention.md#task-approval-badge)
- Plan E2E matrix and existing mobile automation/approval scenarios
- `/e2e`, `/mobile-parity`, `/docs-maintainer`, and `/simple-english`

## Results

Implemented provider-backed desktop and phone coverage, and documented the amber lock and conflict priority in `docs/public/integrations.md`. The phone picker fixture starts with a same-head fork approval and no conflict, alongside one ready sibling. It then changes the provider PR to dirty on that same head and verifies conflict priority while retaining both explanations.

- Desktop E2E: `(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/pr/pr-status-badge.spec.ts -- --retries=0)`; 11 tests passed.
- Phone E2E: `(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/pr/mobile-pr-sidebar-automation-indicators.spec.ts tests/pr/mobile-pr-ci-chip.spec.ts -- --retries=0)`; 9 tests passed.
- Focused phone capture E2E: `(cd apps/web && CAPTURE_PR_ASSETS=1 pnpm e2e:run --host --shards 1 --project mobile-chrome tests/pr/mobile-pr-sidebar-automation-indicators.spec.ts -- --retries=0 --grep "shows touch indicators")`; 1 test passed and the approval-only phone capture was visually inspected with the amber lock visible.
- Focused component regression suite: `(cd apps/web && pnpm exec vitest run components/github/pr-task-icon.workflow-approval.test.tsx --reporter=dot)`; 7 tests passed.
- Component stale-evidence regression: `(cd apps/web && pnpm exec vitest run components/github/pr-task-icon.render.test.tsx components/github/pr-task-icon.workflow-approval.test.tsx components/github/pr-task-status-summary.test.ts components/github/pr-workflow-attention-summary.test.ts --reporter=dot)`; 49 tests passed.
- `pnpm run typecheck`: passed.
- `pnpm run build:vite`: passed.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet`: passed.
- `node --test scripts/validate-public-docs.test.mjs`: 62 tests passed; `node scripts/validate-public-docs.mjs`: 47 published pages validated.
- `python3 scripts/list-docs.py validate`: 337 decisions and 1,271 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed; `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- Focused capture E2E runs passed. Captures are in the ignored `apps/web/.pr-assets/` directory, indexed by `manifest.json`:
  - `pr-status-badge--desktop-workflow-approval-badge-light.png`
  - `pr-status-badge--desktop-workflow-approval-badge-dark.png`
  - `pr-status-badge--desktop-workflow-approval-attention.png`
  - `mobile-pr-sidebar-automation-indicators--sidebar-workflow-approval-only-badge-mobile.png`
  - `mobile-pr-sidebar-automation-indicators--sidebar-workflow-approval-only-details-mobile.png`
  - `mobile-pr-sidebar-automation-indicators--sidebar-workflow-conflict-priority-badge-mobile.png`
  - `mobile-pr-sidebar-automation-indicators--sidebar-workflow-conflict-priority-details-mobile.png`

The desktop run verified the badge before hover, both computed amber colors, no narrow fine-pointer overflow, Kanban visibility, and clearing after refresh. The phone run verified the approval-only lock before tap, its containment within the task row, automation dots, two-PR attribution, drawer overflow, focus restoration, and unchanged task URL after dismissal. It then updated the provider-backed PR to a conflict on the same head and verified the red warning replaced the lock while the drawer retained both approval and conflict reasons. The approval-only capture shows the amber padlock; the conflict capture shows the red conflict triangle.
