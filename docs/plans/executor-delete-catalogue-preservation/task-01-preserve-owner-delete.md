---
id: "01-preserve-owner-delete"
title: "Preserve accepted owner-delete publication"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-OWNER-DELETION-001
acceptance_criteria:
  - AC-EXECUTORS-OWNER-DELETION-001.1
  - AC-EXECUTORS-OWNER-DELETION-001.2
  - AC-EXECUTORS-OWNER-DELETION-001.3
system_design:
  - ../../specs/executors/system-design/executor-deletion.md
---

# Task 01: Preserve accepted owner-delete publication

## Summary

Apply accepted executor deletion over the current owning store so only the
target owner disappears and current surviving catalogue choices remain.
Independently prove the active rendered delete/transport/events/selector path
before the local production correction. Start only after ROOT's later explicit
implementation interrupt to the same primary and exclusive global heavy lease.

## In scope

- `DeleteExecutorSection` current-state publication in the real legacy page.
- Independently authored real route/dialog/API-or-WS/store/registered-event/router
  and actual profile-selector tests, ordinary and exact-guard controls.
- Bounded genuine HTTP rejection observed by awaiting the real action adapter,
  explicitly limited to transport evidence.
- Accurate results/status for this one order, manifest and paired specifications.

## Out of scope

- Backend, registry, global state, provider/event handler redesign or dependencies.
- Adjacent profile/policy/connection save, delete, refresh or selection behavior.
- Catch/toast/retry additions, rejected-click redesign, UI markup/copy/layout,
  navigation policy, new browser suites or unchanged broad suite replay.
- Protected ROOT candidate, oversized taska6032d95, protected anonymous volume,
  foreign edits/processes/refs/caches/dependencies and new delegates/sessions.

## Acceptance

1. Independent mixed-live HTTP RED fails because accepted deletion loses a new
   eligible choice and resurrects removed inventory, while ordinary accepted
   deletion and mismatched exact-token controls pass. No fixture/label/setup
   or unhandled errors count as causal evidence. Corrected HTTP/WS success
   passes the matrix and real SPA return/actual selector checks for AC .1-.2.
2. The current owning-store read and synchronous target-ID filter preserve all
   current survivor values/order/profiles. AC .3 retains exact confirmation,
   system exclusion, transport payload, pending/dialog/navigation and original
   rejection propagation. Genuine rejected transport is observed safely at the
   real adapter boundary; rendered rejection is not claimed.
3. Listed scoped checks and actual-path coverage/one-order references pass with
   recorded original joins/results and no resource leakage. Promote draft
   requirement/design only after their implementation criteria are satisfied;
   mark this order done and plan implemented after verification.

## Implementation sequence

After the explicit release, mark only this order in progress. Re-read its pair,
source and scoped guidance; preserve other edits. Install frozen dependencies
once from `apps/` only if the fresh worktree lacks them and ROOT grants heavy
resources. Activate the already installed Node 24 toolchain without global
changes. Do not run package commands until that grant.

Independently author fixtures and tests at the paths below. Hold external fetch
for DELETE with a causal admission promise. Mount `LegacyExecutorSettingsRoute`
and its real page, StateProvider/store and actual save/provider dependencies.
Operate the real dialog using its actual localized label (do not assume label
capitalization), then type the exact lowercase token. Keep all internal API,
store actions, handlers, routing and selector logic real. External Monaco may
be isolated only for unavailable DOM renderer/loader capability, preserving
loader exports and recording the isolation.

Use registered executor-created, executor-deleted and profile-created handlers
to establish valid live owner/profile changes while DELETE is demonstrably
pending. Subscribe to current catalogue and project missing owner name/type
exactly as task creation/subtasks do; use actual `useExecutorProfileOptions`
and open actual `ExecutorProfileSelector` after acceptance/SPA return. Assert
real displayed choice IDs/counts/labels/eligibility and survivor metadata and
inventory, not a mock option list. Keep independent ordinary-success and exact
confirmation controls passing in the initial RED selection. Save original
command handles/joins and causal failure details.

Implement only the section's owning `useAppStoreApi` read after successful
transport and synchronous current-array target filter. Remove the obsolete
captured catalogue subscription. Retain all existing control/transport/finally
behavior. Re-run GREEN and the bounded matrix plus existing affected suites.
Settle every external promise, restore connection/history/guards/stubs, unmount
and drain owned timers on all exits. No original process handle may be discarded
or replaced by a duplicate run. Report any actual unhandled-error failure.

## Regression matrix

New suite describe label: `executor owner deletion publication`.
The independently authored test names below form the verification selectors.

