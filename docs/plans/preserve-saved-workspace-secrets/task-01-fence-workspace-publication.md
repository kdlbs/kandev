---
id: "01-fence-workspace-publication"
title: "Fence workspace initial success publication"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-SECRET-CATALOGUE-001
  - REQ-WORKSPACES-REPOSITORY-SECRETS-001
acceptance_criteria:
  - AC-WORKSPACES-SECRET-CATALOGUE-001.1
  - AC-WORKSPACES-SECRET-CATALOGUE-001.2
  - AC-WORKSPACES-SECRET-CATALOGUE-001.3
  - AC-WORKSPACES-SECRET-CATALOGUE-001.4
  - AC-WORKSPACES-REPOSITORY-SECRETS-001.1
  - AC-WORKSPACES-REPOSITORY-SECRETS-001.2
  - AC-WORKSPACES-REPOSITORY-SECRETS-001.14
system_design:
  - ../../specs/workspaces/system-design/workspace-secret-catalogue.md
---

# Task 01: Fence workspace initial success publication

## Summary

Independently prove and fix the workspace initial-success publication race.
Accepted local metadata remains authoritative within its current lifetime,
while ordinary reads, scoped cancellation, replacement inputs, and readiness
retain their behavior. Execute only after ROOT's later explicit same-primary
implementation release and exclusive local-heavy grant.

## In scope and owned files

- Production: `apps/web/hooks/domains/settings/use-secrets.ts`, workspace success
  effect/callbacks and required ref import only. Preserve sibling Global code.
- Independently authored hook suite:
  `apps/web/hooks/domains/settings/use-secrets.workspace-publication.test.tsx`.
- Independently authored component suite:
  `apps/web/components/settings/secrets-settings.workspace-publication.test.tsx`.
- Four design artifacts in this package, with statuses/results synchronized
  after actual implementation; no unrelated tracked procedural edits.

## Out of scope

