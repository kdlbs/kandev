---
created: 2026-10-06
status: implemented
requirements:
  - REQ-OFFICE-LIVE-UPDATES-002
system_design:
  - ../../specs/office/system-design/live-updates-02.md
legacy_specs: []
---

# Implementation Plan: Preserve live Office health during snapshot loads

## Overview

Retain provider-health rows observed during an outstanding snapshot read while
hydrating unaffected response rows. One sequential work order owns the existing
health hook and its real-context HTTP/WS regression suite. Office live updates
owns this projection/publication contract; execution routing policy remains
with dynamic profiles. Extend the existing diagnostic requirement with only
`AC-OFFICE-LIVE-UPDATES-002.9` and `.10`, retaining `.1` through `.8`.

ROOT reviewed the four artifacts, ended the design checkpoint, then released
implementation in the same primary session with the exclusive global
local-heavy lease. The bounded hook change and targeted tests are implemented;
validation is recorded below. Publication, exact-head hosted CI/review and the
separate serial merge grant are tracked in the durable task plan. No delegation,
new session/tab, model/profile switch, backend or browser work occurred.

## Evidence and current source audit

Current base: `3cfb02ffb1162066df8ffd99ce8e3665438077d2` (PR4241 merged).
Current health hook blob: `baf302079fe9e67b3c5895a596cdf2de55c2225e`.
The prior [workspace-selection manifest](../office-diagnostics-workspace-refresh/plan.md)
and [completed order](../office-diagnostics-workspace-refresh/task-01-current-workspace-reads.md)
explicitly excluded HTTP/live writer arbitration. They stay unchanged.
The current hook has committed callback ownership and newest-request guards;
accepted success still calls `setProviderHealth(workspaceName, res.health ?? [])`
without inspecting health observed during transport. Do not claim the older
proof's hook blob is still current or that the predecessor fixed this race.

Accepted READ ONLY ROOT evidence, not replayed or altered:

- `/tmp/kandev-office-health-live-order-repro.test.tsx`, SHA256
  `05ae25be27fcae5bad7a4c9942a89ce3ab7a4c6087a466fb4c0b96018b0dff71`.
- `/tmp/kandev-root-office-health-live-order-proof-receipt.json` and
  `/tmp/kandev-root-office-health-live-order-proof.log`: actual joined handle
  `41134`, exit `1`, duration `3.12s`, two causal initial/manual failures and
  one ordinary snapshot/later-event passing control. The real hook,
  StateProvider/createAppStore, API client and registered WS handler ran;
  only fetch transport was replaced. No setup/teardown failure.
- Original proof base `8776877048ea029b8f861b4d658be719dfbadff2`, hook blob
  `98aa83c6650b5e16c7b757a6848b73abb4b24fc4`; WS blob
  `b0037b1cc016289114b9a135fe2d88aeba5706b1` remains current.
- `/tmp/kandev-root-office-health-live-order-current-main-audit.json` records
  ROOT's current-main audit and the distinct remaining publication cause.

| Current boundary | Verified behavior | Planned ownership |
| --- | --- | --- |
| `hooks/domains/office/use-provider-health.ts` | Per-instance committed selection and newest-read fences; unconditional accepted whole snapshot | Capture actual store rows per request; reconcile only accepted success |
| `components/state-provider.tsx`, `lib/state/store.ts` | Context owns real Immer store; nested providers reuse parent | Read the owning store with `useAppStoreApi`; no context/store change |
| `lib/state/slices/office/office-slice.ts` | `setProviderHealth` immediately replaces one workspace entry; `upsertProviderHealth` replaces/appends one full key | Preserve existing actions and single synchronous publication |
| `lib/state/slices/office/routing-types.ts` | Key is provider ID plus scope plus scope value; optional dates do not define total order | Direct triple equality or collision-safe tuple; no schema/revision addition |
| `lib/ws/handlers/office.ts` and `lib/ws/router.ts` | Registered health event extracts a new row; active-workspace filter, legacy fallback, invalid-payload rejection | Invoke actual registered handler in new tests; no handler/registration change |
| `lib/api/domains/office-routing-api.ts` through `office-extended-api` | GET `/api/v1/office/workspaces/:wsId/routing/health` via `fetchJson` | Keep API client real; replace only fetch in regression fixtures |
| `ProviderHealthCard` within `/office` | Mounts hook with active workspace; collapses display to provider severity under enabled routing config | No component change; display grouping is not the health row key |
| `ProviderRoutingPage` and `ProviderHealthBanner` | Mount hook with active workspace; banner consumes non-healthy rows | No page/banner/action/copy change; manual hook refresh is independently covered |

