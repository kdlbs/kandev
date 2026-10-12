---
created: 2026-10-02
status: implemented
requirements:
  - REQ-WORKSPACES-LOCAL-REPOSITORIES-002
  - REQ-WORKSPACES-LOCAL-REPOSITORIES-003
system_design:
  - ../../specs/workspaces/system-design/local-repositories.md
legacy_specs: []
---

# Implementation Plan: Repository Discovery Ordering

## Overview

Preserve current repository choices when a cached discovery read and a refresh
scan overlap. One sequential work order repairs publication ownership in the
existing browser-tab coordinator and proves the result through deferred
coordinator tests and a real hook/consumer integration. Workspaces owns the
local discovery contract; backend scanning and UI presentation stay within
their existing boundaries.

## Evidence and assumption check

Local HEAD and authoritative remote main were both
`87dcd788a8f43faa72445bd35c0e86b270e2ebfd` before design. This is distinct from
the merged branch-list hook repair in PR #4171.

The parent exercised the real exported `RepositoryDiscoveryCoordinator`, with
only `RepositoryDiscoveryClient` deferred. A pending `load()` followed by
`refresh()` ended with `/old` when the newer refresh resolved `/new` first.
A pending `refresh()` followed by `load()` ended with `/removed` when the newer
snapshot resolved `/selected` first. Both ordering assertions failed; two
concurrent loads shared one snapshot and passed. The parent runner joined
session 65595, exit 1 (test 11 ms, total 2.80 s). Its temporary in-tree test was
removed. The accepted fixture at
`/tmp/kandev-discovery-ordering-repro.test.ts` was inspected, retained, and not
rerun. Permanent RED/GREEN evidence remains required during implementation.

`CoordinatorEntry.snapshotPromise` and `refreshPromise` deduplicate each kind.
`startSnapshot` and `startRefresh` both publish response/error unconditionally.
Snapshot `finally` can schedule a refresh from shared state without request
ownership. Pending handles are currently assigned after subscriber notification.

AC-003.5, 003.6, and 003.7 already require manual refresh, sharing, and failure
preservation. New AC-003.13 supplies the missing cross-kind ordering outcome.
The design defines latest **new transport start** as publication authority;
same-kind joins retain their original order. This is a local cache correction,
with adequate rationale in the requirement/design/tests; no separate ADR is
needed. No material question remains unresolved.

## Scope

### In scope

- Per-workspace ownership across cached reads and refreshes, including current
  failure, obsolete failure, empty success, metadata, and returned snapshots.
- Operation-local handle cleanup and accurate exposed busy state.
- Ownership of successful snapshot follow-up, reentrant subscriptions, and
  release/disposal behavior without changing lease or visibility policy.
- Existing failed-root suppression and explicit manual/root recovery controls.
- One permanent regression matrix and one real hook/consumer integration.

### Out of scope

- Root-mutation invalidation of an already pending same-kind request. Joining
  that request remains coalesced and does not gain new publication authority.
- Backend discovery cache/scan redesign, store/API shapes, generic request
  framework, timers, background retries, and cancellation protocol.
- Layout, touch, scrolling, navigation, breakpoints, localized copy, native
  picker policy, and public documentation changes.
- Additional agents, tasks, sessions, broad local suites, or optional audits.

## Technical approach

Change `apps/web/hooks/domains/workspace/use-repository-discovery.ts` within
`RepositoryDiscoveryCoordinator`. Keep separate pending handles and add one
entry-local owner token or sequence shared by both operation kinds. Advance it
only when actually starting an operation. Install ownership and the handle
before notifying subscribers. Accept response/error only when the operation
still owns the currently registered workspace entry.

Clear each operation's own handle and derive loading/refreshing state from
remaining handles and accepted `refreshing` metadata. An obsolete completion
may update pending-derived flags but cannot replace accepted data/error.
Never clear a handle installed by synchronous subscriber reentry. Follow-up
must recheck authority after notifying subscribers and must be based on an
accepted successful snapshot, an active lease, and visibility. Current failures
preserve accepted data. Successful empty data replaces it normally.

Keep release as cache retention, without follow-up after the last lease.
Guard settlement by entry identity after `dispose()`, even if the same workspace
is recreated. Public methods return the current registered accepted response
after await, including for obsolete joined calls; they do not recreate disposed
entries. Preserve normalization and all existing hook fields.

### Consumer inventory

Search covered both `apps/web/app/` and `apps/web/components/`.

| Consumer | Existing behavior to preserve |
| --- | --- |
| `app/settings/workspace/workspace-repositories-client.tsx` | Dialog lease, shared choices/busy state, manual refresh |
| `components/repository-discovery-controls.tsx` | Shared desktop root state and `useDiscoveryRootActions` |
| `app/office/projects/use-discovered-repositories.ts` | Real hook projection used by `project-repository-picker.tsx`, null until accepted snapshot |
| `components/automations/config-section.tsx` | Shared discovered repositories |
| `components/task-create-dialog-repo-chips.tsx` | Load after adding a discovery root |
| `components/task-create-dialog-effects.ts` | Shared choices/loading/error projected into Create Task state |
| `components/task/add-workspace-sources/use-workspace-repository-options.ts` | Shared choices/busy/error; manual refresh alongside saved repositories |
| `components/task/add-workspace-sources/saved-repository-source-row.tsx` | Load after root selection |