| Test | AC | Observable proof |
| --- | --- | --- |
| `retains mixed live catalogue after accepted HTTP owner deletion` | .1, .2 | Held real HTTP 204, registered new owner/profile plus unrelated owner removal; assert pending current inventory, then target absent, created choice retained, removed choice absent in actual selector after real SPA navigation |
| `ordinary accepted HTTP owner deletion`; `ordinary accepted WS owner deletion` | .1, .2, .3 | Unchanged catalogue success, exact endpoint or action/ID, target and nested choices absent, survivor fields/order unchanged, original hub return and dialog settlement |
| `mismatched exact deletion confirmation` | .3 | Case/whitespace/mismatch remains disabled, no transport admission or catalogue/navigation publication; exact lowercase token enables existing action |
| `retains live survivor fields and profiles` (HTTP and WS cases) | .1, .2 | Registered current metadata/config/status update, sibling profile add/update/remove; compare full pre-acceptance survivors with final survivors and actual current eligible labels/owner metadata |
| `retains mixed live catalogue after accepted WS owner deletion` | .1, .2, .3 | Same mixed membership through real connected-client branch, external held request only, exact action/payload and selector assertions |
| `target notification before acknowledgement` | .1, .3 | Registered target removal while pending; acceptance preserves current survivor objects and does not recreate target, original navigation still commits |
| `system executor has no owner delete action` | .3 | Real page excludes destructive section and dispatches no deletion |
| `rejects HTTP owner deletion at the action boundary` | .3 (transport limit) | Real `deleteExecutorAction` awaited with external HTTP rejection; assert rejected promise and unchanged request semantics, zero unhandled errors; no rendered failure-flow PASS asserted |

The safe adapter rejection test does not exercise the ignored React click
promise or prove final UI cleanup. Audit unchanged `try/finally` ordering and
record that evidence limit. Add rendered rejection only if an existing bounded
harness safely observes the original promise without mocking internal callback
behavior or suppressing errors; otherwise do not broaden the fix.

## Verification

Run sequentially under ROOT's exclusive global heavy grant, from repository
root with independently rooted commands. Node 24 must be active. The helper is
included in ESLint if created. No full suite, Playwright, container, backend or
whole-app build is required for this state/data correction.

```bash
# Once only, if this fresh worktree is missing apps/node_modules:
(cd apps && pnpm install --frozen-lockfile)

# Initial independent RED before production edits: one causal case, two controls.
(cd apps/web && pnpm exec vitest run 'app/settings/executor/[id]/executor-delete-catalogue-publication.test.tsx' -t 'retains mixed live catalogue after accepted HTTP owner deletion|ordinary accepted HTTP owner deletion|mismatched exact deletion confirmation')

# GREEN plus affected page/route/selector controls after correction.
(cd apps/web && pnpm exec vitest run 'app/settings/executor/[id]/executor-delete-catalogue-publication.test.tsx' 'app/settings/executor/[id]/executor-policy-catalogue-publication.test.tsx' components/settings/legacy-executor-settings-route.test.tsx src/settings-routes.test.ts components/task-create-dialog-selectors.test.tsx)
(cd apps/web && pnpm exec eslint --max-warnings 0 'app/settings/executor/[id]/page.tsx' 'app/settings/executor/[id]/executor-delete-catalogue-publication.test.tsx')
# If created, also lint app/settings/executor/[id]/executor-delete-catalogue-publication.test-helpers.tsx.
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short
```

Use `.github/scripts/pr-docs.cjs` `validateCoverage` on the actual tracked/staged
and untracked inventory. Require implementation `covered` with no errors,
exactly one changed work order and complete requirement/AC/design/plan references.
Record the actual paths and results, not planned counts. A docs-only exemption
does not prove implementation coverage. Light design scripts are permitted
before implementation; package/install/test/lint/build/hooks are not.

Normal commit hooks remain enabled for authorized publication. If a hook
reformats an owned file, re-stage and make a new commit; never bypass hooks.
Return heavy resources and proceed directly to authorized hosted validation in
the same turn. ROOT owns the separate static check and expected-head squash
grant; no merge or protected ROOT-source cleanup before that grant/release.

## Files likely touched

- `apps/web/app/settings/executor/[id]/page.tsx` (only `DeleteExecutorSection`).
- `apps/web/app/settings/executor/[id]/executor-delete-catalogue-publication.test.tsx`.
- `apps/web/app/settings/executor/[id]/executor-delete-catalogue-publication.test-helpers.tsx` if needed.
- `docs/specs/executors/requirements/executor-deletion.md`.
- `docs/specs/executors/system-design/executor-deletion.md`.
- This manifest and work order.

## Dependencies

None. Execution requires ROOT's later explicit implementation release and
exclusive heavy lease, not another work order or delegate.

