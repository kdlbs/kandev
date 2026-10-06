---
created: 2026-10-05
updated: 2026-10-06
status: in_progress
requirements:
  - REQ-SYSTEM-PAGE-BACKUP-GUIDANCE-001
  - REQ-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001
  - REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-002
system_design:
  - ../../specs/system-page/system-design/backup-list-query-cache.md
legacy_specs:
  - ../../specs/system-page/system-design/system-page-01.md
---

# Implementation Plan: Give the System Backup List One Query Owner

## Overview

After QUERY-02's frontend Query ownership migration merged into main as PR
#4225 at `059260b30fc68bbcbead629f7fa7e80f1ee0a5e8`, move the System Backups
list and request lifecycle from Zustand to one identity-scoped TanStack Query
entry. Preserve the observable backup list, mutation, permission, restart, and
resolved-directory behavior.

## Scope

### In scope

- Move backup-list data and request state to TanStack Query and remove the
  backup-only Zustand state, defaults, setter, hydration, and duplicate effects.
- Reuse the merged QUERY-02 identity, cancellation, stable-client, and
  targeted-cleanup contract.
- Preserve reload returning Promise<SnapshotInfo[]> and rethrowing the
  original request error. Preserve `loaded` after refetch failure, including a
  previously successful empty list.
- Refresh only the backup-list identity after operations that can publish a
  listed snapshot.
- Preserve the DatabasePanel backup_directory description supplied by the
  useDatabaseStats observer.
- Cover concurrent consumers, auth/backend/process transitions, permissions,
  failures, in-flight requests, mutation races, and unrelated shell/form state.

### Out of scope

- Backend backup API, retention, classification, or snapshot semantics.
- Restore/reset operation behavior, confirmation, quit/relaunch, or permission
  semantics.
- Disk usage, WebSocket generalization, boot payload changes, or a new
  provider/identity abstraction.
- A UI layout, copy, route, navigation, or touch-interaction change.

## Technical approach

### Dependency and shared Query behavior

The dependency gate was satisfied by child task
95ad66df-588d-48d1-afee-5c674c928013's frontend database-statistics Query
migration PR #4225, merged as `059260b30fc68bbcbead629f7fa7e80f1ee0a5e8`.
Implementation began from refreshed main `95c040e84951773dc8ed6f684fff0425d7b63f96`,
which contains that merge. Reuse its stable QueryClient, identity keying,
AbortSignal transport, and exact cleanup behavior. #4001 is a backend
resilience change and does not satisfy this gate.

Register the backup query family with the landed targeted cleanup. Keep the
QueryClient stable across backend and auth identity changes. do not clear
unrelated entries or remount the shell. Keep backup_directory in the
DatabasePanel's useDatabaseStats observer as delivered by QUERY-02.

### Backup list and consumers

Use the existing fetchBackups API and pass the Query observer AbortSignal
through its existing RequestInit support. Give the list a mutable finite
freshness policy and disable automatic retries. useBackups exposes the Query
result while retaining reload's Promise<SnapshotInfo[]> result and original
error rejection. Bind the transport base URL to the captured key.
Use `isFetching` for the existing loading flag and successful data presence
for `loaded`. Refetch failure must not turn a loaded list into an unloaded one.
BackupsTable keeps its local create/delete error display. It currently ignores
the hook's initial-read error, so this package adds no read-error control. Successful empty results remain distinct from pending and
error. refetch failures retain the last successful data.

Set `staleTime` to 30 seconds and `gcTime` to five minutes. Freshness controls
whether focus/reconnect events need a request. inactive retention starts only
after the last observer unmounts and does not trigger a request. Use
`refetchOnMount: "always"` so every later Backups mount can observe an external
maintenance snapshot, even while cached data is fresh. Use stale-only
`refetchOnWindowFocus: true` and `refetchOnReconnect: true`, `retry: false`,
`networkMode: "always"` for the local backend, and no `refetchInterval`. This
intentionally replaces Zustand's shared `loaded` guard, which suppresses later
mount reads. It adds no polling. explicit reload always performs an
authoritative read regardless of freshness. After five observer-free minutes,
the inactive entry may be collected and a later mount uses the initial-loading
state. The retention window keeps the small last-good list across short route
changes. the freshness window avoids repeated focus/reconnect reads during
brief returns. Do not copy the DatabaseStats polling schedule.

BackupsTable remains the list consumer. Preserve the existing create poll. POST acceptance does not add a snapshot.
Reload continues every 250 ms for up to 15 seconds and returns the list for the new-name check.
After DELETE succeeds, perform an authoritative read for the current key.
DELETE failure does not invalidate the query.
Before the first read after a writer's result, cancel any earlier read of that exact key.
Apply this boundary even without cached data. Default refetch cancellation alone does not cover that case.
Explicit reload rejects errors and returns successful data, rather than a Query result object.
Concurrent reloads can share an eligible read. Cancellation cannot resolve as a successful empty list.

