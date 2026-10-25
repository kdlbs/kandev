---
created: 2026-10-22
status: done
requirements:
  - REQ-UI-SETTINGS-MANUAL-SAVE-001
system_design:
  - ../../specs/ui/system-design/settings-manual-save.md
legacy_specs: []
---

# Implementation Plan: Linear settings refresh ordering

## Overview

Preserve acknowledged connection config/health against pending reads, then prove
ordinary draft and later refresh behavior through real consumers. Source-first
parent evidence at immutable base `36a12f5ff99e4095116a823d937fef2a205a671a`
proved old 200 and 204 regressions, with old 500 control passing.

## Scope

Only mounted Linear settings publication, initial seeding, and lifecycle fencing.
Exclude backend, credentials, API/schema, other integrations, watchers, teams
redesign, shared Query migration, global resources, and new revision frameworks.

## Technical approach

Adapt Jira's bounded local owner/sequence pattern into
`hooks/domains/linear/use-linear-config-publication.ts`. Gate every config-read
publication and fence pending reads only at accepted mutation ACK. Keep separate
Linear baseline/form semantics. Start the existing interval after initial load.

### UI-01: Linear connection after Save and stale read settlement

Entry: `/settings/workspaces/:workspaceId/integrations/linear`.
Existing shared composition on desktop and phone:

```text
Linear connection
[Accepted auth health banner]
API key [saved mask / raw draft]
Default team [selected team]
[Test connection]  [Remove configuration]
           [Reset] [Save changes] (only while dirty)
```

Before: an older GET can clear saved mask/removal or revert health after Save.
After: accepted metadata remains until a legitimate later read. Phone entry is
its dedicated settings page; Remove opens the shipped bottom sheet. Reuse the
single settings scroll owner and fixed safe-area Save surface. Structure and
state are required; spacing is illustrative. No new responsive layout. Save and Remove disable conflicting actions while
a same-route mutation is pending.

## Tests

`linear-settings-refresh.test.tsx` maps AC-UI-SETTINGS-MANUAL-SAVE-001.4 to
transport-controlled real-provider/coordinator/API tests: stale 200/204/500,
overlapping reads, Save/Remove during reads, failure, raw draft/canonical ACK,
Reset/next Save, slow initial load, workspace/unmount, and StrictMode.

## E2E tests

`linear-settings-refresh.spec.ts` (chromium) and
`mobile-linear-settings-refresh.spec.ts` (mobile-chrome) prove the same AC with
real disposable backend/mock Linear and captured pre-save response transport.
Own unique workspace/repo and explicit Worktree executor; guard seed and owned
source HEAD/full raw porcelain, aggregate cleanup failures, remove owned data.
Capture and inspect shipped desktop and phone screenshots.

## Work orders

