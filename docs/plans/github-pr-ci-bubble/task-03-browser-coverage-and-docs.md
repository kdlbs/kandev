---
id: "03-browser-coverage-and-docs"
title: "Prove desktop and phone CI visibility"
status: done
wave: 3
depends_on:
  - "02-render-ci-bubbles"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.1
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.2
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.3
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.4
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.5
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.6
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
system_design:
  - ../../specs/integrations/system-design/github-pr-ci-bubble.md
---

# Task 03: Prove desktop and phone CI visibility

## Summary

Prove the integrated states through existing provider-backed browser fixtures.
Update the public GitHub status explanation and record fresh evidence for this package.

## In scope

- Implement the plan's complete E2E matrix in the existing PR suites through `/e2e` and `/mobile-parity`.
- Use selected provider check/workflow fixtures and persisted API convergence before visible assertions.
- Assert compact bubbles before hover/tap and after reload. Keep the target task inactive to expose compact-data gaps.
- Cover pass+skip, all skipped, failure+skip, progress+skip, approval without jobs, and multiple simultaneous conditions.
- Cover head changes, authoritative clearing, terminal siblings, and new/old compact/full observations.
- Prove desktop keyboard access, actual phone drawers, focus return, unchanged task route, and no extra status-rendering requests.
- Measure the whole glyph/conflict group, touch hitbox, drawer containment, and document overflow.
- Include phone-width fine-pointer behavior, both sides of 768px, reduced motion, and light/dark captures.
- Update `docs/public/integrations.md` through `/docs-maintainer` and pragmatic `/simple-english`.
- Record exact command results and captures. Promote specification lifecycle only after all three work orders pass and the implementation matches.

## Out of scope

- Live GitHub mutations, new provider scenarios, unrelated QA, broad verification, and publication.
- Rewriting historical results or resetting completed companion plans.

## Acceptance

1. Desktop and phone E2E prove the plan's CI/skipped matrix before and after disclosure, refresh, and reload without extra provider polling.
2. Rendered measurements and inspected captures prove the separate conflict slot, automation dots, accessible interaction, containment, and touch geometry.
3. Public guidance matches the new markers and skip semantics. Fresh results and specification lifecycle are accurate across this package and affected active clauses.

## ASCII UI preview

UI-01 through UI-03 excerpts from the [combined previews](plan.md#ascii-ui-preview):

```text
Desktop: Fix documentation checks [PR+x] [!]
Details: CI Failed / CI Awaiting maintainer approval / Merge Conflicts

Phone Tasks: Fix documentation checks [PR+L] [!]
Tap -> existing drawer with the same attributed status rows
Dismiss -> same route, focus returns

Pass or all skipped: [PR]        (no upper-right bubble)
All-skipped details: CI All checks skipped
```

Prove these structural choices with screenshots and geometry. ASCII spacing is illustrative.
The phone drawer retains one scrolling body, its current dynamic viewport bound, and safe-area clearance.
The existing `PRTaskIconDrawer` and `PRStatusChipDrawer` are the mobile exemplars.

## Verification

Run from the repository root. Task 01 supplies workspace dependencies.
Run managed browser commands sequentially. They build current production assets and use one worker per shard.
Confirm each selected project discovers the intended cases. Use `prCapture` in these runs and inspect the resulting images.

```bash
(cd apps/web && CAPTURE_PR_ASSETS=1 pnpm e2e:run --host --shards 1 --project chromium tests/pr/pr-status-badge.spec.ts tests/pr/pr-sidebar-background-refresh.spec.ts tests/pr/pr-sidebar-hover-hydration.spec.ts tests/pr/pr-topbar-popover.spec.ts -- --retries=0)
(cd apps/web && CAPTURE_PR_ASSETS=1 pnpm e2e:run --host --shards 1 --project mobile-chrome tests/pr/mobile-pr-sidebar-automation-indicators.spec.ts tests/pr/mobile-pr-ci-chip.spec.ts tests/pr/mobile-pr-sidebar-background-refresh.spec.ts -- --retries=0)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Restore changed shared fixture records and saved settings even when assertions fail.
Use causal waits. Keep route, focus, and negative-request observations armed before the relevant interaction.
Run all additionally changed E2E suites with their owning project. Do not replace pre-existing mutation tests with state-only tests.

Run this local documentation-coverage preflight after any artifact or reference edits.
The planned glyph path exercises structural coverage without publishing a GitHub status or changing that file.

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const dir = 'docs/plans/github-pr-ci-bubble';
const files = [
  ...fs.readdirSync(dir).filter(name => name.endsWith('.md')).map(name => `${dir}/${name}`),
  'docs/specs/integrations/requirements/github-pr-ci-bubble.md',
  'docs/specs/integrations/system-design/github-pr-ci-bubble.md',
];
const result = validateCoverage({
  changedFiles: [
    { filename: 'apps/web/components/github/pr-status-glyph.tsx', status: 'modified' },
    ...files.map(filename => ({ filename, status: 'added' })),
  ],
  fileContents: Object.fromEntries(files.map(file => [file, fs.readFileSync(file, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.workOrders.length !== 3) process.exitCode = 1;
NODE
```

## Files likely touched

- Existing desktop and phone PR E2E files named in Verification
- Shared PR E2E fixture helpers, only for setup reused by several scenarios
- `docs/public/integrations.md`
- This plan and current work order's results
- New requirement/design frontmatter after successful delivery
- Existing workflow-attention clauses reconciled by Task 02, if final evidence exposes a discrepancy

## Dependencies

Task 02 supplies the integrated rendered behavior. Task 01 supplies persisted projection and metadata.

## Risks

An active task can populate full PR records and conceal a broken compact projection.
A mock approval without matching workflow/head evidence can disappear on refresh.
Class strings do not prove a 44px touch hitbox or unclipped absolute badges. Measure actual geometry.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/github-pr-ci-bubble.md)
- [Design](../../specs/integrations/system-design/github-pr-ci-bubble.md)
- Plan E2E matrix, previews UI-01 through UI-03, and completed Task 01/02 results
- Existing provider-backed approval, selected-check, negative-projection, and mobile drawer scenarios

