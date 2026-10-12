---
id: "01-root-mutation-synchronization"
title: "Synchronize discovery after successful root mutations"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-LOCAL-REPOSITORIES-002
  - REQ-WORKSPACES-LOCAL-REPOSITORIES-003
acceptance_criteria:
  - AC-WORKSPACES-LOCAL-REPOSITORIES-002.7
  - AC-WORKSPACES-LOCAL-REPOSITORIES-002.9
  - AC-WORKSPACES-LOCAL-REPOSITORIES-002.15
  - AC-WORKSPACES-LOCAL-REPOSITORIES-002.16
  - AC-WORKSPACES-LOCAL-REPOSITORIES-003.3
  - AC-WORKSPACES-LOCAL-REPOSITORIES-003.4
  - AC-WORKSPACES-LOCAL-REPOSITORIES-003.5
  - AC-WORKSPACES-LOCAL-REPOSITORIES-003.6
  - AC-WORKSPACES-LOCAL-REPOSITORIES-003.7
  - AC-WORKSPACES-LOCAL-REPOSITORIES-003.13
  - AC-WORKSPACES-LOCAL-REPOSITORIES-003.14
system_design:
  - ../../specs/workspaces/system-design/local-repositories.md
---

# Task 01: Synchronize Discovery After Successful Root Mutations

## Summary

Add an explicit successful-root-mutation synchronization boundary that cannot
join pre-mutation reads. Connect every immediate root-action caller and prove
shared workspace choices and flags with the real coordinator and production
hooks, preserving existing normal request ordering and sharing.

## In scope

- The paired design's **Successful root mutation synchronization** section.
- Successful Add/Home/Reconnect fresh load and Remove fresh refresh through
  the shared action hook and the two direct Add Home callers.
- Permanent deferred coordinator and transport-only real hook/consumer tests;
  focused direct-caller wiring checks, and final package results.

## Out of scope

- Global invalidation/broadcast, cross-workspace/tab immediate freshness,
  backend/API/store changes, generic request frameworks, cancellation, retries.
- Owner lifetime, error-toasts, Home consent/admission, root failure policy,
  native picker behavior, and foreign edits/processes/workspaces.
- Markup, copy, touch/layout/navigation/breakpoints, browser/E2E/build,
  old unmodified passing suites, generic audits, workers/tasks/session tabs.

## Acceptance

1. Each successful root action starts its correct post-mutation transport even
   with same-kind work pending. Both prior kinds lose publication/follow-up
   authority. Empty roots/repos are accepted by all same-workspace consumers;
   detached success/failure cannot corrupt current choices/metadata/error or
   busy flags. Failure of the new synchronization preserves the accepted
   baseline and current error against old success or failure.
2. Ordinary same-kind reads still coalesce and retain their order. Distinct
   post-boundary reads obey AC-003.13. Failed actions and manual Refresh do not
   invalidate. Preserve mutation and Home admission refs, visibility/lease/
   freshness/failed-root policy, reentry, workspace isolation and disposal.
3. Meaningful permanent RED precedes production edits, then all exact scoped
   checks pass. Race integration uses actual coordinator, discovery/root-action
   hooks and a real shared Office projection, mocking only workspace transport.
   Both direct Add Home success paths use the seam, with failure controls.

## Implementation sequence

1. Wait for explicit later parent reviewed-package implementation interruption
   in this same session. Read this order, the manifest and the paired design.
   Mark status `in_progress`. Do not replay/alter/remove the parent archive.
2. Dependencies are absent at design time. If still absent, perform one
   project-pinned pnpm 9.15.9 frozen install from `apps/` using the existing Node
   runtime. No lockfile/cache edits. Do not repeat an already completed install.
3. Add the permanent tests below. Start with the real Remove pending-refresh
   integration proof using current production APIs, so RED fails on stale
   output or missing fresh read rather than a missing method. Settle old and
   fresh transports causally and assert transport counts and accepted state.
   New-seam unit tests can follow when its public interface exists, but a
   missing-method exception is not the correctness RED evidence.
4. Implement the smallest explicit coordinator/hook boundary and immediate
   glue. Fence both old handles/authority without intermediate notification;
   reserve the selected fresh operation before subscriber callbacks. Reuse
   existing owner/entry checks, normalization and return behavior. Keep manual
   Refresh ordinary. Do not introduce a generic invalidation subsystem.