## Risks

- Fixtures can miss the live path by using system targets, wrong label case,
  owner-only creation events or mismatched profile ownership.
- Mocked router/page/selector or early current reads can hide the defect.
- React's unawaited rejection must not become a false passing control.
- Fake timer/provider/history cleanup must not leave foreign process or state
  ownership ambiguous; original command handles must actually join.

## Parallelism

`sequential`. Same primary only; no delegation or model/session changes.

## Inputs

- [Owning requirement](../../specs/executors/requirements/executor-deletion.md).
- [Owning design](../../specs/executors/system-design/executor-deletion.md).
- [Qualified root cause](plan.md#qualified-root-cause), receipt only.
- Real legacy route, page, action adapter, registered executor/profile handlers,
  state provider, client router, options hook and selector.
- Existing executor-policy integration tests for patterns only; do not copy or
  inspect ROOT's protected candidate. Mobile-parity state/data exception applies.

## Results

Implemented after ROOT's later explicit release and exclusive heavy grant.
Only the owner-delete section's publication changed: the post-acknowledgement
current owning store supplies its target-ID filter. The route, actual adapters,
registered event handlers and selector remain real in independent integration
coverage. Monaco's external renderer alone is stubbed, retaining loader exports.

| Command / evidence | Original native / exit / join | Result |
| --- | --- | --- |
| Frozen `pnpm install --frozen-lockfile` from apps | 24572 / 0 / b2d8db | One needed install; no lockfile change |
| Initial RED selector | 99946 / 1 / b055c4 | Fixture-only unavailable DOM matcher error, not causal |
| Corrected independent RED selector before production | 68140 / 1 / e11610 | One causal mixed-live HTTP failure; two ordinary/exact-confirmation controls pass |
| Full listed five-suite GREEN | 20314 / 0 / 2971c2 | 118 tests pass, including 10 new |
| Initial scoped ESLint | 52248 / 1 / ffddc3 | Test-group length and duplicate-literal warnings, corrected |
| Scoped ESLint after grouping/constants | 77759 / 0 / c0f9d6 | Zero warnings |
| Initial typecheck | 43399 / 2 / 637ea4 | Full executor notification fixture fields missing, corrected |
| New suite after full notification fixtures and rendered metadata/eligibility assertions | 19081 / 0 / c2ab61 | 10 tests pass |
| Scoped ESLint after fixture edits | 15037 / 1 / 9f4409 | Unused import, removed |
| Sequential scoped lint, typecheck, i18n check and ratchet | 98448 / 0 / 5e6582 | All pass; no new copy/catalogue changes |
| Formatter, new suite and lint | 7799 / 1 / bfd3fe | 10 tests pass; formatter wrapping exceeded test-group line limit, corrected |
| Formatter, affected new suite, scoped lint and typecheck | 30125 / 0 / 194428 | 10 tests pass, zero lint warnings and typecheck passes |
| First normal commit hooks | 95769 / 1 / 9c9892 | Required i18n-new-code hook caught newly staged helper placeholder; commit not created |
| Final helper localization, new suite, scoped lint/typecheck/i18n/staged ratchet | 25491 / 0 / 2b5c50 | 10 tests and all static/copy checks pass; existing translated key used |

The full five-suite command is recorded in Verification above; only the changed
new suite was repeated after test fixture/assertion/grouping changes. No passing
broad suite was replayed. Final ESLint includes the created same-directory
helper. Final typecheck follows the helper localization. i18n check and ratchet ran
from apps/web as listed. The initial unstaged ratchet omitted the then-untracked
new helper; the normal staged hook caught its literal placeholder. The helper
now uses existing `t("task:executorProfile2")`, and the final staged ratchet
checks one added plus one modified file successfully. No locale key was added.
No backend/container/Playwright/full-suite/build check was run.

The initial matcher failure is not RED evidence. The causal RED shows both
catalogue replacement and actual displayed selector loss/resurrection after
real SPA return. Full notification DTOs corrected by typecheck retain the same
registered-handler interleaving. Safe genuine 403 rejection is explicitly
awaited through the real HTTP action; it is transport-only evidence and no
rendered failed-click PASS is claimed. Source inspection confirms rejection
still skips publication/navigation and reaches the unchanged try/finally.

Every original handle above actually joined. Per-command logs, clocks, process
IDs and group-absence receipts are in
`/tmp/kandev-child108-executor-delete-design-20261010.RX4pOB/runs/`.
All recorded child groups are absent at their joins. Actual seven-path coverage
and final documentation gates are recorded in the manifest. Normal hooks,
commit/ordinary push/PR and hosted readiness follow these local results; no
merge is authorized by this completed work order.
