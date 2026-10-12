---
created: 2026-10-03
status: implemented
requirements:
  - REQ-WORKSPACES-LOCAL-REPOSITORIES-002
  - REQ-WORKSPACES-LOCAL-REPOSITORIES-003
system_design:
  - ../../specs/workspaces/system-design/local-repositories.md
legacy_specs: []
---

# Implementation Plan: Repository Discovery Root Mutations

## Overview

Successful root changes must synchronize through a read started after the
mutation. One sequential work order adds that explicit boundary to the shared
coordinator and immediate callers, with deterministic transport-only hook and
consumer integration. Workspaces owns discovery roots, cached workspace
choices, and recovery; no independent UI contract changes.

## Evidence and assumption check

Base: `e6e13f39a29b2694852c39396e08b700b8c37bac`; branch:
`feature/refresh-discovery-af-8am`. This is a confirmed independent moderate
bug, not a tracker issue. PR #4174's completed
[ordering package](../repository-discovery-ordering/plan.md) explicitly excluded
root-mutation invalidation. Its results remain historical evidence, and its
cross-kind publication rules remain required. The owning design now explicitly
limits same-kind joining to unchanged roots and adds successful-mutation
synchronization; it does not rewrite the earlier implementation record.

The read-only parent fixture
`/tmp/kandev-discovery-mutation-repro.test.tsx` exercises the actual coordinator,
`useRepositoryDiscovery`, and `useDiscoveryRootActions` using React `renderHook`.
Only workspace action transport is mocked. It seeds `/removed`, starts a
deferred stale refresh, successfully removes that root, and resolves the old
response with `/removed`. Synchronization settles but roots remain
`['/removed']`, expected `[]`; the next fresh response supplies empty current
state. Parent handle 24556 joined exit 1, sole correctness assertion (19 ms
test, 2.65 s package). The archive was inspected and RED accepted without replay
or modification. Permanent RED/GREEN is required only after release.

`startRequest` returns a pending same-kind promise before starting transport.
The root-action hook's successful synchronization consequently joins a read
started before the mutation. Its local mutation ref serializes its own actions
but cannot fence sibling consumers or existing background work. Refresh UI
busy state does not prohibit every Remove or picker action. Latest distinct
request ownership alone cannot repair this absence of a new read.

AC-003.6/003.13 already cover sharing/ordering; new AC-003.14 defines mutation
synchronization. AC-002.7/002.9/002.15/002.16 retain root selection, recovery,
Home admission, and install-wide persistence. Scope is every consumer of the
initiating workspace entry in the current tab coordinator. Install-wide roots
do not imply immediate freshness of other workspace caches or tabs. Failed
mutations do not fence pending reads. No material question is unresolved.
This local correction needs no new ADR: the paired design preserves its
rationale and scope without a new architectural ownership rule.

## Scope

### In scope

- Explicit successful-mutation fencing of both read kinds, followed by a fresh
  load for Add/Home/Reconnect or refresh for Remove.
- Integration in the shared root-action hook and both direct Add Home callers.
- Detached completion safety for data/error/metadata, current busy flags,
  snapshot follow-up, reentry, release, dispose, and repeated root changes.
- Transport-only regressions using actual production hooks and a real shared
  Office consumer, with accurate final command results in this package.

### Out of scope

- Global broadcasts or cross-workspace/tab freshness, backend cache/API changes,
  general coordinator redesign, transport cancellation, timers, or retry loops.
- Other owner-lifetime, error-toast, native-picker, failed-root, or Home policy
  repairs; mutation admission and confirmation semantics remain intact.
- Markup, copy, layout, touch, scrolling, navigation, breakpoints, previews,
  browser/build/E2E, broad audits, or unrelated public guide polish.
- Delegation, additional workers, persistent tasks, or session tabs.

## Technical approach

Implement the paired design's **Successful root mutation synchronization**
section. Add one narrow `synchronizeAfterRootMutation` coordinator/hook seam
with `"load" | "refresh"` kind. Revoke prior response authority and detach both
pending handles, then reserve the selected new operation before notification.
Reuse the existing request machinery, symbols, entry identity, normalization,
and await-return contract. Ordinary `load`/`refresh` keep coalescing and latest
distinct-operation authority. Ignore detached settlement owning neither a
handle nor the response; do not strand or clear current flags. Preserve the
last accepted baseline until a current response or error settles.