5. Complete the named matrix and direct-caller tests, run exact GREEN then
   focused lint/typecheck/i18n/document/coverage checks, one heavy command at a
   time. Retain every returned session ID and poll to actual terminal before
   starting the next job. No duplicate run while a handle is unresolved.
6. Record actual results for every listed command and changed suite; mark this
   order `done` and manifest `implemented`. Proceed with standing authorized
   delivery and exact-head hosted review/merge gates in the manifest.

## Required scenarios

Use externally observed state and real exported production behavior. Prefer
parameterized tests over private predicate tests.

- Each handler: Remove with preexisting refresh; Add, Home and Reconnect with
  preexisting snapshot. Assert a second same-kind transport begins after
  success and its response contains changed roots/repos, including empty
  authoritative Remove results. Check root state, freshness, cached flag,
  Home metadata, error, pending flags and Office choices, not roots alone.
- Both boundary modes with both preexisting kinds and both old/new settlement
  orders. Old completion while fresh work is pending cannot publish; old
  completion after fresh success cannot overwrite it or revive loading. Old
  work may remain physically pending after new synchronization settles.
- Seed a nonempty accepted baseline. Both old-success/new-failure and
  old-failure/new-success, plus both-failure settlement directions, retain
  correct current error and baseline/latest choices. Join every started call.
- Mixed root states: one successful changed root and one retained failed root
  from the authoritative new response. No frontend path filtering or invented
  global-empty result; preserve failed-root retry suppression and explicit
  recovery. Empty successful roots/repos replace old nonempty collections.
- Ordinary same-kind calls before and after mutation share one current handle.
  An ordinary read joining the fresh synchronization retains its authority;
  a later distinct opposite-kind start owns publication normally. Repeated
  successful mutations fence the preceding synchronization as well as old reads.
- Failed Add/Home/Reconnect/Remove actions start no synchronization and do not
  invalidate the active read; let that read publish. Manual Refresh during a
  pending refresh remains coalesced. Duplicate mutation and Home-confirmation
  invocations are rejected by existing refs; failure/success finally reset
  busy/admission state for a later action.
- Old snapshot with stale/refreshing/root metadata cannot initiate follow-up.
  A current eligible snapshot retains one ordinary follow-up. Visible, hidden,
  released and failed-root controls preserve the existing policy without timers.
- Reentrant subscriber at fresh request notification joins the installed handle;
  at fresh response publication it can start a newer read without old cleanup
  clearing it. At invalidated settlement it cannot receive a stale publication.
  Dispose during notification is fenced; no removed entry can notify or scan.
- Concurrent A/B entries remain isolated: mutate A with old A/B reads pending;
  A synchronization changes every A subscriber while B work publishes normally
  with no extra B transport. This tests frontend ownership, not different
  backend root sets. Release A before settlement retains valid accepted cache
  without auto follow-up. Dispose and recreate A; old reads cannot affect it.

Integration renders `useRepositoryDiscovery`, `useDiscoveryRootActions` and
`useDiscoveredRepositories` for the same workspace. Only mock
`@/app/actions/workspaces` transports; use actual shared singleton/coordinator
and real Office projection. Unmount before final disposal. Read-only transport
fixture remains parent-owned; use fresh local fixtures in permanent tests.

Direct-caller checks must await the actual action and subsequent synchronization
before asserting. For their focused component tests, stub unrelated presentation
and store scaffolding as needed; keep a real discovery hook/coordinator wherever
the assertion claims current shared state. A hook mock may establish a caller
invokes the seam, but is not freshness evidence. Test rejected action leaves
the seam unused. No new production hook exports solely for testing.

## Mobile and documentation assessment

Pure shared-state repair with no rendered or interaction changes. Apply
`/mobile-parity`'s narrow targeted unit/component exception. No mobile browser,
E2E, build or ASCII preview. Public root/recovery guidance remains accurate;
only the owning internal pair and this package change.

## Verification