Every operation captures the full initiating identity and caller lifetime.
Late create/delete/reset responses cannot refresh another identity or recreate an obsolete query.
An A-to-B-to-A transition ends the old lifetime even though the final key equals the original key.
Stop the old create poll on unmount, logout, or identity change.

The tool-payload-retention hook already polls preparation status. Before a
backup-choice save, capture its full backup-query identity and candidate next
policy revision (`submitted revision + 1`). Only a change that requires new
preparation creates a save candidate. Reuse the existing preparation-review
logic and saved baseline. A save without new review can carry historical ready
state into the next revision. It must not count as a new writer.
For a qualifying candidate, establish a local attempt only from a matching successful save response.
The response must have the expected revision, backup choice, and pending, running, ready, or failed state. Process
the save response itself, because it can already be terminal. Discard the
request candidate on a rejected or nonmatching response while preserving the
save result/error contract. A first status read that observes pending/running
with backup choice can independently establish an observed attempt. an initial
ready/failed status is historical and does not invalidate. Do not use Operation
ID: success replaces the backup Operation with cleanup.
Invalidate once when the matched attempt reaches ready or failed, or when a
previously observed pending/running attempt ends through cancellation or a
newer policy revision. Settle the old attempt before considering a candidate
for a replacement revision. This refresh accounts for a snapshot published
before cancellation or replacement prevented ready from being recorded. Ignore
historical terminal status, unrelated cleanup, repeated polls,
and responses from an obsolete identity. Clear the active attempt before invalidation, but keep a bounded settled-revision marker for that lifetime.
Duplicate save responses or late pending/running status cannot re-arm a settled revision.
Clear all correlation state on full identity or caller lifetime changes.
Keep the existing mutation serialization and read-generation guards.
Skip choice creates no attempt but can settle an older active backup attempt.
End the old retention request epoch on identity change and reset its remote state.
Old request finalizers cannot release a new mutation lock or clear its pending state.
Guard the retention draft Save contributor after its await, so old save results cannot update the new draft or clear choice. Query
invalidation refreshes an active list and marks an inactive entry stale. Keep
retention status in its current hook.

An initial historical ready/failed status without a matching save response or
an observed pending/running state has no safe attempt correlation. Leave that
case to the list's mount, stale-focus/reconnect, or explicit-reload paths.

FactoryResetDialog captures the initiating identity, lifetime, and accepted job ID.
Its existing job observer settles that pair once on succeeded/failed, through WS or fallback polling.
Only a current pair refreshes this list. Reset can publish a snapshot before a later failure.
Retention/reset list-refresh errors preserve their operation result and last-good list.
Restore does not mutate listed files. after its existing quit/relaunch flow,
the new process identity fetches a new query. Startup snapshots are observed by
the initial read. Operator-run maintenance is external to the SPA and remains
visible on the next fresh mount or explicit reload. do not add polling or a
generic job-event bridge.

### Remove old owner

Remove SystemBackupsState, system.backups defaults, setSystemBackups, the
backup slice test cases, its `index.ts` barrel export, and any backup-specific hydration or selectors found
by the post-gate code search. Preserve all unrelated System, auth, form, and
shell state.

### Tests

- Hook/provider: simultaneous observers share one request. reload resolves an
  empty or nonempty list and rejects with the exact original error. initial
  request does not retry. failed refetch retains the prior list. observer
  cancellation reaches fetch. obsolete backend/process/auth/user entries are
  cancelled and removed without clearing unrelated resources. Verify
  freshness and retention behavior with controlled timers and focus/online
  events. Cover the 30-second freshness window, five-minute inactive retention,
  mount-always reads, offline local reads, and no periodic polling. Do not add
  tests that only repeat the configured option values. Verify a later mount
  reads an external maintenance write, while explicit reload reads even when
  data is fresh.
- Consumer/mutations: create polling sees a newly listed name and stops on
  reload rejection. only successful DELETE refreshes. failed DELETE keeps
  current data. Retention tests cover accepted save responses already at ready/failed and
  failure after visible publication. Repeated terminal polls must invalidate once.
  Cover cancellation to none and policy replacement after visible publication. Also cover initial historical
  ready/failed without a matching attempt, skip choice, unrelated cleanup, a
  later distinct revision, and active/inactive list observers. Defer status,
  save, and cancel responses, change backend/auth identity, then resolve them;
  none may invalidate or update the new identity. Reset terminal states
  refresh the list. restore errors keep data and successful restore waits for
  the new process identity. stale in-flight list responses cannot overwrite a
  newer mutation or identity. Cover initial pending requests without data,
  cached refetches, one remaining observer, StrictMode replay, and A-to-B-to-A
  transitions. Cancellation must never complete create with a fabricated list.
- State and presentation: removing backup Zustand state leaves other System
  defaults/actions and unrelated shell/form state intact. The DatabasePanel
  continues to display the resolved backup directory from the
  useDatabaseStats observer.