`hooks/domains/workspace/use-discovery-root-actions.ts` uses `load()` after
Add/Reconnect/Home and `refresh()` after Remove/manual refresh. Its local
mutation guards do not order independent consumers. No consumer production
changes are expected. The existing branch-read ownership pattern is a local
reference, not an abstraction to reuse or a scope extension.

## Tests

All abbreviated AC references use `AC-WORKSPACES-LOCAL-REPOSITORIES`.

| Criteria | Required behavior evidence |
| --- | --- |
| 003.13 | Unit matrix: both cross-kind starts with both settlement orders; compare choices, roots, freshness/flags/error and public await returns |
| 003.7, 003.13 | Seed accepted baseline; current failure plus older success preserves baseline/error; obsolete failure after latest success preserves latest state, in both directions |
| 003.13 | Newest empty response; workspace isolation; removed-entry and same-ID replacement settlement |
| 003.6, 003.13 | Deferred same-kind load and refresh coalescing, including a join after an opposing operation has taken ownership |
| 003.3, 003.4, 003.13 | Obsolete stale/refreshing snapshot starts no follow-up; reentry during notification cannot duplicate requests or steal authority; release prevents follow-up without discarding valid cache |
| 002.9, 003.5 | Existing failed-root/reconnect suppression, manual recovery and post-action load controls remain green |
| 003.2, 003.13 | Real `useRepositoryDiscovery` plus Office `useDiscoveredRepositories` integration, transport-only mocks: shared current choices and flags survive late obsolete data |

Unit tests extend `hooks/domains/workspace/use-repository-discovery.test.ts`.
The integration belongs in
`hooks/domains/workspace/use-repository-discovery.integration.test.tsx` and
uses the singleton coordinator and actual exported hook/Office projection.
Retain the existing Office consumer suite as an isolation/cache control.
No coordinator, discovery hook, store, or consumer projection mocks.

## Mobile parity and E2E assessment

This change is purely shared state/data handling inside existing selection
surfaces. It changes no rendered layout, touch behavior, scroll owner,
navigation, breakpoint, or copy. `/mobile-parity` explicitly allows targeted
unit/component tests plus this note for that narrow case. The real shared
hook/Office integration proves the same state path used on phone and desktop.
No browser/E2E/build or ASCII preview is planned. Existing desktop/mobile
discovery E2E belongs to completed consent/failure-recovery packages and is
not replayed for this internal ordering defect.

## Documentation and companion packages

The existing owning requirement is near 20 KiB. Remove only duplicated legacy
Why/What prose already covered by AC-001.1 through 001.4 to fit AC-003.13;
preserve unique legacy facts. No supplement is necessary if validation passes.
Reconcile the shared coordinator section in the paired design.

Public `desktop-app.md`, `configuration.md`, and `use-kandev.md` already describe
cached choices, Refresh, and root recovery. README/screenshot searches show no
guidance affected by request-ordering internals. `/docs-maintainer` assessment:
internal docs only; user instructions and terminology are unchanged.

The linked consent, explicit-trust, local-only merge/rebase, and failure-recovery
packages are completed historical records. Their task scopes, E2E matrices and
recorded results remain accurate and are not rewritten. This package owns the
new coordinator regression.

## Work orders

- [x] [Task 01: Preserve discovery publication ownership](task-01-discovery-publication.md) (done)

Execute this single work order in the current session after a later explicit
parent instruction. No delegation or model-switch checkpoint is introduced.

## Verification results

Design validation passed on 2026-10-02:

- `python3 scripts/list-docs.py validate`: 343 decisions and 1,311 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- Repository `validateCoverage` preflight with the prospective coordinator
  change and actual four package documents: covered, no errors.
- `git diff --check`: passed; plan-directory status confirmed the new work order
  is present and untracked. All package files remain unstaged/uncommitted.

Requirement size is 20,399 bytes (20 KiB limit); design size is 23,688 bytes
(32 KiB limit). No supplement or size exception was added.
Implementation completed on 2026-10-02 after the explicit parent continuation.
Permanent RED produced 21 expected assertion failures and 10 passes; final GREEN
passed all 35 tests across the coordinator, real hook/Office integration, and
existing Office controls. Focused ESLint passed, including the changed control
waits. Those waits now await actual shared publication instead of transport
invocation. One frozen dependency install reused cached packages; no lockfile
change. Exact commands and results are recorded in the work order.
Final document/preflight/diff gates are recorded there before publication.
No product test was run during the preceding design turn.

## Delivery constraints

Leave this package unstaged/uncommitted at the design handoff. Later
implementation, commit, push, ready PR, scoped remediation, normal merge, and
owned cleanup are already authorized. Use one heavy local command at a time,
normal hooks, one managed `scripts/pr-await` monitor, and exact published-head
review evidence. Do not rebase a published candidate because main moved.
Require terminal hosted gates and an authenticated configured full semantic
review over every changed file at the published SHA; disposition actionable
findings. Verify actual merge SHA and owned content and join all handles before
the completion report. The parent owns task archival and the retained fixture.

## Risks

- Coalesced post-mutation calls do not guarantee a fresh transport. This remains
  explicit rather than being implied repaired by the ordering change.
- Busy flags represent actual pending operations even when an obsolete transport
  remains in flight; they must settle without corrupting current metadata.
- Synchronous subscriber callbacks can change authority during publication;
  checking only before notification is insufficient for follow-up.
- An entry token without map-identity protection is insufficient after disposal.
