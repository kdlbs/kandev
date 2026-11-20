---
id: "04-route-native-checks"
title: "Route required checks to native tools"
status: completed
wave: 4
depends_on: ["03-migrate-full-oxlint"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-009
  - REQ-PLATFORM-CI-PERFORMANCE-010
  - REQ-PLATFORM-CI-PERFORMANCE-011
acceptance_criteria:
  - AC-PLATFORM-CI-PERFORMANCE-009.5
  - AC-PLATFORM-CI-PERFORMANCE-010.4
  - AC-PLATFORM-CI-PERFORMANCE-010.5
  - AC-PLATFORM-CI-PERFORMANCE-011.1
  - AC-PLATFORM-CI-PERFORMANCE-011.4
system_design:
  - ../../specs/platform/system-design/web-native-verification.md
---

# Task 04: Route required checks to native tools

## Summary

Route normal local and hosted web verification through the accepted native candidates.
Preserve existing command names, generation, failure propagation, and workflow topology.
This task prepares the reviewable command change. Task 05 owns production adoption and rollback after performance acceptance.

## In scope

- Replace the final web `tsc` invocation with `tsrs --noEmit --fast`.
- Retain `pretypecheck`, `rich-output:check`, project settings, and current heap configuration.
- Route full and focused lint package scripts to the native configurations.
- Route the existing changed-file hook through the same native policy with unchanged filename forwarding and selection.
- Replace the targeted ESLint CLI call in `upgrade-regression.yml` without changing its source selection or required result.
- Retain API-based ratchets, wait inventory, RuleTester, and classic TypeScript resolution.
- Add a focused package/hook contract test and extend existing frontend workflow contracts through TDD.
- Register evidence-helper tests in the existing action-pinning job.
- Retain all required gate names, dependency joins, statuses, permissions, runner allocation, matrices, and concurrency expressions.
- Run the complete frontend suite once after integration, with the existing local worker budget.
- Update root/scoped engineering guidance for primary commands and retained API consumers.
- Reconcile the Query lint and System Query guard designs with the accepted native host.
- Retain the existing real-ESLint wiring assertions required by the architecture contract, and add native-host assertions for the required CLI.
- Supersede only the old architecture decision's host choice after successful cutover checks.

## Out of scope

- New jobs, new CI triggers, runner activation, SDK/desktop compiler replacement, and release workflow changes.
- Permanent duplicate full lint or compiler commands in the normal verification path.
- Deleting guard fixtures, removing strictness, disabling fast mode, or bypassing unsuccessful commands.

## Acceptance

1. Stable package commands and the changed-file hook invoke the exact intended tools, with generation, source selection, and original failures preserved.
2. Classic API consumers and unchanged package compilers pass. Existing gates reject candidate errors and retain valid irrelevant-change skips.
3. Integration tests, full frontend tests, production build, and tool-specific guidance agree with the new host boundaries.

## Verification

`scripts/web-native-toolchain.test.mjs` is a planned new contract test.
Its failure fixtures must execute command routing, including missing-binary and child-error cases.

```bash
node --test scripts/web-native-toolchain.test.mjs
python3 .github/scripts/web-toolchain-evidence_test.py
python3 .github/scripts/frontend-tests-workflow-contract_test.py
python3 .github/scripts/ci-gates_test.py
python3 .github/scripts/upgrade-regression-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
(cd apps/web && pnpm run lint)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm run e2e:sleep-ratchet)
(cd apps/web && pnpm run test)
(cd apps/web && pnpm run build:vite)
(cd apps && pnpm --filter @kdlbs/plugin-sdk test && pnpm --filter @kdlbs/plugin-sdk typecheck)
actionlint .github/workflows/frontend-tests.yml .github/workflows/upgrade-regression.yml .github/workflows/lint-action-pinning.yml
git diff --check
```

The upgrade-regression contract file is planned if no current dedicated contract exists.
Use the existing contract location if implementation discovery finds one.
Do not overlap the full frontend suite with another suite or override its worker budget.
Add focused ESLint-API adapter checks to the package contract test without changing their public scope.

## Files likely touched

- `apps/web/package.json` and `.pre-commit-config.yaml`.
- `scripts/web-native-toolchain.test.mjs` (new).
- `.github/workflows/upgrade-regression.yml` and `.github/workflows/lint-action-pinning.yml`.
- `.github/workflows/frontend-tests.yml` only where command or helper wiring needs a change.
- Existing frontend/gate contracts and the planned upgrade-regression workflow contract.
- `AGENTS.md`, `apps/web/AGENTS.md`, and contributor tool guidance that names direct old commands.
- `docs/specs/platform/system-design/web-tanstack-query-eslint.md`.
- `docs/specs/architecture-lint/system-design/migrated-system-query-owner.md`.
- `docs/decisions/2026-10-08-bounded-frontend-eslint-architecture-guard.md`.

## Dependencies

Tasks 01–03, including complete candidate compatibility and source inventory evidence.
Read `.github/AGENTS.md` before workflow changes.
Use the harness-improvement skill before engineering instruction changes.
The design's Entry points section defines the retained API boundary.

## Risks

- A hook can pass a different configuration or lose filenames that contain spaces.
- A native package can shadow workspace `tsc` even when the web command appears correct.
- Legacy API tests alone cannot prove that CI runs the migrated guards.
- The required frontend job can appear green if child failure propagation is lost.

## Parallelism

`sequential`

## Results

Completed on November 3, 2026.

- `apps/web/package.json` now routes `lint`, `lint:i18n`, and `lint:e2e-sleeps`
  to the native configurations with `--deny-warnings`. `typecheck` keeps
  `pretypecheck` generation, runs `rich-output:check`, then runs
  `tsrs --noEmit --fast`. The existing heap configuration and project settings
  are unchanged.
- `.pre-commit-config.yaml` forwards the same changed filenames to the same
  native policy. The upgrade-regression harness step runs the native
  configuration over its exact file selection. No job, trigger, permission,
  matrix, concurrency expression, runner, or required gate changed.
- Added `scripts/web-native-toolchain.test.mjs` (9 passing cases). It executes
  the real hook routing with a filename containing spaces, a clean file, a
  missing-binary path, and a lint error; asserts the package commands, pins,
  retained API dependencies, and workflow call sites.
- Added `.github/scripts/upgrade-regression-workflow-contract_test.py` (3
  passing cases). The evidence-helper tests run in the existing action-pinning
  job; the routing contract runs as a step in the existing frontend static
  check, where dependencies are installed, so no job or install cost was added.
  The existing frontend, gate, action-pinning, and size-label contracts pass.
- `pnpm run lint`, `NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck`,
  `pnpm run i18n:check`, `pnpm run i18n:ratchet`, and
  `pnpm run e2e:sleep-ratchet` pass.
- The complete frontend suite passed: 3,261 files and 29,610 tests
  (4 skipped). `pnpm run build:vite` passed in 16.40 seconds. The standalone
  plugin SDK tests and typecheck passed. `actionlint` passed on the frontend,
  upgrade-regression, action-pinning, and size-label workflows.
- Updated root and scoped contributor guidance, the Query lint design, the
  System Query guard design, and the prior architecture decision's host note.
  The Python architecture engine, RuleTester, and ESLint API consumers remain
  unchanged.