Run from repository root. Every command is separate and sequential; an exec
session ID is a running job, not success. Use the already installed runtime:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:$PATH"
```

Prerequisite only if `apps/node_modules` is absent:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

Permanent behavioral RED after adding the real Remove regression and before
production edits (the fixture must run through existing APIs):

```bash
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm exec vitest run hooks/domains/workspace/use-discovery-root-actions.integration.test.tsx --maxWorkers=1)
```

GREEN covers only new race suites and changed caller suites; the existing chip
suite is filtered to the affected Home path instead of replaying unrelated tests:

```bash
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm exec vitest run hooks/domains/workspace/use-repository-discovery.mutation.test.ts hooks/domains/workspace/use-discovery-root-actions.integration.test.tsx components/task/add-workspace-sources/saved-repository-source-row.test.tsx --maxWorkers=1)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm exec vitest run components/task-create-dialog-repo-chips.test.tsx --maxWorkers=1 -t 'adds home folder')
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm exec eslint hooks/domains/workspace/use-repository-discovery.ts hooks/domains/workspace/use-discovery-root-actions.ts hooks/domains/workspace/use-repository-discovery.mutation.test.ts hooks/domains/workspace/use-discovery-root-actions.integration.test.tsx components/task-create-dialog-repo-chips.tsx components/task-create-dialog-repo-chips.test.tsx components/task/add-workspace-sources/saved-repository-source-row.tsx components/task/add-workspace-sources/saved-repository-source-row.test.tsx --max-warnings=0)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm run typecheck)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm run i18n:check)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/repository-discovery-root-mutations
```

Web typecheck is necessary because the hook result and caller contract gain an
explicit method; it is not a generic verification audit. Account for generated
release-notes/changelog outputs from package pretypecheck without committing
incidental changes. No old unmodified product suites or broad coverage replay.

Real repository documentation-coverage preflight (prospective production files
at design time, actual files after implementation):

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const docs = [
  'docs/specs/workspaces/requirements/local-repositories.md',
  'docs/specs/workspaces/system-design/local-repositories.md',
  'docs/plans/repository-discovery-root-mutations/plan.md',
  'docs/plans/repository-discovery-root-mutations/task-01-root-mutation-synchronization.md',
];
const sources = [
  'apps/web/hooks/domains/workspace/use-repository-discovery.ts',
  'apps/web/hooks/domains/workspace/use-discovery-root-actions.ts',
  'apps/web/components/task-create-dialog-repo-chips.tsx',
  'apps/web/components/task/add-workspace-sources/saved-repository-source-row.tsx',
];
const fileContents = Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, 'utf8')]));
const changedFiles = [...docs, ...sources].map(filename => ({ filename, status: 'modified' }));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({ ok: result.ok, status: result.status, errors: result.errors }));
if (!result.ok) process.exitCode = 1;
NODE
```

Design also runs `python3 scripts/lint-spec-files.test.py` once per `/spec`.
After GREEN, repeat a passing check only for a subsequent relevant edit or real
failure. Normal hooks and hosted required checks remain active. Preserve all
foreign processes/workspaces; no machine crash-immunity claim.

## Files likely touched

- `apps/web/hooks/domains/workspace/use-repository-discovery.ts`
- `apps/web/hooks/domains/workspace/use-discovery-root-actions.ts`
- `apps/web/components/task-create-dialog-repo-chips.tsx`
- `apps/web/components/task/add-workspace-sources/saved-repository-source-row.tsx`
- `apps/web/hooks/domains/workspace/use-repository-discovery.mutation.test.ts` (new)
- `apps/web/hooks/domains/workspace/use-discovery-root-actions.integration.test.tsx` (new)
- `apps/web/components/task-create-dialog-repo-chips.test.tsx` (affected Home checks)
- `apps/web/components/task/add-workspace-sources/saved-repository-source-row.test.tsx` (new)
- Owning requirement/design, this work order, and `plan.md`

Read-only inputs: settings reexport, shared root controls, Office hook,
coordinator ordering tests and prior package, current client/backend root
contracts, public desktop/use/configuration guides. No backend/package/lockfile
change. Stop dependent work and send parent evidence if a real finding requires
another production boundary instead of silently widening scope.

## Dependencies

None. Existing React, Vitest and testing-library dependencies suffice. Exactly
one sequential implementation outcome; no delegated implementation.