The [manifest exclusions](plan.md#scope) apply. Never read/copy/mutate/delete
ROOT's protected candidate fixture or sibling tests/spec edits. Do not change
Global shared ownership, workspace catch settlement, API/backend/auth/values,
transfers/bindings, lifecycle/cache policy, component markup, localization,
package manifests/lockfile, or old plans.

## Acceptance

1. The independently authored principal component test fails before production
   change because the real acknowledged row disappears after the older empty
   successful GET, with both ordinary controls passing and no fixture/transport/
   unhandled failure. After the minimal fence it passes with no second GET.
2. Hook tests prove whole-current-list authority, empty/restored histories,
   readiness, and uncontested lists through current workspace callbacks; scoped
   cancellation/replacement/unmount controls and all listed existing suites pass.
3. All seven frontmatter criteria map to the
   [manifest test table](plan.md#tests-and-all-ac-mapping), actual checks/joins are
   recorded, and sibling Global production and reviewed definitions remain intact.

## Implementation sequence

1. Read this package and current exact base/sibling contracts again. Resolve
   moved/merged dependency evidence without overwriting foreign work. Confirm
   ROOT's release and exclusive heavy grant before any install/product command.
2. Use `/tdd`; mark this work order `in_progress`. If dependencies are missing,
   run the normal frozen install once from `apps/`, retaining its original handle:
   `(cd apps && pnpm install --frozen-lockfile)`. Preserve existing dependencies
   and cache; do not install during design.
3. Author the two suites without using protected fixtures. Keep real providers,
   store, hook, API adapters, form/shared Save and row widgets. Defer only fetch
   transport using valid synthetic HTTP 200 responses. Assert exact scoped GET
   query and workspace POST payload, GET-before-POST ordering, row visibility
   before releasing GET, then retention afterward. Drain deferred fetches and
   restore globals during cleanup so failures cannot leave pending work.
4. Run the first block below on unmodified production and qualify the causal
   RED plus ordinary controls. Test failures from setup do not count as causal
   evidence. Record original command handle, actual exit and join. Companion
   hook cases use current add/update/remove callbacks and test complete mixed
   metadata, snapshot-only rows, empty current state after add/remove, and
   rename/restoration with equal final values.
5. Apply only the
   [local success fence](../../specs/workspaces/system-design/workspace-secret-catalogue.md#success-publication-fence).
   Advance the local ref synchronously before functional local writes, capture
   it at read admission, and suppress only superseded successful contents.
   Keep current success readiness/finalization, cancellation, dependencies,
   ordinary reads, supplied arrays, absent ID, and catch branch.
6. Run all exact scoped checks below serially under the grant. Record results
   from the original handles; do not discard one and infer its exit from a
   repeated command. Re-run only checks justified by later changes or failures.
7. Run full actual-path documentation/link coverage, synchronize `done` and
   manifest results only after checks pass, then follow authorized normal hooks
   and delivery gates in the manifest. Design status is not implementation done.

## Verification after release

Run from repo root. First block is the new causal RED and then the same GREEN:

```bash
(cd apps/web && timeout --signal=TERM --kill-after=10s 4m pnpm exec vitest run hooks/domains/settings/use-secrets.workspace-publication.test.tsx components/settings/secrets-settings.workspace-publication.test.tsx --maxWorkers=1 --no-file-parallelism)
```

Final affected suites include every new suite plus the established local/global
compatibility controls:

```bash
(cd apps/web && timeout --signal=TERM --kill-after=10s 5m pnpm exec vitest run hooks/domains/settings/use-secrets.workspace-publication.test.tsx components/settings/secrets-settings.workspace-publication.test.tsx hooks/domains/settings/use-secrets.lifetime.test.tsx components/settings/secrets-settings.lifetime.test.tsx hooks/domains/settings/use-secrets.test.ts components/settings/secrets-settings.test.ts --maxWorkers=1 --no-file-parallelism)
(cd apps/web && pnpm exec eslint hooks/domains/settings/use-secrets.ts hooks/domains/settings/use-secrets.workspace-publication.test.tsx components/settings/secrets-settings.workspace-publication.test.tsx --max-warnings=0)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:ratchet)
```

Light documentation checks from repo root, permitted during design:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.py specs --system workspaces --format paths
git diff --check
git status --short --untracked-files=all
```

For whitespace on untracked Markdown, validate file contents or compare each
new file against `/dev/null`; ordinary `git diff --check` alone omits them.
Check every local Markdown file link/anchor, valid statuses, one-order/all-AC
mapping, and the complete unfiltered tracked/untracked changed-path inventory.
Use `.github/scripts/pr-docs.cjs`'s `validateCoverage` with those actual paths and
the complete referenced file contents. At design time, four actual doc paths
are `exempt`; separately add the one planned production and two planned test
paths for a clearly labelled seven-path projected `covered` preflight. Expect
one changed work order, two declared requirement mappings, and errors=[].
After implementation, use actual production/test paths; never present the
design exemption or projected inventory as actual production coverage.

No E2E/build/broad verification is required: the
[shared data-only mobile exception](plan.md#rendered-workflow-and-mobile-parity)
applies. Component evidence proves the actual form-to-rendered-row workflow.
New copy or layout changes would require scope reconciliation first.

## Dependencies

No prior work order. ROOT's review/release and exclusive heavy grant are required
before implementation. Sibling PR 4398 owns the Global effect in the same hook;
integration is serial and must preserve its reviewed scope. Inspect only the
approved exact specification files read-only in its worktree.

## Risks

- Do not use the snapshot's membership, length, or field equality as freshness.
- Do not advance the ref inside a delayed React updater or add it/items to
  effect dependencies; both can defeat the intended fence.
- Do not let an old scoped success or finally settle the replacement lifetime.
- Existing rejection behavior remains intentionally outside qualified repair.

## Parallelism

`sequential`. Same primary only; no native delegates, persistent workers, new
sessions, or model switches.

## Inputs

- [Owning requirement](../../specs/workspaces/requirements/workspace-secret-catalogue.md#requirements),
  all four ACs, and existing repository-secrets .1/.2/.14 compatibility.
- [Owning design](../../specs/workspaces/system-design/workspace-secret-catalogue.md),
  preserving the sibling's reviewed Global extension.
- Permitted evidence and protected-fixture boundary in
  [the manifest](plan.md#evidence-and-assumption-check).
- Existing real provider/hook and component patterns in the four listed
  compatibility suites, read-only.

## Results

ROOT reviewed the full actual design package and explicitly released this same
primary under exclusive grant112. Independent qualified RED31268 actually
joined exit1: principal real rendered row-loss plus three callback ordering
failures, with ten controls passing and no setup/transport/unhandled error.
The initial RED91032 fixture mistake and its actual join are recorded in
[the manifest](plan.md#verification-results).

The minimal workspace-local mutation-generation fence preserves the complete
current list and still settles successful readiness. Global effect, workspace
catch, dependencies, cancellation, and existing callback mutation semantics
remain unchanged. All exact product checks passed: affected GREEN5197 joined
exit0 (six suites, 61 tests), scoped ESLint72599 (zero warnings), typecheck86624,
and staged-file i18n ratchet54052. One normal frozen install only; formatting
preceded scoped checks. Receipts/logs retain original argv, native frame/session
joins, exit results, and current owned-group absence under
`/tmp/kandev-child112-implementation-20261010/`.

All seven acceptance criteria have meaningful hook/component evidence and the
unchanged compatibility suites. No protected source or sibling fixture was
read/copied, and no backend, Global owner, value, copy, or layout contract changed.
Final documentation coverage uses all seven actual paths, replacing the
design-only projected preflight. Normal hooks and hosted delivery remain
separate receipts; `done` means local implementation, not publication or merge.
