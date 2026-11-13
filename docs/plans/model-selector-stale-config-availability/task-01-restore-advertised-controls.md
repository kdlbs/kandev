---
id: "01-restore-advertised-controls"
title: "Restore advertised model controls with stale runtime config"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001
acceptance_criteria:
  - AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001.4
  - AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001.5
  - AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001.7
system_design:
  - ../../specs/platform/system-design/session-config-cross-agent-reconcile.md
---

# Task 01: Restore advertised model controls

## Summary

Make the selector gate accept a usable advertised catalog despite historical
runtime-only keys and absent or false settlement. Prove the correction through
real hydration inputs, live handler inputs, and desktop/phone interactions.

## In scope

- Write failing regressions for the sanitized issue fixture before changing the gate.
- Implement the additional runtime-only exception defined in the system design.
- Cover profile overlaps, category aliases, missing markers, mixed keys,
  live option removal, and empty or flat-only catalogs.
- Extend existing desktop and phone E2E specs with the stale-config case.
- Record exact results and synchronize this work order with `plan.md`.

## Out of scope

- Backend changes, new state fields, marker inference, or metadata cleanup.
- Profile policy changes, permission changes, new UI surfaces, copy, or runtime flags.
- Reconstructing the original incident's missing-marker transition.

## Acceptance

1. A usable advertised catalog releases only unadvertised runtime-only non-agent
   keys. Both profile sources and all existing readiness exceptions retain their behavior.
2. Boot, client fetch, and live handler inputs preserve settlement and saved
   metadata. The picker renders and sends only explicit advertised choices.
3. Desktop and phone can select a model and open its advertised option controls.
   Startup, rollback, consecutive-switch, and phone target checks still pass.

