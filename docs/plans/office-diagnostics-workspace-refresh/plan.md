---
created: 2026-10-06
status: implemented
requirements:
  - REQ-OFFICE-LIVE-UPDATES-002
system_design:
  - ../../specs/office/system-design/live-updates-02.md
legacy_specs: []
---

# Implementation Plan: Refresh Office diagnostics on workspace selection

## Overview

Make the two existing diagnostic hooks automatically read their current
workspace after a switch and prevent obsolete requests from publishing. One
sequential work order owns both hooks and their real-store regression tests.
ROOT reviewed all four artifacts and released implementation in this same
primary session/profile. The correction and permanent tests are implemented;
scoped verification is recorded below. CI, review, and serial merge gates are
tracked separately in the durable task plan.

Office live updates owns the read lifecycle and projections. Execution
routing policy remains with dynamic profiles; the deprecated Office routing
specification is not revived. No new ADR is needed for this local correction
inside existing hook/store boundaries.

## Evidence and consumer audit

Baseline: `0eb74e57332900b7bd5ca756c541d16170000492` (merged Discard).
Source blobs match ROOT's evidence: preview
`8b89eef6222c506c7c8e3bafd2359412d15a1cf3`, health
`98aa83c6650b5e16c7b757a6848b73abb4b24fc4`.
Both hooks set one instance-wide `fetched` boolean after success; the automatic
effect then skips every later workspace in that mounted instance.

Accept ROOT's read-only proof without replay:
`/tmp/kandev-office-workspace-switch-repro.test.tsx`, SHA256
`0b39f1744b85c62f89de39285f8f3cc31c8b32ddc108c5017c7e2a5e95df48b3`;
`/tmp/kandev-root-office-workspace-switch-proof-receipt.json` and
`/tmp/kandev-root-office-workspace-switch-repro-correct-cwd.log`.
Handle 18610 was actually joined, exit 1: two causal REDs for missing beta
reads after alpha completed, and two passing initial/explicit-refresh controls.
The earlier wrong-cwd failure executed no tests and is not causal evidence.
The archive checksum was verified here; preserve all ROOT proof files.

| Actual consumer | Selection and visible use | Existing mitigation/limit |
| --- | --- | --- |
| `ProviderRoutingPage` at `/office/workspace/routing` | `workspaces.activeId` goes to both hooks; preview table and health banner consume the selected results | Save explicitly refreshes preview; health events may update health independently |
| `AgentsPageClient` at `/office/agents` | Mounts preview hook; `AgentCard` selects preview by current workspace and agent ID | Routing chip requires enabled config and a matching preview; agent-list refresh does not load preview |
| `ProviderHealthCard` within `/office` | Mounts health hook with active ID, reads existing selected routing config | Card requires enabled config; health events independently upsert data |
| `StateProvider` / `createAppStore` | One real context/store; nested providers reuse parent; diagnostic actions write workspace-keyed entries | Data ownership already correct by key; completion latch and request-state lifetime are the defect |

`src/spa-routes.tsx` renders the Office route without a workspace key;
`src/office-routes.tsx` still registers these consumers while Office is enabled.
`OfficeShell` exposes desktop and phone workspace pickers.
`useOfficeWorkspaceData` reads agents/projects/inbox/meta, not diagnostics.
`office.provider.health_changed` can mitigate health symptoms. This evidence
proves missing automatic reads, not that entire dashboards always stay stale.
The existing backend diagnostic endpoints remain registered; this package
does not change them or claim a new routing-policy contract.

No existing companion implementation package owns the new diagnostic-read
criteria. The only pre-existing live-update requirement references found in
plans are the unrelated prompt-attachments documentation audit; its scope
and status are unaffected.

## Scope

### In scope

- Per-hook selection-driven automatic reads, request lifetime guards, and
  current loading/error ownership.
