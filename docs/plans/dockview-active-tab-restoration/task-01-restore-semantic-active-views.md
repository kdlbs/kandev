---
id: "01-restore-semantic-active-views"
title: "Restore semantic Dockview active views"
status: done
wave: 1
depends_on: []
parallelism: sequential
plan: "plan.md"
spec: "../../specs/ui/task-layout-profiles.md"
---

# Task 01: Restore Semantic Dockview Active Views

## Acceptance

- A saved or default Agent selection represented by `chat` or a stale
  `session:<id>` selects the incoming task's live Agent panel after environment
  restoration.
- A deliberately selected, still-live non-Agent panel such as PR Details,
  Plan, Files, or Changes remains selected by exact identity.
- Every live group's target selection is restored, with the serialized global
  active group applied last.
- Fast-path, slow-path, saved-layout, and fresh-default task switches follow the
  same selection semantics.
- Pending inactive-task Changes attention, session-tab ordering, group
  structure, layout geometry, and mobile/tablet behavior remain unchanged.

## TDD Sequence

1. RED: add fast-path unit cases where `chat` and a stale session ID are saved
   as active beside PR Details; prove exact-ID replay leaves PR Details active.
2. RED: add the slow-path stale-session replacement case, exact PR Details
   preservation, multi-group/global-focus ordering case, and fresh-default
   selection case.
3. RED: add the production-build desktop round trips proving both Agent-last
   and PR-Details-last behavior across two tasks.
4. GREEN: normalize the target layout's selected views, rebind only semantic
   Agent identities to the incoming session panel, and replay selections after
   session reconciliation on both environment-switch paths.
5. REFACTOR: keep target-selection extraction, semantic identity resolution,
   and ordered Dockview activation explicit; do not widen the downstream
   session-tab activation policy.
6. Run the exact targeted unit, typecheck, and production-build E2E commands.

## Verification

If this worktree has not installed dependencies yet, bootstrap once:

```bash
cd apps && pnpm install --frozen-lockfile
```

Then run:

```bash
cd apps && pnpm --filter @kandev/web test -- --run \
  lib/state/dockview-env-switch.test.ts \
  components/task/dockview-session-tab-activation.test.ts \
  components/task/dockview-session-tabs.test.ts
cd apps/web && pnpm run typecheck
cd apps/web && pnpm e2e:run tests/pr/pr-detail-layout.spec.ts \
  -- --grep "restores each task's selected center tab"
```

## Files Likely Touched

- `apps/web/lib/state/dockview-env-switch.ts`
- `apps/web/lib/state/dockview-env-switch.test.ts`
- `apps/web/e2e/tests/pr/pr-detail-layout.spec.ts`
- `apps/web/e2e/pages/session-page.ts` only if the scenario needs a reusable
  active-session-tab locator or Dockview-state helper.

## Dependencies

None.

## Parallelism

Sequential. The unit cases, post-reconciliation ordering, and browser round
trip describe one timing-sensitive Dockview state transition and should remain
in a single Red-Green-Refactor cycle.

## Inputs

- `docs/specs/ui/task-layout-profiles.md`, especially desktop restoration,
  persistence guarantees, and the Agent/PR Details round-trip scenarios.
- `docs/plans/dockview-active-tab-restoration/plan.md`.
- `apps/web/lib/state/dockview-env-switch.ts`, including
  `restoreSavedActiveViews`, `tryFastEnvSwitch`, `replaceStaleSessionPanels`,
  `restoreMissingSessionPanel`, and the slow `fromJSON` path.
- `apps/web/lib/state/layout-manager/session-panels.ts` for the existing
  reusable-Chat-to-live-session materialization semantics.
- Existing task-switch coverage in
  `apps/web/e2e/tests/layout/preview-tab-session-switch.spec.ts` and PR Details
  helpers in `apps/web/e2e/tests/pr/pr-detail-layout.spec.ts`.

## Risks

- A final `api.activePanel` assertion alone does not prove every group's local
  selected tab; unit and E2E checks must inspect both concepts.
- Activation order matters because each `panel.api.setActive()` can update the
  global active group. Apply the intended global group last.
- A stale selected ID may represent a removed non-Agent task-specific panel;
  do not map arbitrary missing IDs to Agent.
- Default selection extraction must materialize only the semantic Agent ID and
  must not rebuild or overwrite the fast-path layout structure.

## Output Contract

Report the RED failures, semantic mapping rule, minimum production change,
exact files changed, unit/typecheck/E2E results, plan/task status updates, and
any remaining ambiguity in Dockview's group-local versus global focus state.

## Results

- RED unit run: 4 expected failures showed exact-ID replay skipped saved
  `chat`/stale session selections and did not restore the default Agent view.
- GREEN implementation: `restoreTargetActiveViews` rebinds semantic Agent IDs
  after session replacement, preserves exact non-Agent IDs, and orders the
  serialized active group last. The same rule now covers saved and fresh
  default layouts on both fast and slow paths.
- Unit verification: 70 tests passed across the environment-switch and session
  activation suites.
- Typecheck: `pnpm run typecheck` passed.
- E2E verification: the managed production-build Chromium scenario passed
  (1/1), including Agent-last restoration, exactly one live session tab, and
  deliberate PR Details preservation.
- Targeted ESLint: no errors; two max-lines warnings remain in
  `dockview-env-switch.test.ts`.
- Formatting: Prettier check passed for all changed code and plan/task files;
  `git diff --check` passed.
- Mobile/tablet: no rendered or responsive path changed; those viewports do
  not mount `DockviewDesktopLayout` or execute this restoration hook.
