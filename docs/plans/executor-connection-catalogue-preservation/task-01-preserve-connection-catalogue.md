---
id: "01-preserve-connection-catalogue"
title: "Preserve connection-save catalogue publication"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PROFILE-EDITOR-001
acceptance_criteria:
  - AC-EXECUTORS-PROFILE-EDITOR-001.21
  - AC-EXECUTORS-PROFILE-EDITOR-001.22
  - AC-EXECUTORS-PROFILE-EDITOR-001.23
  - AC-EXECUTORS-PROFILE-EDITOR-001.24
system_design:
  - ../../specs/executors/system-design/profile-editor.md
---

# Task 01: Preserve connection-save catalogue publication

## Summary

Independently reproduce the successful refresh's catalogue loss, then constrain
the shared connection-save hook's publication to eligible current target fields.
Retain server normalization and observed changes through PATCH/refresh, with
real rendered caller and task-start consumer proof.

## In scope

- `useSaveExecutorConnection`, its existing tests, one new rendered regression
  suite and its local helper, plus the four owning design-package artifacts.
- Invocation-local semantic name/config observation, target removal protection,
  current catalogue publication, refresh fallback and observer disposal.
- Real SSH and Remote Docker callers and actual task-start options as test
  consumers; their production code remains unchanged unless ROOT admits scope.

## Out of scope

- Backend, API adapters/contracts, store actions, event handlers, global stale
  policy, server-write ordering, other saves/refreshes, dependencies and UI design.
- Any protected ROOT candidate, child101/PR4382, foreign resources or cleanup.
- Implementation or check execution before explicit same-primary release and
  ROOT's local-heavy grant. No delegations, new sessions or model switches.

## Acceptance

1. Real transport-held creation/deletion tests fail causally on the unmodified
   hook; failed-refresh and normal-save controls pass. After correction, the
   mixed catalogue and actual eligible options retain every current unrelated
   entry and omit every removed entry, preserving all profiles/metadata.
2. Normalization reaches untouched target name/config fields; observed changes,
   clears, change-and-restore, target removal/replacement and profile-only changes
   follow the design ownership table on successful and failed refreshes.
3. Rendered callers retain trust/coordination and normalized fingerprint behavior;
   PATCH failure and awaited callback success/rejection preserve semantics.
   Every check below passes with actual joins and no unhandled errors or leaks.

## Implementation sequence

After release, mark this order `in_progress`. Author permanent regressions
independently without reading ROOT's protected candidate. Replace the existing
mock-only hook catalogue assertion with real-store/transport tests; do not
retain a test requiring whole-array replacement. Keep API adapter, StateProvider
and `createAppStore` real; use `fetchJson`'s external transport boundary or actual
fetch to hold PATCH/list/reload/test responses. Exercise real registered
executor/profile handlers while requests are pending, not manual fabricated
replacement actions. Add no production changes until the first causal RED run
has a passing failure-refresh control and zero setup/unhandled failures.