- [x] [Task 01: Preserve saved config](task-01-preserve-saved-config.md)

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/linear/linear-settings-refresh.test.tsx components/linear/linear-settings.test.tsx lib/api/domains/linear-api.test.ts --maxWorkers=1)
(cd apps/web && GOMAXPROCS=2 GOFLAGS=-p=2 pnpm e2e:run --host --shards 1 --project chromium tests/integrations/linear-settings-refresh.spec.ts --retries=0)
(cd apps/web && GOMAXPROCS=2 GOFLAGS=-p=2 pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome tests/integrations/mobile-linear-settings-refresh.spec.ts --retries=0)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/linear/linear-settings.tsx components/linear/linear-action-bar.tsx hooks/domains/linear/use-linear-config-publication.ts components/linear/linear-settings-refresh.test.tsx e2e/tests/integrations/linear-settings-refresh-helpers.ts e2e/tests/integrations/linear-settings-refresh.spec.ts e2e/tests/integrations/mobile-linear-settings-refresh.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```
## Initial checkpoint verification results

- Permanent real-consumer/API suite: 49 passed across three files (21 refresh
  ordering/lifecycle cases, 4 existing removal cases, 24 API cases), one worker.
  Final command ran after all production and test corrections.
- Corrected pre-fix permanent RED: 11 expected assertion failures and 6 passing
  controls. Parent RED remains 2 expected failures and 1 passing 500 control.
  Initial mixed test-scaffold failures are retained separately, not causal RED.
- Browser RED: desktop accepted-health reversion and phone old-204 removal loss
  both failed as intended on the pre-fix production build.
- Fresh managed desktop browser: 1 passed (4.0s test, 7.0s total). Phone on the
  same unchanged build: 1 passed (4.7s test, 7.4s total). One worker/shard,
  retries 0, GOMAXPROCS=2, GOFLAGS=-p=2; both screenshots inspected.
- Unique disposable workspace/repository/Worktree executor cleaned up. No tasks
  or sessions started. Seed, source, and owned repository HEAD/full raw porcelain
  guards passed. All four backend fixture directories absent after teardown.
- ESLint with zero warnings and Prettier checks passed for all six changed TS files.
- Full frontend typecheck passed with the documented 8GB Node heap. Default-heap
  exhaustion and an intermediate test-only type-option failure are separately
  retained in the external receipts. No product incident claim.
- Documentation catalog: 397 decisions and 1712 specifications validated.
  Full specification lint and PR-documentation coverage preflight passed.
- Public-doc assessment: existing workspace credential/manual Save/blank-key
  contract is unchanged; no public guide or screenshot catalog changes needed.
- Duplicate target patches refreshed after causal proof: #1512 has 895 files,
  Query config writes without pending-read cancellation/admission fencing;
  #4375/#4584 have no target changes; #2336 changes watcher tests only.
  All four heads remained unchanged at the final metadata refresh.
- Diff whitespace and package cross-references checked. Source commit and active
  hook receipt are supplied in the external parent-validation packet. No push/PR
  is authorized at this checkpoint.


## Risks

Initial seeding must remain authoritative despite slow transport. Read sequence
must never reject a successful write ACK. Failed writes must not fence reads.
Public docs retain their existing Save/blank-key contract; no new user operation.

## PR review remediation

Greptile identified the required domain-hook location. CodeRabbit identified
concurrent same-route Save and Remove admission. Move the unchanged publication
hooks to `hooks/domains/linear/`; use the existing coordinator `runExclusive`
for confirmed Remove, preventing overlap with Save and other Remove submissions.
Disable Remove and its confirmation while a mutation owns the route gate.
Keep raw drafts, retry behavior, accepted-write read fences, and existing
confirmation compositions. Add real-transport RED controls and native desktop/
phone busy-action proof before the bounded correction. Reuse this one work order.
Prior checkpoint results remain historical; new-head checks, assets, reviews,
and exact-main combined verification must be refreshed before merge.

## Review remediation verification

- Two additional real-consumer mutation admission assertions failed before the
  correction; all 21 existing refresh controls passed. The initial mixed timeout
  attempt is retained separately from the corrected two-assertion RED.
- Six focused suites passed: 86 tests, including 51 Linear publication/removal/API
  cases and coordinator/exclusive-action/enablement compatibility controls.
- Desktop and native phone each failed the pending-Save Remove-button assertion
  on the prior production build, then passed on the rebuilt correction. Both
  prove Save is disabled during confirmed Remove and allow ordinary subsequent
  operations. One worker/shard, retries zero, normal resource guards.
- Changed-file ESLint, frontend typecheck with the documented 8GB heap, and
  documentation catalog/spec lint passed. Final exact-head combined-tree,
  hosted review/CI, and publication evidence is retained externally; it is
  refreshed after this commit rather than represented by historical receipts.

## Remove settlement draft preservation

The current-head semantic review found that successful Remove cleared edits
entered after DELETE admission. Preserve the later form as dirty against the
new null baseline; clear the form only when it still matches the submitted
snapshot. Keep Reset and next Save behavior, accepted-read fences, and mutation
exclusion. Add real-consumer RED plus desktop/phone value assertions before
changing production. Prior verification is historical after this correction.

### Remove settlement verification

- Two real-provider/API assertions failed at the erased draft before the fix;
  all 23 existing cases passed. Both desktop and phone reproduced the same
  lost value on the prior build, with retries zero.
- Final six focused suites passed 88 tests, including 53 Linear publication,
  removal, and API cases. Both direct next Save and Reset-after-removal paths
  preserve the accepted baseline and canonical ACK behavior.
- Final rebuilt desktop and phone each passed with the later draft retained,
  dirty, and eligible for Save after Remove. Fixture cleanup and full raw
  source/seed guards passed. Typecheck (8GB), zero-warning ESLint, formatting,
  and documentation catalog/spec lint passed.
- The shared local accepted-form updater is used by Save and Remove; this
  keeps the action hook within the existing function-size limit without a
  new framework. Exact-head combined checks, assets, CI, and semantic-review
  results remain externally tracked and are refreshed after commit.

## Initial-load replacement and Test admission

Validate the current-head review's locale-triggered replacement load with the
real i18next producer. Only the active initial-load effect may finish loading, so an
older same-owner completion cannot start polling before seeding. Also prevent
new Test probes while Save or Remove is pending, preserving ordinary probes.
This covers admission during a mutation, not probes already pending before it.
Use real transport RED controls and native desktop/phone disabled-action proof
before the minimal guards; retain this single package.

The initial loading lifetime is distinct from config-read admission: an accepted
write can fence a pending initial result without stranding its loading state.
Validate that interaction before selecting the minimal effect-lifetime guard.

### Prior-head initial lifetime and probe verification

- On the prior head, three real-consumer tests failed and 25 controls passed.
  Real locale replacement admitted two initial reads; the older completion
  started the actual timer early and the replacement could not seed the mask.
  New Test requests were observed during held Save and Remove. Desktop and
  phone both failed the pending-mutation Test disabled assertion, retries zero.
- A current-request-only loading candidate failed an additional write-ACK
  control (1 failure, 28 passing): accepted Save fenced the replacement data
  and stranded loading. The final effect-local active lifetime fixes both
  initial-load cases while config/error publication retains sequence admission.
- Final six focused suites passed 92 tests, including 57 Linear consumer/API
  cases. Ordinary Test probes work before and after mutations. Probes already
  pending before a mutation retain their existing behavior.
- Fresh rebuilt desktop and native phone each passed all shared causal flow
  and mutation-control assertions, retries zero; fixture/source/seed guards
  and cleanup passed. Full 8GB typecheck, zero-warning lint, formatting, and
  documentation checks passed. Exact-head/current-main combination, fresh
  assets, hosted checks and semantic review are refreshed externally.

## Locale changes preserve the initial-seeding lifetime

A later real locale change can rerun the current initial loader and clear a
raw draft, including on the proof base. Keep one initial-load lifetime per
mounted workspace. Reuse the repository's native React effect-event pattern
for current-locale error feedback, so translation identity does not restart
config/form seeding. Revise locale controls to assert no new GET admission;
keep workspace/StrictMode replacement controls. Prove dirty draft and localized
error behavior with real i18next/API RED before production. No new seed or
revision framework is needed.

### Locale-lifetime verification

- Four intended real-consumer RED tests failed and 27 controls passed on the
  prior head, including a raw draft erased by the actually admitted locale
  read. The final producer admits no locale-triggered config read; revised
  controls explicitly assert that absence instead of releasing invented reads.
- Six focused suites passed 94 tests, including 59 Linear consumer/API cases.
  Dirty draft and Reset baseline survive locale changes, pending initial loads
  still seed once, accepted Save stays authoritative, and pending-load errors
  use the current locale. Existing workspace/StrictMode controls still pass.
- The only new callback is the repository-established native React effect
  event for localized load errors. Workspace identity owns initial seeding;
  config publication keeps its local read/ACK fence. No seed/revision framework.
- Fresh rebuilt desktop and native phone each passed the full shared scenario
  with retries zero. Fixture/source/seed guards and cleanup passed. Typecheck
  (8GB), zero-warning lint, format, and docs checks passed. New-head combined,
  assets, hosted checks and thread/review evidence remain externally tracked.
