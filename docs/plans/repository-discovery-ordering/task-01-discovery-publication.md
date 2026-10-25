---
id: "01-discovery-publication"
title: "Preserve discovery publication ownership"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-LOCAL-REPOSITORIES-002
  - REQ-WORKSPACES-LOCAL-REPOSITORIES-003
acceptance_criteria:
  - AC-WORKSPACES-LOCAL-REPOSITORIES-002.9
  - AC-WORKSPACES-LOCAL-REPOSITORIES-003.2
  - AC-WORKSPACES-LOCAL-REPOSITORIES-003.3
  - AC-WORKSPACES-LOCAL-REPOSITORIES-003.4
  - AC-WORKSPACES-LOCAL-REPOSITORIES-003.5
  - AC-WORKSPACES-LOCAL-REPOSITORIES-003.6
  - AC-WORKSPACES-LOCAL-REPOSITORIES-003.7
  - AC-WORKSPACES-LOCAL-REPOSITORIES-003.13
system_design:
  - ../../specs/workspaces/system-design/local-repositories.md
---

# Task 01: Preserve Discovery Publication Ownership

## Summary

Order accepted discovery state across cached reads and refreshes within each
workspace entry. Preserve existing single-flight, lease, visibility, recovery,
normalization, and public method/hook contracts, and prove the exposed behavior
with real coordinator and shared hook/consumer tests.

## In scope

- Entry-local response/error ownership across both request kinds.
- Independent pending-handle cleanup and derived busy state, with accepted
  `refreshing` metadata retained.
- Successful snapshot follow-up ownership, notification reentry, release,
  disposal, same-workspace recreation, and current-response await returns.
- Permanent deterministic regressions and one real hook/Office integration.
- Final results in this work order and the parent manifest; owning specs already
  reconciled by the design package.

## Out of scope

- Invalidating an already pending same-kind read on a root mutation. A repeated
  load/refresh joins the active handle and retains its original authority.
- Backend scan/cache changes, new store/API shapes, request frameworks, timers,
  automatic retry loops, cancellation, and caller production rewrites.
- Markup, copy, mobile interaction, public guidance, broad verification, and
  additional workers/tasks/sessions.

## Acceptance

1. A later newly started request owns response/error publication across both
   kinds in both completion orders. Latest empty success is accepted; current
   failure preserves the accepted baseline/error against older success; obsolete
   failure cannot erase latest success. Same-kind joins do not create requests
   or retake authority. `load()`/`refresh()` await returns reflect current
   accepted registered state.
2. Settlement clears only its own pending work, cannot strand or falsely clear
   busy flags, and preserves accepted cached/refreshing/root-recovery metadata.
   Only current successful snapshots can initiate existing eligible follow-up.
   Synchronous subscriber reentry, workspace isolation, lease release, and
   removed/recreated entries obey the paired design.
3. Permanent regression tests fail for the accepted ordering defect before the
   patch and pass afterward, alongside existing controls. Real shared-hook and
   Office consumer assertions preserve visible repository choices and exposed
   flags with transport-only mocks. No rendered or localized changes are needed.

## Implementation sequence

1. Wait for a later explicit parent implementation continuation. Mark this work
   order `in_progress`. Read the design's **Shared coordinator response ordering**
   section and `apps/web/AGENTS.md`. Do not rerun or delete the parent's fixture.
2. This worktree has no `apps/node_modules` at design time. Perform exactly one
   frozen install from `apps/` before package commands, using managed cache and
   leaving the lockfile unchanged. Reuse an existing completed install if one
   becomes available; do not reinstall gratuitously.
3. Add permanent deferred coordinator assertions. Seed accepted data before
   failure scenarios; use distinct roots, repositories, and metadata so a stale
   write cannot pass by equality. Establish meaningful RED with the two accepted
   overlap cases and real hook/consumer regression. Await all started calls;
   release leases/unmount/dispose in owned cleanup even on assertion failure.
4. Make the smallest coordinator correction. Register pending handles before
   notification; separate data ownership from pending cleanup; check map identity
   and post-notification authority for follow-up. Keep same-kind coalescing.