## Risks

Both old pending handles must be detached before fresh reservation; notification
reentry and disposal must remain fenced. Old physical transports still require
owned settlement/join cleanup. Install-wide backend storage does not provide a
global frontend invalidation event. Current synchronization failure preserves
baseline and surfaces the current error, without implicit retries.

## Parallelism

`sequential`. Sole session; no native or persistent workers/tasks/tabs.

## Inputs

- [Manifest](plan.md): proof, consumer inventory, scope and delivery gates.
- [Owning requirements](../../specs/workspaces/requirements/local-repositories.md):
  REQ-002/003 and the listed acceptance criteria.
- [Owning design](../../specs/workspaces/system-design/local-repositories.md):
  Shared coordinator response ordering / Successful root mutation synchronization.
- Parent-owned read-only fixture; its supplied RED is accepted without replay.

## Results

Implemented on 2026-10-03 after the later explicit parent reviewed-package
release in this same session. No delegation or extra platform tasks/sessions.

- Prerequisite: one `pnpm install --frozen-lockfile` from `apps/`, pinned 9.15.9,
  joined handle 8487 exit 0. Reused all 923 packages, 2 seconds; no lockfile change.
- Exact permanent RED command: joined handle 33041 exit 1, one expected
  assertion failure (`['/removed']` versus `[]`), 28 ms test / 2.70 s package.
  Actual coordinator/discovery/root-action/Office hooks, only transport mocked.
  Parent archive untouched and not replayed.
- Initial corrected one-test GREEN: joined 20505 exit 0, 2.46 s.
- Exact three-file GREEN: joined 93995 exit 0, 60 tests passed, 2.77 s:
  46 coordinator / 12 hook integration / two actual saved-source caller tests.
- Exact focused Create Task Home command: joined 40781 exit 0, two passed /
  23 unrelated tests skipped, 3.70 s. It tests call-site wiring and failure;
  freshness evidence is separately provided by the real shared-hook tests.
- Exact eight-file ESLint initially reported only two test-group callback size
  warnings, joined 94901 exit 1. Removed grouping wrappers without suppression
  or behavioral changes. Affected-file ESLint joined 95003 exit 0; coordinator
  tests rerun joined 86336 exit 0, all 46 passed, 2.43 s. Other lint inputs passed.
- Exact web `pnpm run typecheck` initially joined 97549 exit 2 for one missing
  argument in a test cleanup fixture. Supplied the explicit baseline argument;
  integration rerun joined 43267 exit 0, all 12 passed, 2.52 s; affected-file lint
  joined 54523 exit 0. Exact web typecheck rerun joined 55529 exit 0. Generated
  pretypecheck outputs introduced no tracked diff.
- Exact `pnpm run i18n:check`: joined 37941 exit 0. All keys/catalogs/indices,
  plural/module-scope/punctuation/non-JSX checks passed; existing orphan-key
  advisory is informational (419). No new copy/catalog changes.
- Exact `pnpm run i18n:ratchet`: joined 31355 exit 0; four modified production
  files clean, allowlist intact.

The coordinator now detaches both prior request handles and owner before
reserving the selected fresh operation. Detached settlement owns neither a
handle nor response, so it performs no publication/notification/follow-up.
Current operations retain existing ownership, normal sharing, failure baseline,
metadata and busy state. All four shared root actions and both direct successful
Add Home callers use the new seam; failed actions/manual Refresh remain ordinary.
All initiating workspace subscribers share the accepted state. This does not
provide immediate coherence for other workspace entries or tabs.

Final documentation checks passed: `python3 scripts/list-docs.py validate`
(343 decisions / 1,321 specifications), `python3 scripts/lint-spec-files.py --all`,
and the documented real `validateCoverage` evaluator with the actual four
production paths and four package documents (`covered`, no errors).
`git diff --check` and plan-directory status confirm the complete package;
the sole work order is present. Delivery receipt follows the hosted gates.
All local handles above are actual terminal results; none remains outstanding.
No browser/build/E2E, old unmodified passing suites, generic audit, hidden retry,
assertion weakening, backend policy change, unrelated production edits or
parent-fixture cleanup. Hosted CI and semantic-review gates remain active.