Implement the [field/membership design](../../specs/executors/system-design/profile-editor.md#connection-save-catalogue-publication)
in the hook, with small local helpers if needed for limits. Subscribe before
PATCH; remember semantic touched fields and target absence, map current state
synchronously on success/fallback, and dispose in `finally` before `onSaved`.
Retain list refresh and the void API. No global action/revision framework.
Finish the remaining regressions and exact checks, then record results in this
order and the manifest. Do not convert planned counts into evidence.

## Regression matrix

Hook/store cases belong in
`apps/web/hooks/domains/settings/use-save-executor-connection.test.ts`.
Rendered caller cases belong in new
`apps/web/hooks/domains/settings/executor-connection-catalogue-publication.test.tsx`,
with setup in new sibling
`executor-connection-catalogue-publication.test-helpers.tsx`.

| Named case | AC suffix | Required proof |
| --- | --- | --- |
| `successful refresh retains a created destination` | .21 | Admit real list request, WS-create owner with profile, settle stale list; current IDs and actual eligible options survive |
| `successful refresh does not resurrect a deleted sibling` | .21 | WS-delete existing sibling while list held; absent before/after settlement in catalogue and options |
| `failed refresh retains live membership` | .21/.23 | Passing RED control, list rejects after event; current memberships survive submitted target patch |
| `mixed executor and profile events survive refresh` | .21 | Real create/update/delete handlers, saved-owner and sibling profiles, renamed metadata and an empty-catalogue variant |
| `unchanged target adopts normalized name and fingerprint` | .22 | Submitted values differ from returned name/fingerprint; current target adopts server-read fields |
| `profile-only updates allow connection normalization` | .21/.22 | Saved-owner profile changed while held; profile survives and fingerprint normalizes |
| `target field transitions survive PATCH and refresh` | .22 | Changes in both transport windows; name/config key update, addition, deletion and change-and-restore persist; untouched fingerprint still normalizes |
| `removed targets are never restored or patched after reintroduction` | .21/.22 | Initially absent, deleted and deleted/readded same-ID cases, success and fallback; old result makes no target publication |
| `missing target and rejected refresh use submitted fallback` | .23 | Keep current membership, clear omitted unchanged config keys; protect observed name/config changes; no normalized-value claim |
| `PATCH rejection preserves live state and skips callback` | .24 | Held PATCH rejects after event, no GET/publication/callback, real coordinator failure and recoverable dirty draft |
| `onSaved is awaited once after publication` | .23 | Held callback sees published target and current choices; save promise waits; callback rejection propagates with no fallback or second call |
| `save observation is isolated and released` | .22/.24 | Two real stores, success, PATCH rejection, refresh rejection, callback rejection and unmount settlement; zero live owned observers afterward |
| `SSH page shows the normalized pin after coordinated save` | .22/.23 | Real route/card/provider/coordinator, test/trust/edit/save, held list and route reload; actual pinned fingerprint text and current options |
| `Remote Docker section retains config and normalized pin` | .21/.22/.23 | Real section/card with subscribed current target, real builder/remote test adapter, non-SSH config retained and fingerprint remount visible |

Include actual production executor-name/type fallback projection and
`useExecutorProfileOptions`, render `renderLabel`, and assert IDs, multiplicity,
labels, owner metadata and disabled eligibility. Preserve provider gates with
an unavailable-provider control. Normalization test responses are controlled
server-read fixtures; they do not claim a new backend normalization algorithm.

Mount real StateProvider/createAppStore and SettingsSaveProvider. Only external
transport and absent external editor rendering may be stubbed; no mocks of the
hook, callers, store, coordinator, handlers, API adapter, router, config builder
or options logic. Admit held requests explicitly and assert live changes before
settlement. Dispose/unmount trees, settle every held promise and save completion,
restore navigation/connection state and drain only owned timers on every exit.

## ASCII UI preview

Excerpt of [UI-01 / UI-02](plan.md#observable-desktop-and-phone-states), AC .21-.24:

```text
UI-01 desktop settings / phone direct settings:
  Connection: pinned SHA256:server-normalized
  Test connection -> Trust -> existing Save changes
UI-02 desktop dialog / existing phone task-start picker:
  retained current choices + new eligible choice
  current names and owner labels; deleted choices absent
```

Only data changes. Shared settings scroll owner, phone safe areas, touch sizing,
control order, task-start composition and localized copy retain their owners.
Rendered integration covers the state/data mobile-parity exception; no new
browser/E2E/build gate is imposed. If that boundary changes, stop for scope review.

## Verification

Run from repository root only after ROOT grants local-heavy execution. Retain
every returned native session handle and actually join it; no duplicate runs
to infer completion. This fresh managed workspace lacks `apps/node_modules`
and `apps/web/node_modules`. After the later explicit implementation release
and ROOT's local-heavy grant, install once from `apps/` before any package
command, as required by AGENTS.md:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

Do not install during the design turn. Preserve shared dependencies and caches.

First run the RED/controls subset before editing production:

```bash
(cd apps/web && timeout --signal=TERM --kill-after=10s 5m pnpm exec vitest run hooks/domains/settings/use-save-executor-connection.test.ts -t 'successful refresh retains a created destination|successful refresh does not resurrect a deleted sibling|failed refresh retains live membership')
```

Expected: exactly the two catalogue regressions fail for their causal
assertions, failure-refresh control passes, no collection/setup/unhandled error.
If a different failure occurs, correct the independent fixture first; never
present it as product RED.

Then run the complete affected suites and checks once after correction:

```bash
(cd apps/web && timeout --signal=TERM --kill-after=10s 10m pnpm exec vitest run hooks/domains/settings/use-save-executor-connection.test.ts hooks/domains/settings/executor-connection-catalogue-publication.test.tsx 'app/settings/executors/ssh/[executorId]/page.test.tsx' components/settings/ssh-connection-card.test.tsx components/settings/ssh-connection-card.remote-docker.test.tsx components/settings/remote-docker-connection-config.test.ts components/task-create-dialog-options.test.tsx)
(cd apps/web && timeout --signal=TERM --kill-after=10s 3m pnpm exec eslint --max-warnings 0 hooks/domains/settings/use-save-executor-connection.ts hooks/domains/settings/use-save-executor-connection.test.ts hooks/domains/settings/executor-connection-catalogue-publication.test.tsx hooks/domains/settings/executor-connection-catalogue-publication.test-helpers.tsx)
(cd apps/web && timeout --signal=TERM --kill-after=10s 10m pnpm run typecheck)
(cd apps/web && timeout --signal=TERM --kill-after=10s 3m pnpm run i18n:check)
(cd apps/web && timeout --signal=TERM --kill-after=10s 3m pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/executor-connection-catalogue-preservation
```

No backend suite, build or Playwright run is required by the state-only change.
No passing broad-suite replay. Re-run affected checks only for a new change,
failure or material unresolved concern, within ROOT's grant.

Documentation coverage preflight uses actual tracked/untracked paths; it does
not substitute a hypothetical list. Run from repository root after checks:

```bash
timeout --signal=TERM --kill-after=10s 60s node <<'NODE'
const fs = require('node:fs');
const cp = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const base = '64c34321d2d3843851116f87fec1a5b8b48cd32e';
const tracked = cp.execFileSync('git', ['diff', '--name-only', '-z', base], { encoding: 'utf8' }).split('\0');
const untracked = cp.execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], { encoding: 'utf8' }).split('\0');
const paths = [...new Set([...tracked, ...untracked].filter(Boolean))];
const docs = [
  'docs/specs/executors/requirements/profile-editor.md',
  'docs/specs/executors/system-design/profile-editor.md',
  'docs/plans/executor-connection-catalogue-preservation/plan.md',
  'docs/plans/executor-connection-catalogue-preservation/task-01-preserve-connection-catalogue.md',
];
const fileContents = Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, 'utf8')]));
const changedFiles = paths.map(filename => ({ filename, status: fs.existsSync(filename) ? 'modified' : 'removed' }));
const result = validateCoverage({ changedFiles, fileContents });
process.stdout.write(JSON.stringify(result, null, 2) + '\n');
if (!result.ok || result.errors.length || result.status !== 'covered' || result.workOrders.length !== 1) process.exitCode = 1;
NODE
```

The design-only diff would be `exempt`, not product proof. If foreign paths enter
the actual diff, preserve them and report the coverage/scope blocker to ROOT;
do not conceal them by filtering. Reconcile referenced documents/IDs before
marking done. No new ADR: this local observation implements existing store
ownership, and its rationale is captured by the owning design. Public docs
assessment: executor guide, README and screenshots retain existing user steps;
this restores those outcomes without new public instructions or terminology.

## Files likely touched

- `apps/web/hooks/domains/settings/use-save-executor-connection.ts`
- `apps/web/hooks/domains/settings/use-save-executor-connection.test.ts`
- `apps/web/hooks/domains/settings/executor-connection-catalogue-publication.test.tsx`
- `apps/web/hooks/domains/settings/executor-connection-catalogue-publication.test-helpers.tsx`
- `docs/specs/executors/requirements/profile-editor.md`
- `docs/specs/executors/system-design/profile-editor.md`
- `docs/plans/executor-connection-catalogue-preservation/plan.md`
- `docs/plans/executor-connection-catalogue-preservation/task-01-preserve-connection-catalogue.md`

## Dependencies

None. Companion profile edit/create/policy packages are already documented;
their existing scopes and results remain intact. This order does not authorize
altering their implementations or replaying their checks.

## Risks

- Observer start/disposal, per-key absence handling and profile-only updates
  must be independently tested; object identity is insufficient evidence.
- Live changes seen before PATCH completion include the save's own echo; retain
  those delivered values rather than trying to classify event ownership.
- Callback rejection must remain outside refresh fallback. Server normalization
  is available only from delivered events or the successful target read.
- Additional caller production changes require concrete evidence and ROOT's
  scope checkpoint; no adjacent refresh correction is preauthorized.

## Parallelism

`sequential`. No native delegation authorized.

## Inputs

- AC-EXECUTORS-PROFILE-EDITOR-001.21 through .24 and paired design's Connection
  save catalogue publication and existing Current catalogue publication sections.
- Shared hook, both active callers, settings API, registered executor/profile
  handlers and `task-create-dialog-computed.ts`/`task-create-dialog-options.tsx`.
- Existing connection-card save tests and policy publication rendered-fixture
  patterns, as read-only examples; do not copy ROOT's protected candidate.
- ROOT qualified-proof metadata and [manifest admission/delivery gates](plan.md).

## Results

Implemented in this primary after ROOT's explicit release and exclusive
local-heavy grant. Production changes are limited to the shared hook; both
caller production files, API adapters, store and registered handlers are unchanged.
All commands completed with actual joins. Receipts and current operational
state are external at `/tmp/kandev-child102-connection-design-20261010/`.

| Check | Actual result |
| --- | --- |
| `pnpm install --frozen-lockfile`, cwd `apps/` | Exit 0; native35124 joined fed778; one install, existing cache reused |
| Exact planned creation/deletion/failure-refresh RED subset | Exit 1; native86618 joined f2a33b; two causal failed cases, failed-refresh control passed, no setup/unhandled errors |
| Independent broader hook RED before production changes | Exit 1; native24433 joined 6f54a6; 18 failed, 5 passed; field/profile/lifetime assertions exposed expected old behavior |
| Hook GREEN after correction | Exit 0; native55956 joined 6b744b; 23 passed |
| Initial real rendered callers | Exit 0; native17816 joined 4e1b0e; 4 passed |
| Exact seven-suite affected command | Exit 0; native61453 joined 3ebcc9; 57 passed across 7 files |
| Final two changed suites after fixture corrections | Exit 0; native86800 joined d4c9d3; 31 passed across 2 files (27 hook, 4 callers) |
| Exact scoped ESLint | Exit 0; native26915 joined f0a160 after final helper correction; no warnings |
| `pnpm run typecheck`, cwd `apps/web` | Exit 0; native95560 joined d16ded; pretypecheck generated required ignored release-note/changelog artifacts |
| `pnpm run i18n:check` | Exit 0; native76664 joined 395135; existing orphan-key notices, no errors |
| `pnpm run i18n:ratchet` | Exit 0; native82708 joined a26453; new-code and allowlist checks passed |
| Python documentation catalog and specification lint | Exit 0; 368 decisions and 1496 specifications validated; all specs passed |
| Actual tracked/untracked documentation coverage preflight | Exit 0, 82aef4; eight paths, `covered`, errors empty, exactly one work order |
| Scoped/whole diff whitespace checks | Exit 0 |

Scoped ESLint initially rejected test group lengths, repeated literals and
transport branching; the fixture corrections passed the affected test suites
and scoped lint. Direct `tsc --noEmit` initially exposed circular fixture typing,
required provider children and missing fresh-workspace generated artifacts.
The first scripted correction failed before file writes, so its already-started
check retained the old fixture diagnostics; all original handles were joined.
Explicit fixture typing and a real JSX provider wrapper corrected those issues.
The verification command now uses the repository's `pnpm run typecheck` with
its required pre-generation. A first root-environment coverage attempt could
not find Node (exit127); the same root-cwd script passed with Node resolved
from the package environment. No failure is promoted into success evidence.

The rendered tests preserve actual API adapters, hook, store, event handlers,
coordinator, trust controls, both callers and production task-start options.
Only external HTTP is controlled. They verify normalized pins, latest target
fields/clear/restoration, delivered own-save echo, profiles and mixed membership,
missing/deleted/reintroduced targets, fallback, provider eligibility, callback
semantics, observer release, separate providers and recoverable rendered drafts.

No browser build or new Playwright test was required by the reviewed state/data
mobile-parity boundary. Existing desktop/phone composition and public executor
steps remain unchanged. Commit hooks, hosted CI/reviews and ROOT's separate
expected-head merge grant remain delivery gates, not inferred from local GREEN.