5. Run the named focused GREEN command once. Expand scenarios only where the
   matrix names an uncovered boundary; do not weaken controls. Run focused lint
   and cheap document/diff/preflight checks, record actual results, mark this
   order `done` and manifest `implemented`, then perform authorized delivery.

## Required scenarios

Use the real exported coordinator, injecting deferred transport only. Prefer
table-driven behavior assertions over helper-unit tests.

- Cached read then refresh, and refresh then cached read; each newer response
  first and each older response first. Assert no obsolete publication even
  while the current operation is pending, plus final state and await returns.
- Both directions: current failure with older success, and obsolete failure
  after latest success. Start from an accepted baseline; assert error identity
  or message, retained choices, metadata and settled flags.
- Latest successful empty repositories/roots replace previous non-empty data.
- Same-kind pending loads and refreshes each share one transport; joining an
  older same-kind handle after a newer opposite-kind start does not gain authority.
- Obsolete stale/refreshing cached response with an active visible lease cannot
  launch a follow-up scan. Current successful stale/refreshing snapshots retain
  the existing eligible follow-up behavior.
- Subscriber reentry at start sees the installed same-kind handle; reentry at
  response publication can start a newer read without old cleanup/follow-up
  disturbing it. Assert request counts and final accepted state.
- Workspace A/B concurrency stays isolated. Release before settlement keeps
  valid cache but prevents automatic follow-up. Dispose before settlement
  does not notify or scan, and cannot alter a recreated same-workspace entry.
- Keep existing visibility, shared lease, failed-root/reconnect suppression,
  explicit recovery, post-action load and Office reopen/switch controls.

The integration test renders real `useRepositoryDiscovery` and
`useDiscoveredRepositories` for the same workspace. Mock only the action
transport. Resolve a newer manual refresh before an older mount snapshot and
assert both consumers retain current choices, with `hasSnapshot`, `cached`,
busy/error/root metadata from the shared hook correct after settlement.
Use fresh or empty roots to avoid an unrelated automatic scan. Dispose the
singleton between scenarios and explicitly unmount before final disposal.

## Mobile and documentation assessment

Pure state/data repair: no layout, touch, scroll, navigation, breakpoint, or
copy changes. Apply `/mobile-parity`'s narrow unit/component-test exception;
no browser/E2E/build/ASCII preview is required. Public documentation already
describes the intended Refresh/recovery workflow; internal specs/plans suffice.

## Verification

Run from repository root, one command at a time. Installation is a prerequisite,
not a repeatable check:

```bash
(cd apps && mise exec -- pnpm install --frozen-lockfile)
```

Permanent RED after adding regressions and before production changes:

```bash
(cd apps/web && mise exec -- pnpm exec vitest run hooks/domains/workspace/use-repository-discovery.test.ts hooks/domains/workspace/use-repository-discovery.integration.test.tsx --maxWorkers=1)
```

GREEN with all changed tests and the existing real consumer controls:

```bash
(cd apps/web && mise exec -- pnpm exec vitest run hooks/domains/workspace/use-repository-discovery.test.ts hooks/domains/workspace/use-repository-discovery.integration.test.tsx app/office/projects/use-discovered-repositories.test.ts --maxWorkers=1)
(cd apps/web && mise exec -- pnpm exec eslint hooks/domains/workspace/use-repository-discovery.ts hooks/domains/workspace/use-repository-discovery.test.ts hooks/domains/workspace/use-repository-discovery.integration.test.tsx --max-warnings=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/repository-discovery-ordering
```

Documentation coverage preflight using the repository's real evaluator. During
design the production path is a prospective change; after implementation it
is an actual change. This does not create a test or modify files:

```bash
mise exec -- node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const docs = [
  'docs/plans/repository-discovery-ordering/plan.md',
  'docs/plans/repository-discovery-ordering/task-01-discovery-publication.md',
  'docs/specs/workspaces/requirements/local-repositories.md',
  'docs/specs/workspaces/system-design/local-repositories.md',
];
const fileContents = Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, 'utf8')]));
const changedFiles = [...docs, 'apps/web/hooks/domains/workspace/use-repository-discovery.ts']
  .map(filename => ({ filename, status: 'modified' }));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({ ok: result.ok, status: result.status, errors: result.errors }));
if (!result.ok) process.exitCode = 1;
NODE
```