- Existing manual refresh and workspace-keyed store publication.
- One transport-only real-hook/real-store suite covering both hooks.
- Minimal amendments to [Office live-update requirements](../../specs/office/requirements/live-updates.md#req-office-live-updates-002-current-workspace-diagnostic-reads)
  and [design](../../specs/office/system-design/live-updates-02.md#current-workspace-diagnostic-reads).

### Out of scope

Generic Office coordinator, global cache or dedupe, shared request arbitration,
store/API/backend/schema changes, provider registry, routing policy, runner,
broadcast changes, unrelated routing-configuration/draft defects, layout,
copy, new polling/retry, and HTTP-versus-live-event races. No production or
permanent test changes, dependency installation, browser, build, E2E, commit,
or PR work in this design turn.

## Technical approach

Follow the design's selection and request lifetime in
`use-routing-preview.ts` and `use-provider-health.ts`. Remove the `fetched`
latch; drive automatic admission from workspace/callback identity. Use
instance-local ownership and request identities to fence success, catch,
finally, switch-to-empty, A-to-B-to-A, and unmount. Keep stable selector empty
arrays, current cached data, existing actions, result types and translations.
The nearby `useWorkspaceRouting` demonstrates scoped refresh sequencing, but
its mock-store tests are not adequate proof for this work. The accepted ROOT
test and `use-inbox-history-controller.test.tsx` demonstrate the real context
boundary. Do not extract a generic coordinator or mirror the implementation
in helper-only tests.

## Tests

Implemented file:
`apps/web/hooks/domains/office/office-diagnostic-workspace-reads.test.tsx`.
Each scenario exercises both production hooks with their own typed payloads.
Mock only `getRoutingPreview` and `getProviderHealth` at the existing transport
module; never mock StateProvider, createAppStore, selectors or store actions.

| Scenario/test name | Criteria | Observable evidence |
| --- | --- | --- |
| `loads current workspace and permits explicit refresh` | .1, .6 | Correct alpha transport and real entry, rerender has no extra call, explicit refresh reads alpha |
| `does not read with no workspace` | .2 | Null and empty selection expose empty/false/null; explicit refresh makes no call; later alpha loads |
| `treats an empty response as completed` | .1, .6 | Empty entry is accepted, loading settles, rerender creates no loop |
| `loads beta after alpha completes` | .1, .3 | Actual `setActiveWorkspace` switch causes beta read and beta payload publication; alpha entry remains separate |
| `refreshes a cached workspace on return` | .1, .3 | A-to-B-to-A reads anew, cached A data can be used during pending read |
| `ignores departed success while beta is pending` | .3, .4 | Deferred alpha succeeds after beta admission; neither beta loading nor any store entry is changed by alpha |
| `ignores departed failure while beta is pending` | .3, .4 | Alpha rejection cannot set beta error or settle beta loading; repeat settlement after beta completes |
| `invalidates pending reads on empty selection and unmount` | .2, .4 | Resolve/reject departed read; real store snapshot unchanged and empty state neutral |
| `does not revive an old alpha read after returning to alpha` | .4, .5 | A-to-B-to-A deferred old A cannot replace accepted new A or settle its pending state |
| `retains current data on failure and recovers manually` | .3, .6 | Current rejection retains cached entry, settles error/loading, no rerender retry, explicit refresh recovers |
| `newest refresh wins with reversed settlement` | .5, .6 | Two manual reads overlap; older success/failure cannot publish or clear newest loading/error; newest failure keeps prior data |
| `keeps mounted instances and stores independent` | .7 | Two instances both admit initial/current reads; unmount one leaves sibling valid; separate stores with the same key retain distinct payloads |
| `StrictMode replay accepts setup after cleanup and ignores the precleanup read` | .4, .8 | Real root StrictMode admits the second setup read; precleanup success cannot publish or settle it |

The hook suite observes the full affected hook-to-store path, independent of
viewport. It is the scoped regression evidence for `.8` as well. Deferred
promises must all be settled and renders cleaned up, even on failures. Use
distinct sentinels for prior/current/newest data and assert actual store entries
plus returned state, not just transport counts or setter spies.

## Mobile and rendered assessment

State-only change: no markup, layout, touch behavior, scroll, navigation,
responsive branch, or copy is modified. Desktop selection and the existing
phone `OfficePageNav` picker share `workspaces.activeId` and these hooks.
The mobile-parity state/data exception permits the real-hook/component tests
and this explicit assessment instead of a new mobile Playwright test. No
rendered preview or browser/E2E run is scheduled; browser/build/E2E work is
also explicitly prohibited in the design phase.

## Public documentation audit

Searched `docs/public/**`, root `README.md`, and `docs/screenshots.md` for
Office/routing/diagnostic terms. README describes Office as in progress;
`docs/public/feature-status.md` describes the gated routes and mutable routing
surface, and dynamic profiles separately. No published diagnostic-refresh
instructions or screenshot/control change needs correction. Internal docs are
updated; no public-doc change is needed for this bounded read-lifecycle fix.

## Work orders

- [x] [Task 01: Correct both mounted diagnostic read lifetimes](task-01-current-workspace-reads.md) (`done`, sequential, no dependencies).

## Verification results

The real-store settled-switch regression produced two causal REDs before
production edits. The expanded baseline suite had 28 semantic failures and
18 passing controls. Both hooks now guard all publication with committed
callback ownership and an instance-local newest-request generation.

GREEN: 46 tests passed in the one-worker `browser-locales` project. Changed
lint, web typecheck (including normal generation), `i18n:check`, the baseline
i18n ratchet, and documentation catalog validation passed. A test registration
length warning was corrected by splitting suites; scoped tests and lint passed.
An additional empty-selection store assertion passed in the final 46-test run.
Specification lint passed, and actual-path documentation coverage returned
`covered`, `ok: true`, and `errors: []` for the seven changed files.
All completed command handles are joined; the ROOT global local-heavy grant
remains held until normal hooks/publication finish. Exact receipts and resource
identities are in the durable task plan. The broader migrated specs remain
draft; this package does not claim their unrelated legacy criteria completed.

## Risks and delivery gates

- Guard all publication branches, not just successful data; cleanup must
  invalidate before late settlements, and repeated workspace strings do not
  prove the same selection lifetime.
- Latest-response guarantees stop at one hook instance. Shared-cache/event
  arbitration would expand scope and requires a ROOT checkpoint.
- The deprecated Office routing policy stays deprecated. Dynamic-profile
  migration/removal is separate work; do not expand this correction to it.
- The one conditional frozen pnpm9.15.9 install completed after release;
  retain the managed dependencies and worktree.
- ROOT owns ONE GLOBAL LOCAL-HEAVY slot and serial MERGE. Join every owned
  handle; checkpoint ROOT on timeout/resource/transport/out-of-scope/unknown CI
  problems, with no automatic retry budget.
- Later delivery retains active hooks, exact published SHA, all six required
  contexts and product parent-workflow SUCCESS, full exact-head review evidence,
  and a separate ROOT expected-head squash MERGE grant. Detailed standing
  authorization and crash-recovery state remain in the durable task plan.