`useOfficeWorkspaceData` does not read provider-health snapshots. Both desktop
and phone `OfficePageNav` selection paths use existing workspace state.
The task-overview normalizer does not normalize health rows. There is no live
health-deletion event; no tombstone handling or generic event journal is needed.
The store's immutable untouched references support the narrow comparison.
Nearby task-PR hydration also reads the owning store at settlement, but its
cache/coordinator policies are not imported into this fix.

Assumption check: outcome, scope, proof acceptance, design boundary, no replay,
and later release gates are confirmed by ROOT. Row identity and all writers
above are source-verified. No material unresolved dependency requires a parent
question. This local publication amendment fits the current hook/store design;
no speculative ADR is needed.

## Scope

### In scope

- Provider-health-only request-start baselines and accepted publication using
  actual current-versus-start row identity/presence.
- Immediate one-setter publication and all prior selection/request fences.
- One real-hook/store/API/registered-handler suite with fetch-only replacement.
- Existing Office dashboard hydration fixture compatibility with the hook's
  owning-store API, preserving its three dashboard assertions.
- Minimal amendment of the existing [requirement](../../specs/office/requirements/live-updates.md)
  and [design](../../specs/office/system-design/live-updates-02.md#provider-health-snapshot-publication).

### Out of scope

Routing preview, run attempts, routing ownership/policy, generic coordinator,
global/cross-tab cache, cross-consumer latest-response arbitration, event
journal, universal server timestamps/revisions, polling, retries, store/action/
API/schema/backend redesign, new UI/copy/layout/mobile flow, browser/build/E2E,
unrelated docs cleanup or policy, and replay/alteration of ROOT proof.

## Technical approach

Read the [provider-health publication design](../../specs/office/system-design/live-updates-02.md#provider-health-snapshot-publication).
Each admitted refresh captures the actual store/workspace rows before GET;
accepted success reads current rows and protects each current row whose full
key was absent at start or whose reference changed. Substitute protected rows
for matching HTTP keys and append protected keys missing from HTTP. Keep all
unaffected HTTP rows and remove unchanged rows omitted from HTTP. An empty
response retains only protected rows. Call the unchanged setter once without
an intervening scheduling boundary. A later read captures a fresh baseline.

Keep row references intact, including through a sibling snapshot, and retain
the entire observed payload. Do not compare state severity, optional dates,
serialized values, or only provider ID. Same-value repeated WS observations
still replace object identity. Do not use render-stale health as the baseline.
Store replacement belongs in callback ownership/dependencies. Catch/finally,
empty selection, unmount, stale callbacks and A-to-B-to-A retain existing fences.

| Input/ordering | Accepted publication | Unsupported guarantee |
| --- | --- | --- |
| HTTP with no intervening row replacement | Ordinary complete snapshot, including empty | No absolute server freshness claim |
| WS full key changed/added during initial or manual GET | Current full row survives matching or omitted HTTP key | No severity/timestamp choice |
| Unchanged key beside a changed key | Unchanged HTTP row hydrates or is removed when omitted | No whole-response suppression |
| Same provider, different scope/value | Each triple reconciles independently | No provider-level collapse |
| Event before GET admission | Fresh HTTP may replace earlier observed row | No permanent WS priority |
| Event after publication | Normal immediate upsert | No replay of missed events |
| Other workspace or separate store | Baselines/results stay local; existing handler filter applies | No global/cross-tab order |
| Sibling same-store publication during GET | Changed current identities conservatively survive older request | No new cross-consumer latest-read policy |

## Tests

New file: `apps/web/hooks/domains/office/use-provider-health-snapshot-loads.test.tsx`.
Use the production hook, actual StateProvider/createAppStore and API client,
and `registerOfficeHandlers(store)["office.provider.health_changed"]`.
Stub only fetch. Emit complete WS payloads through the registered handler;
drive selection via `setActiveWorkspace`; observe hook state and real keyed
entries. Never mock state/actions/hook/handler or mirror a merge predicate.
Use deferred responses and explicit causal settlement, with no timing sleeps.

| Planned test/scenario | AC-OFFICE-LIVE-UPDATES-002 | Required observation |
| --- | --- | --- |
| `retains live health across delayed initial snapshot` and `... manual snapshot` under `Office provider health snapshot publication` | .9, .10 | Healthy HTTP arrives after actual degraded WS; full live row survives; different-provider HTTP row hydrates. These are the first causal REDs |
| `hydrates ordinary snapshots and applies later events` | .6, .9, .10 | Normal initial success, later WS, and fresh manual response with no intervening event remain usable; assert loading/error and actual fetch URL |
| `reconciles full health keys independently` | .9, .10 | Same-provider provider/model/tier and two values; only event-touched triple wins, other triples update from HTTP; include collision-prone string tuples |
| `keeps repeated and recovering live observations` | .9 | Multiple events to one key retain latest full metadata; healthy recovery can replace degraded during GET; repeated equal-state payloads still survive stale differing HTTP |
| `retains inserted and omitted live keys` | .9, .10 | New live key absent at request start survives both matching and omitted HTTP; unchanged existing/new HTTP keys hydrate; unchanged omitted key disappears |
| `accepts empty snapshots without erasing intervening live rows` | .6, .9, .10 | Empty array and existing `?? []` fallback retain touched keys only; subsequent fresh empty refresh clears earlier health, loading settles |
| `captures admission state before a render catches up` | .9, .10 | Synchronous real event then manual refresh in one act observes event at admission; no false permanent protection; during-read event still survives |
| `keeps live health on current failure and manual recovery` | .6, .9, .10 | Rejected fetch leaves current live data; error/loading settle; rerender does not retry; later explicit fresh read can recover |
| `fences older success and failure after newest reads` | .5, .6, .9 | Overlapping manual reads plus live event; newest success/failure governs; older success/rejection/finally cannot overwrite health or settle pending newest state |
| `isolates workspaces and application stores` | .3, .7, .9 | Matching keys in A/B and two independent real stores; A event survives A snapshot, does not protect B HTTP or change other store; out-of-workspace event ignored |
| `keeps pending same-store consumers independent` | .7, .9 | Both start before event; each settlement preserves live row and hydrates unaffected rows; unmount one cannot cancel sibling. No arbitrary newest-writer assertion |
| `fences departed selection and unmount after events` | .2, .4, .9 | Switch/null/A-to-B-to-A, stale callback, unmount, success/rejection, and StrictMode cleanup/setup; stale work never writes any departed entry or settles current state |

Names beyond the two REDs may be split into focused cases to respect lint
limits without changing the matrix. Assert whole live metadata and key
membership, not just severity/transport counts. Observe direct setter behavior
using the real store: replacement/empty clear is synchronous and a subscriber
sees one complete publication with no transient stale live row. This can live
in the same real-context suite; no setter signature change is planned.

Run the existing `office-diagnostic-workspace-reads.test.tsx` alongside the new
suite to preserve all 46 reviewed predecessor guard scenarios. The existing
store-action and WS-handler files are read-only inputs, not expanded runtime
scope. All deferred responses must resolve/reject and all refresh promises
must be joined during fixture cleanup, even after a failing assertion.

## Mobile and public documentation audit

Mobile parity applied. This fix changes state/data publication only inside
the shared hook, with no rendered layout, touch, scrolling, navigation, copy,
responsive branch, or mobile flow change. Real-context unit/component tests
satisfy that skill's narrow exception. No ASCII preview or new Playwright
scenario is needed; browser/build/E2E is outside the authorized scope.

Public-doc audit includes `docs/public/**`, README and `docs/screenshots.md`.
`docs/public/office-provider-routing.md` does exist: it explains configuration,
degradation/fallback and manual retry. `docs/public/websocket-api.md` lists
the health event. Neither defines snapshot/live publication precedence.
`feature-status.md` and README describe Office as evolving/in progress. This
fix changes no documented command, route, payload, classification, label,
workflow, screenshot or instruction; no public page edit is required. Do not
claim the provider guide is absent or rewrite its routing migration here.

## Work orders

- [x] [Task 01: Reconcile provider-health snapshots with observed rows](task-01-reconcile-health-snapshots.md) (`done`, sequential, no work-order dependency).

## Verification results

Design-only checks passed with actual terminal exit `0`: catalog validation
(351 decisions, 1369 specifications), all specification lint, and
`.github/scripts/pr-docs.cjs::validateCoverage` for the four actual artifacts
plus two anticipated implementation paths (`covered`, `ok: true`, no errors).
Coverage resolves the work order, plan, requirement/AC and design references;
this is documentation evidence, not runtime verification. Existing Node
`24.21.0` was resolved using command-local PATH without runtime edits.
Tracked diff and all four files, including untracked plan/order, passed
whitespace checks. Source/proof inspection was read-only; proof SHA256 matches.

Implementation used one frozen apps install with explicitly pinned
pnpm9.15.9. The exact initial/manual permanent regressions produced two causal
REDs: delayed healthy HTTP replaced degraded WS and its complete metadata in
the real workspace entry. The new suite exercises the real hook, context/store,
API client and registered handler, replacing only fetch transport.

Initial GREEN had 70 passes and one predecessor sibling expectation failure.
That health-specific expectation incorrectly removed a row published after the
sibling's admission; it now matches the reviewed conservative preservation
contract. All independent admission/loading/unmount assertions remain, with
routing-preview expectations unchanged. Final GREEN passed 73 tests in two
scoped files (27 new transport-path cases and 46 predecessor lifecycle cases).

Changed three-file ESLint, web typecheck including normal generation,
`i18n:check`, and baseline `i18n:ratchet` passed. The existing orphan-catalog
warning is informational; there were no copy/catalog changes. Documentation
catalog/lint passed, actual seven-file coverage returned `covered`, `ok: true`,
and no errors, and whitespace passed before delivery.
All command handles and exact outcomes are retained in the durable task plan.
The broader migrated requirement/design remain draft; no unrelated legacy
criterion is claimed complete. No browser/build/E2E, store/action/API/schema/
backend, routing-preview production or dependency/lockfile change occurred.

The first published candidate's Frontend Tests run failed all three existing
`OfficePageClient boot hydration` cases before their assertions: its
StateProvider module mock exported only `useAppStore`, while the real mounted
health card reached the hook's new `useAppStoreApi` dependency. The actual
stack and fixture establish an own-scope fixture contract regression; the
run's 27 health-publication and 46 lifecycle cases passed. The dependent
`Frontend Tests Passed` gate correctly failed too. There were no uploaded
failure artifacts; exact leaf logs and source comparisons are retained in the
durable task plan.

ROOT authorized the smallest test-only correction: one stable `getState()`
adapter over the existing page fixture state and its `useAppStoreApi` export.
All three dashboard assertions, production code and real transport regression
tests remain intact. CI provides meaningful RED; the exact anchored three
affected page tests passed after correction, and changed-file ESLint passed.
No install, typecheck, broad passing replay or CI rerun was used for this
correction. Normal hooks and new-head hosted gates remain delivery evidence,
tracked in the durable task plan.
Catalog/spec lint and whitespace passed after the correction. Documentation
coverage checked all eight actual changed paths and returned `covered`,
`ok: true`, with no errors.

## Risks and delivery gates

- Immutable row identity is verified in source. Later real-context tests must
  prove untouched rows hydrate and touched references survive actual store
  publication. Value equality or provider-only keys would silently break this.
- Comparisons preserve current replacements without attributing their writer;
  same-store HTTP sibling changes can be preserved conservatively. Later fresh
  reads may supersede prior live health. No global chronology is promised.
- No live deletion event exists. If a fix needs tombstones, revision protocol,
  backend changes or a generic coordinator, checkpoint ROOT and END WAITING.
- Missing managed dependencies are expected in this fresh worktree. Later use
  pinned pnpm9.15.9 and one conditional frozen install, only under ROOT's
  global local-heavy lease. Retain managed worktree/deps/shared caches.
- Record actual handles, logs, exits and causal failures. Own-scope fixture or
  lint corrections plus affected checks are allowed. Resource/timeout/
  transport/unknown/out-of-scope failures checkpoint ROOT without auto recovery.
- Later standing delivery is a ready normal PR after the released order passes;
  freeze SHA except real corrective findings. Active hooks remain enabled.
  Exact-head substantive configured CodeRabbit App347564 review must cover all
  changed files; ACK/progress is not review. At most one necessary full request
  follows an actual completed automatic skip/gap.
- Retain one normal all-terminal CI collector with PID/start/cutoff/log/handle;
  join it and prove PID gone before any ROOT-authorized replacement. No CI
  retry without ROOT's bounded workflow/job-name grant; no passing replay or
  moving-main-only rebase. Require all six actual required successes, successful
  product parent workflows, fresh exact-head complete/error-free state and
  no visible/hidden/actionable unresolved threads, changes requested or human gate.
- Separate serial ROOT MERGE lease is required. Use expected-head squash with
  no admin bypass, then independently verify actual merged SHA/tree/all owned
  blobs/remote inclusion. Completion includes all joins and only proven owned
  cleanup; publication alone is not completion. Preserve ROOT proof, managed
  resources and foreign refs. Full current recovery state belongs in the
  durable Kandev task plan; no promise of unattended continuation after stopping.