The requirement IDs and complete behavior live in the linked sources.
The [plan test matrix](plan.md#tests) defines the permanent regression cases.

## ASCII UI preview

UI-01 and UI-02, existing task conversation with a usable catalog and no settlement:

```text
Desktop before: [Mode v]                       [Send]
Desktop after:  [Mode v]       [Composer 2.5 v] [Send]

Phone after:
+------------------------------------+
| Message input                      |
| [Mode v] [Composer 2.5 v]   [Send] |
+------------------------------------+
```

The [full previews](plan.md#ascii-ui-preview) define the structural requirements
for AC-001.4, AC-001.5, and AC-001.7. Width is illustrative. Reuse the shipped
toolbar and touch picker with their existing wrapping behavior.

## Implementation sequence

1. Mark this work order `in_progress` and retain the package as the source of scope.
2. Add the gate and component regressions. Run them and record the expected
   pre-fix failure at selector availability, not at fixture setup.
3. Add the desktop and phone cases before the production correction. Observe
   the missing trigger with the seeded false/absent-marker state.
4. Change only the gate's runtime-only handling. Keep the existing settled,
   flat-model, profile, and legacy-agent cases.
5. Run every verification command. Compare the rendered result with UI-01 and
   UI-02 and record actual counts, failures, and any unresolved difference.
6. Mark the work order `done` only after its checks pass. Update the plan results.

## Verification

Run this block from the repository root. If workspace dependencies are absent,
first run `(cd apps && pnpm install --frozen-lockfile)`.
The managed E2E commands build fresh production assets and use isolated data.
Run desktop and phone sequentially.

```bash
(cd apps/web && pnpm exec vitest run components/task/model-selector.test.ts components/task/model-selector-stale-config-gate.test.ts components/task/model-selector-stale-config.test.tsx components/task/model-selector-consecutive-switch.test.tsx lib/state/slices/session-runtime/session-model-hydration.test.ts lib/ssr/session-page-model-hydration.test.ts lib/ws/handlers/session-models-startup.test.ts lib/ws/handlers/session-models-confirmed-config.test.ts lib/ws/handlers/session-models-user-selection.test.ts)
node apps/web/scripts/generate-release-notes.mjs
node apps/web/scripts/generate-changelog.mjs
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/model-selector.tsx components/task/model-selector.test.ts components/task/model-selector-stale-config.test.tsx e2e/tests/chat/model-selector-error.spec.ts e2e/tests/chat/mobile-model-selector.spec.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --project chromium tests/chat/model-selector-error.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/chat/mobile-model-selector.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/model-selector-stale-config-availability
```

The two generated JSON files are ignored build prerequisites. Do not commit them.
Run this local PR documentation coverage preflight from the repository root.
The prospective source path exercises reference validation during the design turn.

```bash
node <<'NODE'
const fs = require('node:fs');
const cp = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const artifacts = [
  'docs/plans/model-selector-stale-config-availability/plan.md',
  'docs/plans/model-selector-stale-config-availability/task-01-restore-advertised-controls.md',
  'docs/specs/platform/requirements/session-config-cross-agent-reconcile.md',
  'docs/specs/platform/system-design/session-config-cross-agent-reconcile.md',
];
const changedFiles = [...new Set([
  ...cp.execFileSync('git', ['diff', '--name-only', '-z', 'HEAD'], { encoding: 'utf8' }).split('\0'),
  ...cp.execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], { encoding: 'utf8' }).split('\0'),
].filter(Boolean))];
const fileContents = Object.fromEntries(artifacts.map(p => [p, fs.readFileSync(p, 'utf8')]));
for (const [label, paths] of [
  ['actual', changedFiles],
  ['prospective', [...new Set([...changedFiles, 'apps/web/components/task/model-selector.tsx'])]],
]) {
  const result = validateCoverage({ changedFiles: paths, fileContents });
  console.log(JSON.stringify({ label, ok: result.ok, status: result.status, errors: result.errors }));
  if (!result.ok) process.exitCode = 1;
}
NODE
```

## Files likely touched

- `apps/web/components/task/model-selector.tsx`
- `apps/web/components/task/model-selector.test.ts`
- `apps/web/components/task/model-selector-stale-config-gate.test.ts` (new)
- `apps/web/components/task/model-selector-stale-config.test.tsx` (new)
- `apps/web/e2e/helpers/session-store.ts` (shared store-bridge seed helper)
- `apps/web/e2e/tests/chat/model-selector-error.spec.ts`
- `apps/web/e2e/tests/chat/mobile-model-selector.spec.ts`
- This work order and `docs/plans/model-selector-stale-config-availability/plan.md`.

The existing hydration and handler tests are compatibility checks. Their production
modules require no edits. A newly discovered need for transport or persistence
changes requires updated evidence and package scope before implementation.

## Dependencies

None. One vertical work order owns the correction and its user-visible evidence.

## Risks

- A mixed key set must satisfy every remaining requirement. Ignoring one stale
  key must not release an unresolved profile key.
- A static catalog or unusable model option must not satisfy the new exception.
- E2E fixture injection must follow initial hydration and assert its own state
  before interaction. Late startup events can otherwise erase the regression fixture.
- The original missing-marker sequence remains outside the evidence available here.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/platform/requirements/session-config-cross-agent-reconcile.md),
  criteria .4, .5, and .7 and the existing replay guarantees.
- [System design](../../specs/platform/system-design/session-config-cross-agent-reconcile.md),
  selector availability and hydration/live updates.
- [Plan](plan.md), confirmed reproduction, compatibility matrix, tests, and previews.
- `model-selector-consecutive-switch.test.tsx` for component/store mocks.
- Both session hydration helpers and `registerSessionModelsHandlers` for actual inputs.
- Existing desktop error and phone selector specs for browser fixture and interaction patterns.
- The `fix`, `tdd`, `mobile-parity`, and `e2e` skills and scoped web guidance.

## Results

Done.

- Change (`model-selector.tsx`): `hasCompleteDynamicConfig` now accepts an
  unadvertised key when it is recorded only in `runtime_config`/`_overrides`, is
  not required by the profile snapshot or matching profile, is not `agent`, and
  the session entry has a usable advertised catalog. `hasUsableAdvertisedCatalog`
  requires a raw config option plus a flat model or a usable model-shaped select
  (category aliases included). Settlement is not inferred or mutated.
- Tests (TDD): red first with `expected false to be true` in the gate suite and
  no rendered trigger in the component suite. Added
  `model-selector-stale-config-gate.test.ts` (11 tests) and
  `model-selector-stale-config.test.tsx` (7 tests, including a provider-restored
  entry), replaced the violation test in `model-selector.test.ts` (31 tests),
  and retained `model-selector-consecutive-switch.test.tsx` (2 tests). Gate-only cases moved
  to their own file to keep `model-selector.test.ts` inside the `max-lines`
  budget.
- Browser: added `seedStaleRuntimeConfigWithoutSettlement` to the shared E2E
  store helper. Desktop `model-selector-error.spec.ts` 8 passed; phone
  `mobile-model-selector.spec.ts` 3 passed. Both new cases seed after initial
  hydration, assert the cleared marker and historical keys, select Mock Smart
  through the real mock backend, open the advertised effort control, and assert
  only `model` reaches the setter. The phone case also keeps the 44px trigger
  and viewport-containment checks.
- Commands:
  - focused Vitest block plus the gate suite: 9 files, 73 passed
  - `NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck`: passed
  - eslint on the changed files: 0 errors, 0 warnings
  - `pnpm run i18n:check` and `pnpm run i18n:ratchet`: passed
  - desktop and phone managed E2E commands: 8 passed and 3 passed
  - `python3 scripts/list-docs.py validate` and
    `python3 scripts/lint-spec-files.py --all`: passed
  - documentation coverage preflight: actual and prospective changes covered
  - `git diff --check`: passed
- Unresolved: none. The original missing-marker transition remains unproven, as
  scoped.
