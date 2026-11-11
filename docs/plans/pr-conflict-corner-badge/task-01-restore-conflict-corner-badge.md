---
id: "01-restore-conflict-corner-badge"
title: "Restore and prove the shared conflict corner badge"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001
  - REQ-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.1
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.2
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.3
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.4
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.7
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.8
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.9
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.10
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.11
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.12
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.13
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.14
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.15
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.16
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.17
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.18
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.3
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.6
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.7
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.8
system_design:
  - ../../specs/integrations/system-design/github-pr-ci-bubble.md
  - ../../specs/integrations/system-design/github-workflow-attention.md
---

# Task 01: Restore and prove the shared conflict corner badge

## Summary

Restore the shared conflict corner badge with conflict priority over CI markers.
Keep CI evidence, automation dots, attributed details, and current desktop/phone interactions intact.

## In scope

- Use `/tdd`, `/e2e`, and `/mobile-parity` in the primary conversation. Mark this work order `in_progress` before implementation.
- Add the glyph regression before production edits and record its expected failure against the current inline layout.
- Replace the trailing conflict span and gap in `PRStatusGlyph` with one relative container and one conditional corner badge.
- Use the existing 10px background wrapper, 8px triangle, 14px/16px base sizes, test identifiers, and automation positions.
- Keep `ciState` unchanged. Hide only its marker during conflicts and restore it immediately after authoritative conflict resolution.
- Update conflicting assertions in the named component and E2E suites. Keep their action, freshness, reload, and attribution scenarios.
- Prove compact/full and aggregate/per-PR behavior, including a conflict on one open PR and CI attention on another.
- Keep both conditions in accessible names and existing disclosures. No new copy or localization key is needed.
- Update the existing public integration explanation and record fresh validation results here and in the plan.
- After successful delivery, promote the amended specs and preserve the completed original plan's historical evidence.

## Out of scope

- Backend/provider logic, data models, polling, freshness rules, merge/repair eligibility, and new API requests.
- GitLab/plugin status presentation, unrelated main-icon colors, and non-draft composer primary icons.
- New drawers, separate badge targets, dependency changes, or publication.

## Acceptance

1. Both glyph sizes show exactly one upper-right conflict badge without a trailing triangle or extra inline width. Automation dots remain visible.
2. Conflicts hide every CI marker while preserving both reasons and PR identities. Resolution restores the existing eligible CI marker without another request.
3. All listed unit, desktop, phone, and documentation checks pass. Geometry, touch input, drawer behavior, route preservation, and focus return match UI-01/UI-02.

## ASCII UI preview