### Acceptance and evidence map

The work order references existing product criteria for regression evidence.
Query ordering and ownership checks are technical design contracts, not new product requirements.
Add the named focused cases before production changes through `/tdd`.

| Contract or criterion                                          | Planned evidence                                                                                                                                                                                           |
| -------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Backup guidance AC `.1`, `.2`, `.6`. database snapshot AC `.5` | `system-route-copy.test.ts` and `backups-page.spec.ts`: retain resolved directory and absent-metadata behavior after QUERY-02.                                                                             |
| Backup guidance AC `.3`, `.4`, `.5`                            | `backups-table.test.tsx`, desktop Backups E2E, and phone member-gating E2E: preserve action names, touch targets, and wrapping.                                                                            |
| Data/Storage pages AC `.1`, `.5`                               | `backups-table.test.tsx`, `tool-payload-retention-card.test.tsx`, and member-gating E2E: preserve placement, permissions, and maintenance actions.                                                         |
| Hook state and reload contract                                 | New `use-backups.test.tsx`: loaded empty data stays loaded after refetch failure. Concurrent reloads return data or the original error.                                                                    |
| Request ordering and identity cleanup                          | `use-backups.test.tsx` with the real shared provider, plus new `backup-list-query.test.ts`: an initial read cannot populate data after a writer boundary. Late callbacks cannot recreate obsolete entries. |
| Retention correlation                                          | `use-tool-payload-retention.test.ts`: carried historical ready state is ignored. settled revisions cannot re-arm. later qualifying revisions settle independently.                                         |
| Reset correlation                                              | `system-confirmation-dialogs.test.tsx`: duplicate terminal WS/poll observations refresh once. old job acceptance and terminal updates cannot refresh another lifetime.                                     |
| Remove old owner without unrelated state loss                  | `system-slice.test.ts`, provider/hook state-preservation cases, and post-gate source search: no backup mirror, setter, or barrel export remains.                                                           |

## E2E tests

- Desktop Backups page covers the list, resolved path, create/delete flow, and
  admin/member access in tests/system/backups-page.spec.ts and
  tests/auth/system-data-storage-member-gating.spec.ts.
- Phone Data & Logs remains the same surface. The existing
  tests/auth/mobile-system-data-storage-member-gating.spec.ts covers member
  list access, hidden admin actions, touch targets, and horizontal containment.
  The change is state-only, so the existing rendered phone test is the mobile
  parity evidence. Its overflow assertions currently run after navigation to
  Storage. Add an assertion while Data & Logs is visible, so the Backups surface
  itself has containment evidence. Keep its existing admin touch-target checks.
- Add a desktop revisit case in `backups-page.spec.ts`.
  Read the list, then use the header Logs tab to unmount Backups.
  Create a manual snapshot through the existing API fixture. Return to Database and see it without a hard page reload. Clean up the snapshot afterward.
  This is rendered evidence for mount revalidation without a new UI control.

## Work orders

- [ ] [Task 01: Move backup-list ownership to Query](task-01-backup-list-query.md)

## Delivery and documentation

Update only QUERY-03 in `docs/architecture-maintenance/server-state-migrations.md`.
Link this design and work order, record ownership removal and regression evidence,
and keep its status accurate. Do not mark it Done before merge.
This PR is not authorized to merge.

Review public documentation impact through `/docs-maintainer`.
The current scope changes no API, CLI, configuration, copy, or operator procedure.
Record that reason if no public-doc edit is needed.
Reconcile any old backup-specific ownership prose affected by the final diff.
Update the old location design only if QUERY-02 leaves its DatabasePanel contract stale.
Do not overwrite QUERY-02's delivery record or claim completion of its work.

After the listed checks pass, use `/commit`, `/push`, and `/pr` to open one focused PR to main.
Handle CI failures and valid review findings in the primary session through `/pr-fixup`.
Record exact head/base SHAs, dependency merge evidence, owned files, local command results,
exact-head CI and review status, and residual risks. Report the PR URL without merging.

## Verification results

In progress; the final command results, coverage preflight, PR URL, and exact-head
CI/review disposition will be added to the task work order before handoff.

## Risks

- Current main has moved beyond the refreshed implementation base by an
  unrelated GitLab-search fix. Exact-current-main merge validation is required
  before reporting the PR ready; rebase only for a real conflict or contract
  change.
- Tool Payload Retention has no stable backup-attempt ID and replaces its
  Operation on success. Correlation must use the locally observed save,
  policy revision, preparation transitions, and captured query identity.
- Factory reset can publish its pre-reset snapshot before a later job failure.
  Terminal refresh must preserve the existing required relaunch after success.
- system-page-01.md has stale permission prose for backup mutations. Use the
  current handlers, authz tests, and backup-specific design. preserve member
  GET and admin-only mutations.