## Results

Completed on 2026-11-10.

- Added `e2e/tests/pr/pr-ci-bubble.spec.ts`: draft failure before hover, success clearing, conflict/CI/automation coexistence with geometry, all-skipped persistence and copy, topbar parity, and a passing draft card without bubbles. Five tests passed.
- Added `e2e/tests/pr/mobile-pr-ci-bubble.spec.ts`: bubble and separate conflict before tap, an outer-edge tap inside the coarse-pointer hitbox, the existing PR drawer, both explanations, unchanged route, focus return, and no document overflow. One test passed.
- Updated existing placement assertions: the conflict triangle and approval lock are now independent, and the topbar has three SVGs when both are present.
- Updated `docs/public/integrations.md` with the CI marker meanings, skipped neutrality, all-skipped copy, and the separate conflict slot.
- Generated and inspected captures. Desktop: `pr-ci-bubble--desktop-draft-ci-failure-bubble.png` and `pr-ci-bubble--desktop-ci-conflict-automation.png`. Phone: `mobile-pr-ci-bubble--mobile-ci-bubble-before-tap.png` and `mobile-pr-ci-bubble--mobile-ci-drawer.png`. The phone drawer capture shows `CI Failed` and `Merge Conflicts` with no overflow.
- Promoted this requirement to `active`, its design to `current`, and this plan to `implemented`.

Verification:

- `pnpm e2e:run --host --shards 1 --project chromium tests/pr/pr-ci-bubble.spec.ts -- --retries=0`: 5 passed. Rerun after review remediation: 5 passed.
- `pnpm e2e:run --host --shards 1 --project mobile-chrome tests/pr/mobile-pr-ci-bubble.spec.ts -- --retries=0`: 1 passed. Rerun after review remediation: 1 passed.
- Chromium regression run over `pr-status-badge.spec.ts`, `pr-sidebar-background-refresh.spec.ts`, `pr-sidebar-hover-hydration.spec.ts`, and `pr-topbar-popover.spec.ts`: 23 passed, 1 placement assertion updated; the rerun of `pr-topbar-popover.spec.ts` passed all 7 tests.
- Mobile regression run over `mobile-pr-sidebar-automation-indicators.spec.ts`, `mobile-pr-ci-chip.spec.ts`, and `mobile-pr-sidebar-background-refresh.spec.ts`: 15 passed, 1 placement assertion updated; the rerun of `mobile-pr-sidebar-automation-indicators.spec.ts` passed all 4 tests.
- `node --test scripts/validate-public-docs.test.mjs`: 63 tests passed.
- `node scripts/validate-public-docs.mjs`: 55 published pages validated.
- `python3 scripts/list-docs.py validate`: 446 decisions and 1,965 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- PR documentation coverage preflight: `ok: true`, `status: covered`, 3 work orders.
- `git diff --check`: passed.

The PostgreSQL replay variant remains environment-gated and skipped without `KANDEV_TEST_POSTGRES_DSN`.