UI-01 excerpt from the [combined preview](plan.md#ascii-ui-preview), shared across desktop and phones:

```text
Conflict plus CI:          Conflict cleared, CI failed:
        (!)                       (x)
       [PR] 2                    [PR] 2

No separate trailing triangle. The automation dots keep their corners.
```

UI-02: desktop hover/focus and phone tap retain both CI and merge-conflict reasons in the existing disclosure.
The whole PR control remains the phone entry point. The existing drawer owns scrolling and safe-area clearance.
Dismissal returns focus to the opener without task navigation. Preserve the effective 44px touch area, including any existing pseudo-element.
Placement and badge priority are structural. Sample spacing is illustrative.
These views map to CI-bubble criteria .9-.12, .15-.16, .18 and workflow-approval criteria 003.3, 003.6, 003.8.

## Verification

Run from the repository root. On a fresh worktree, first run `(cd apps && pnpm install --frozen-lockfile)`.
Do not install again when the workspace dependencies are already present.

RED: add `renders a conflict in the upper-right slot for either glyph size` in `pr-status-glyph.test.tsx`.
Cover `none`, failure, approval, attention, and progress with a conflict, then rerender without the conflict.
Run the first command before production changes and record the expected placement/single-badge failure.

```bash
(cd apps/web && pnpm exec vitest run components/github/pr-status-glyph.test.tsx)
```

GREEN: run the following commands after the correction. Run desktop and phone commands sequentially with fresh managed builds.

```bash
(cd apps/web && pnpm exec vitest run components/github/pr-status-glyph.test.tsx components/github/pr-task-icon.ci-bubble.test.tsx components/github/pr-topbar-button.ci-state.test.tsx components/github/pr-status-chip.ci-state.test.tsx components/github/pr-task-icon.workflow-approval.test.tsx components/github/pr-task-icon-conflicts.test.ts components/github/pr-task-icon.automation-render.test.tsx components/github/pr-task-icon.negative-projection.test.tsx components/github/pr-ci-state.test.ts components/github/pr-status-refresh-routes.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && CAPTURE_PR_ASSETS=1 pnpm e2e:run --host --shards 1 --project chromium tests/pr/pr-ci-bubble.spec.ts tests/pr/pr-status-badge.spec.ts -- --retries=0)
(cd apps/web && CAPTURE_PR_ASSETS=1 pnpm e2e:run --host --shards 1 --project mobile-chrome tests/pr/mobile-pr-ci-bubble.spec.ts tests/pr/mobile-pr-sidebar-automation-indicators.spec.ts -- --retries=0)
(cd apps/web && CAPTURE_PR_ASSETS=1 pnpm e2e:run --host --shards 1 --project chromium tests/pr/pr-multi-popover.spec.ts tests/pr/pr-topbar-popover.spec.ts -- --retries=0)
(cd apps/web && CAPTURE_PR_ASSETS=1 pnpm e2e:run --host --shards 1 --project chromium tests/pr/pr-ci-bubble.spec.ts -- --retries=0 --grep "prioritizes conflict in the badge")
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Confirm that each project discovers its intended cases. Do not use `--no-build` after source changes.
Use persisted provider-fixture convergence and causal waits. Restore shared fixture state in cleanup.
Update the existing coexistence assertions in both phone files, including their screenshot captions.
The plan's E2E matrix defines required viewport, geometry, attribution, and interaction evidence.
Inspect captured images against UI-01/UI-02. Record actual test counts, geometry, and any unresolved preview difference.
Run any additional suite changed during implementation with its owning project before completion.

Run the documentation coverage preflight after artifact edits:

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const files = [
  'docs/plans/pr-conflict-corner-badge/plan.md',
  'docs/plans/pr-conflict-corner-badge/task-01-restore-conflict-corner-badge.md',
  'docs/specs/integrations/requirements/github-pr-ci-bubble.md',
  'docs/specs/integrations/system-design/github-pr-ci-bubble.md',
  'docs/specs/integrations/requirements/github-workflow-attention.md',
  'docs/specs/integrations/system-design/github-workflow-attention.md',
];
const result = validateCoverage({
  changedFiles: [
    { filename: 'apps/web/components/github/pr-status-glyph.tsx', status: 'modified' },
    ...files.map(filename => ({ filename, status: 'modified' })),
  ],
  fileContents: Object.fromEntries(files.map(file => [file, fs.readFileSync(file, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.workOrders.length !== 1) process.exitCode = 1;
NODE
```

## Files likely touched

- `apps/web/components/github/pr-status-glyph.tsx`
- The five changed component/unit test suites
- The four planned desktop/phone E2E suites, plus the aggregate and topbar suites
- `apps/web/e2e/helpers/pr-badge-geometry.ts` for shared browser geometry checks
- `docs/public/integrations.md` (existing status explanation)
- The paired CI-bubble and workflow-attention specifications linked in frontmatter
- This work order and `docs/plans/pr-conflict-corner-badge/plan.md`

The shared consumers are inspection targets, not an automatic rewrite scope.
They include `pr-task-icon-disclosure.tsx`, `pr-topbar-button.tsx`, `pr-status-chip.tsx`, `task-contribution-icons.tsx`, and `task-subtask-row.tsx`.

## Dependencies

None. The existing CI-bubble projection and provider fixtures are already implemented.

## Risks

- Absolute badges can clip within a parent or overlap automation dots. Measure the rendered wrapper and inspect captures.
- The visible glyph becomes narrower. Preserve the existing effective touch area and verify an edge tap.
- Suppressing CI upstream can discard accessibility or detail information. Limit suppression to shared badge rendering.

## Parallelism

`sequential`

## Inputs

- [Plan and evidence](plan.md)
- [CI-bubble requirement](../../specs/integrations/requirements/github-pr-ci-bubble.md), especially criteria .9, .10, and .18
- [Shared presentation design](../../specs/integrations/system-design/github-pr-ci-bubble.md#shared-presentation)
- [Approval requirement](../../specs/integrations/requirements/github-workflow-attention.md#req-integrations-github-workflow-attention-003-task-approval-badge)
- [Approval design](../../specs/integrations/system-design/github-workflow-attention.md#task-approval-badge)
- Existing `PRTaskIconDrawer`, `PRStatusChipDrawer`, provider-backed E2E fixtures, and `prCapture`

## Results

Completed on 2026-11-11.

- The new glyph regression failed against inline placement before the production change. The targeted unit command then passed 80 tests across 10 files.
- `pnpm run typecheck` and `pnpm run lint` passed from `apps/web`.
- Managed desktop build passed and its PR CI/status badge suites passed 16/16. Managed phone build passed and its PR CI/sidebar suites passed 5/5.
- The additional multi-PR and topbar suites passed 17/17. The focused desktop capture case passed 1/1.
- Geometry checks confirmed 14px task glyphs, 16px topbar glyphs, 10px conflict wrappers, overlap at the upper-right PR icon corner, and no increase to inline glyph width.
- The existing phone pseudo-element measured 44px by 44px. An edge tap opened the current PR drawer; dismissal restored focus and kept the current route.
- A follow-up review found that host button and menu rules enlarged the nested triangle. The explicit `size-2` class keeps it at 8px; the topbar and multi-PR menu cases passed with this measurement enabled (2/2).
- Inspected the desktop `desktop-ci-conflict-automation` capture, phone `mobile-ci-bubble-before-tap` and `sidebar-workflow-conflict-and-approval-badge-mobile` captures, and the conflict details drawer capture. They match UI-01/UI-02: one conflict badge owns the corner, automation markers remain visible, and the current disclosure retains both reasons. No preview mismatch remains.
- The public-doc tests and validator, documentation catalog validation, spec-linter tests and full lint, coverage preflight, and `git diff --check` passed.
- The managed Vite production builds completed. They emitted existing chunk-size and dynamic-import warnings. No build errors occurred.

The amended requirements are active, the designs are current, and the plan is implemented. No provider behavior, classification, API requests, dependencies, or localized copy changed.