Design also runs `python3 scripts/lint-spec-files.test.py` once as prescribed by
the specification skill. No local full Vitest/backend/E2E suites, build, broad
typecheck, verification/security/QA/review audit, synthetic compatibility replay,
or passing check repetition is part of this work order. Normal commit hooks and
required hosted checks remain active.

## Files likely touched

- `apps/web/hooks/domains/workspace/use-repository-discovery.ts`
- `apps/web/hooks/domains/workspace/use-repository-discovery.test.ts`
- `apps/web/hooks/domains/workspace/use-repository-discovery.integration.test.tsx` (new)
- `docs/specs/workspaces/requirements/local-repositories.md`
- `docs/specs/workspaces/system-design/local-repositories.md`
- This work order and `plan.md`

Read-only integration/control inputs:
`apps/web/app/office/projects/use-discovered-repositories.ts` and its test,
`use-discovery-root-actions.ts`, and every consumer in the manifest inventory.
If a concrete finding demands another production boundary, stop dependent work
and send the parent evidence/options instead of widening scope silently.

## Dependencies

None. Existing React/Vitest/testing-library and discovery client types suffice.
No lockfile or package change. This session owns the listed changes; preserve
foreign edits, worktrees, processes, caches, and the parent fixture.

## Risks

Late completion after disposal and synchronous notification can invalidate an
otherwise correct token check. Same-kind sharing cannot serve as root-mutation
invalidation. `refreshing` response metadata and client pending state represent
different activity and must both survive correct cleanup.

## Parallelism

`sequential`. No additional workers/tasks/sessions/native agents.

## Inputs

- [Owning requirements](../../specs/workspaces/requirements/local-repositories.md),
  REQ-002/003 and listed acceptance criteria.
- [Owning design](../../specs/workspaces/system-design/local-repositories.md),
  Discovery flow / Shared coordinator response ordering.
- [Manifest](plan.md): evidence, inventory, delivery and resource constraints.
- Existing coordinator/Office tests and branch-hook request ownership as a local
  reference. The branch hook itself is outside this work order.

## Results

Implemented on 2026-10-02 in this session.

- One frozen `mise exec -- pnpm install --frozen-lockfile` from `apps/` completed
  in 2.3 seconds, reusing 923 cached packages without a lockfile change.
- Permanent RED command: 21 expected assertion failures and 10 passes across
  the coordinator and real shared-hook/Office integration (31 tests, 3.09 s).
  The parent's diagnostic fixture was not rerun or changed.
- The first GREEN attempt exposed one existing control that waited for transport
  invocation rather than publication. Existing snapshot controls now await the
  shared `load()` operation; assertions and timeouts were not weakened.
- Final exact three-file GREEN command: 35 tests passed (30 coordinator, one
  integration, four existing Office controls), 3.02 seconds, `--maxWorkers=1`.
- Exact three-file focused ESLint passed. After the causal-wait test correction,
  affected test-file ESLint also passed; production/integration lint inputs did
  not change. Prettier was applied before final checks.
- Final `python3 scripts/list-docs.py validate`: 343 decisions and 1,311 specs.
- Final `python3 scripts/lint-spec-files.py --all`: all files passed.
- `git diff --check` and plan-directory status check passed.
- Repository documentation-coverage evaluator preflight with the actual
  coordinator change: covered, no errors.

The coordinator now reserves a same-kind handle and workspace-local owner
before notifying subscribers, accepts only current response/error publication,
cleans up each operation independently, and fences disposed entries. Successful
snapshot follow-up rechecks authority and entry identity after notification.
Returned snapshots use the currently registered entry. Existing coalescing,
lease/visibility/freshness policy and manual recovery remain intact.

Only the coordinator, its unit/integration tests, and this four-document package
changed. The paired design is now current. No caller production changes,
new copy, public guidance, backend changes, browser/E2E/build, broad audit,
extra agents/tasks/sessions, or parent-fixture cleanup were needed.
Pending same-kind root-mutation invalidation remains outside scope.

Delivery evidence and actual merged SHA will be reported to the parent after
terminal hosted checks, exact-head full semantic review, normal merge and joined
owned cleanup. The task is not complete merely because this work order passed.