Call this seam after transport success only. Manual Refresh uses ordinary
`refresh`. Keep the root-action hook's mutation refs and Home finally cleanup.
The two direct callers keep their current Add Home action and UI semantics,
replacing only post-success `load()` with the new seam. Backend mutations
already invalidate their aggregate discovery cache; no backend correction is
needed or authorized here.

### Consumer and compatibility inventory

| Boundary | Contract and evidence |
| --- | --- |
| `hooks/domains/workspace/use-discovery-root-actions.ts` | Remove uses fresh refresh; Add/Home/Reconnect use fresh load; failed action and manual Refresh retain normal reads |
| `app/settings/workspace/use-discovery-root-actions.ts` | Reexport only; no duplicate implementation |
| `components/repository-discovery-controls.tsx` | Real discovery/root-action hooks; existing controls remain reachable and unchanged |
| `components/task-create-dialog-repo-chips.tsx` | Direct Add Home success uses new load boundary |
| `components/task/add-workspace-sources/saved-repository-source-row.tsx` | Second direct Add Home success uses same boundary |
| Workspace Repositories, Office, Automations, Create Task effects, Add Workspace Sources options | Shared choices and flags for the same workspace entry; no production rewrite |
| Desktop-launched backend, Tauri or browser client | Existing install-wide root endpoints; current workspace/tab synchronization only |
| Server backend and phone/browser selectors | Ordinary reads, leases, visibility and failure policy preserved; no new mutation or native authority |
| Other workspace entries/tabs | Independent existing activation/freshness rules; immediate coherence unsupported by this package |

The backend root store has no workspace key; workspace discovery responses
apply effective install-wide roots. API actions list/add/confirm/reconnect/remove
at `/api/v1/repositories/discovery/roots`; discovery reads retain a workspace
ID. This package does not broaden the frontend cache ownership quantifier.

## Tests

All short AC references below use `AC-WORKSPACES-LOCAL-REPOSITORIES`.

| Criteria | Permanent evidence |
| --- | --- |
| 003.14, 003.13 | `use-repository-discovery.mutation.test.ts`: same-kind and opposite-kind pending work, both settlement orders, all old success/failure versus new success/failure directions, authoritative empty data, exposed flags/metadata/error and await returns |
| 003.6, 003.13, 003.14 | Same test: ordinary same-kind sharing before/after mutation, latest distinct request authority, second successful mutation, notification reentry and current-handle reservation |
| 003.3/003.4/003.7, 002.9 | Same test: old stale/refreshing snapshots cannot launch follow-up, current eligible snapshot policy, hidden/released/failed-root controls, disposal/recreation and workspace isolation |
| 002.7/002.15, 003.14 | `use-discovery-root-actions.integration.test.tsx`: real coordinator/discovery/root-action hooks plus real `useDiscoveredRepositories`; every successful handler issues correct fresh transport; same-kind stale Remove reproduces parent's bug |
| 003.14, 003.5 | Same integration: mutation failure leaves pending read valid; manual Refresh coalesces; Home/mutation admission and finally reset remain correct |
| 003.14 | Direct Add Home caller wiring checks in `task-create-dialog-repo-chips.test.tsx` and `saved-repository-source-row.test.tsx`; use actual seam through real hooks for race proof, mocks only for presentation dependencies as needed |

Unit tests inject only deferred client transports into the actual exported
coordinator. Integration mocks workspace action transport only, not the
coordinator, discovery hook, root-action hook, store, or Office projection.
Each relevant test asserts externally observed choices, errors, flags and
transport counts, not a private predicate. Settle/join all deferred operations,
unmount, release leases and dispose in owned cleanup even after assertion
failure. Distinct roots/repos/metadata prevent false success by equality.

## Mobile and documentation assessment

Pure shared state/data synchronization; no rendered structure or interaction
changes. `/mobile-parity`'s narrow unit/component-test exception applies to the
same production hooks across viewports. No browser, build, E2E, or ASCII preview
is required. End-to-end state evidence is the actual action -> coordinator ->
shared hook -> Office consumer integration with transport mocked.

`docs/public/desktop-app.md`, `use-kandev.md`, `configuration.md`, root README
and screenshot catalog were checked. Existing guidance promises root selection,
Reconnect, Remove, and Refresh with correct semantics; it does not promise
immediate cross-tab/workspace cache broadcasts. Internal specs/plans suffice.
The older design's unrelated `"~"` wording is outside this repair; the current
backend and direct callers already support Add Home. No policy rewrite here.

## Work orders

- [x] [Task 01: Synchronize discovery after successful root mutations](task-01-root-mutation-synchronization.md)

One sequential order, no dependencies or parallelism. Implementation starts
only on a later explicit parent reviewed-package interruption in this session.

## Verification results

Design validation on 2026-10-03:

- `python3 scripts/list-docs.py validate`: 343 decisions, 1,321 specifications.
- `python3 scripts/lint-spec-files.test.py`: all 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- Real `validateCoverage` evaluator with these four documents and the four
  prospective production paths: `covered`, no errors. All work-order REQ/AC
  references and the manifest/design chain resolve.
- `git diff --check` passed; plan-directory status confirms the new manifest
  and sole work order are untracked. Full package remains unstaged/uncommitted.
- Requirement: 20,417 bytes, under 20 KiB; design: 27,207 bytes, under 32 KiB.
  Duplicate migrated requirement detail was removed without dropping criteria;
  no supplement or size exception was introduced.

No production or permanent test changes, install, product test, browser, or
build during the completed design turn. Exact implementation commands and prerequisite are in
the work order; retain and join each returned process handle before starting
another local heavy job.

Implementation completed after the explicit parent reviewed-package release
on 2026-10-03. The permanent real-hook Remove test failed at the expected
`['/removed']` versus `[]` assertion before production edits and passed after
the correction. Final scoped coverage: 46 coordinator cases, 12 real
root-action/discovery/Office integration cases, two real saved-source caller
cases, and two focused Create Task caller checks (62 total). New tests use
deferred transports and public state; no private-predicate or transport-only
mocked result is treated as shared-state evidence.

Changed-file ESLint, web typecheck, i18n check/ratchet and targeted post-correction
reruns passed. Initial ESLint test-group size warnings were resolved by removing
group wrappers; initial typecheck found one missing test cleanup fixture argument,
which was supplied explicitly. No assertions, timeouts or gates were weakened.
The single pinned frozen install reused 923 packages in 2 seconds. No lockfile,
cache configuration, generated tracked output, public copy, backend or UI
presentation change. Every returned local handle was joined before the next
heavy command. Exact commands, results and limitations are in Task 01.

Delivery remains pending terminal hosted checks, exact-head full semantic review,
normal merge and independently verified merge/owned cleanup receipt.

## Delivery gates

Task `21f292ef-42d4-43e4-8d0a-01f33aa949bb`, session
`0e5bd5b9-7bc1-4992-9bba-5d1cce82b0b1`; parent task
`14825981-b175-411d-999a-31ddc2aa5fc3`, session
`4b15fc37-c487-4e2b-b4a2-2833edf18794`. Leave this four-artifact package
unstaged/uncommitted, send queued parent handoff, and end design turn. After
explicit implementation release, standing autopilot authorizes conventional
commit with active hooks, push, ready PR, scoped valid remediation, normal
expected-head squash merge and owned cleanup. No further approval question.

One local heavy command and one owned `scripts/pr-await` monitor at a time;
join actual terminal handles before replacements. Preserve published SHA
absent a valid finding; no moving-main rebase or check weakening/bypass. Require
terminal hosted required gates and authenticated configured CodeRabbit App
347564 substantive full review of all changed files at the exact current head.
Acknowledgement, zero-second or skipped reviews do not count; a completed full
report suffices without duplicate requests or optional second-review waits.
Disposition every actual finding. Corrected-head incrementals are disabled;
at most one necessary full request. Report unrelated CI leaf evidence/logs
to the parent before expanding scope or retrying blindly. Persist branch/PR/SHA,
review/check evidence and process handles if interrupted.

Completion requires independently verified actual merged SHA, expected owned
tree/blobs and remote state plus joined owned cleanup. Preserve the platform
worktree and parent fixture for parent archival. Use queued parent messages
for design handoff, actionable blocker/recovery, or verified merge/cleanup
receipt; final chat alone does not wake the parent. No frequent heartbeat.

## Risks

- Revoking only response ownership leaves old same-kind handles joinable.
  Detaching only one kind leaves old busy/error/follow-up behavior possible.
- Notifying between invalidation and fresh handle reservation allows reentry
  to start duplicate work or take unintended authority.
- A stale completion must not clear a fresh same-kind handle. Accepted
  `refreshing` metadata is distinct from detached transport activity.
- A successful mutation whose synchronization fails preserves the last accepted
  response and current error; no invented fallback or auto retry is added.
- Backend roots remain install-wide while immediate frontend freshness remains
  workspace/tab scoped. Do not describe this as global cache coherence.
