---
created: 2026-10-03
status: in_progress
requirements:
  - REQ-AGENTS-PROFILE-LIST-ORDERING-001
  - REQ-AGENTS-PROFILE-LIST-ORDERING-003
system_design:
  - ../../specs/agents/system-design/profile-list-ordering.md
legacy_specs: []
---

# Implementation Plan: Agent Profile List Ordering

## Overview

Administrators can manually reorder profile rows within an agent on Settings >
Agents using the drag handle. The order is persisted for that screen and its
Settings navigation tree. Profile selectors keep their existing ordering,
recency, and default-selection behavior. The frontend removes automatic profile
sorting while preserving the backend reorder contract and cross-client sync.

## Scope

### In scope

- `agent_profiles.sort_order`, the ordered Settings read, and the reorder
  endpoint and event.
- Drag handles and reorderable rows in each installed agent's profile list.
- Prepending newly created and duplicated profiles on every client.
- Cross-client sync for the Settings list and navigation tree.
- Preserve existing profile selector ordering, recency, and default selection;
  ensure saved Settings order does not flow into selector options.
- Locale strings in all catalogs and public documentation.
- Removal of the automatic profile-sorting action and its helper, locale key,
  tests, and public-documentation reference.

### Out of scope

- Agent-card order, the Dynamic agent's profiles, workspace-scoped (Office)
  profiles, and the Office agents list.
- Automatic profile sorting by name, agent label, or any other key; persistent
  sort modes; per-user orders; and new selector ordering or recency behavior.

## Technical approach

### Backend (Task 01)

- `store/sqlite.go`: `sort_order` in the `CREATE TABLE`, in
  `agent_profiles_new`, behind a `srcHasSortOrder` copy guard, and via
  `r.migrate.Apply("agent_profiles.sort_order", ...)` after the
  `migrateDropModelCheckConstraint` block; `ListAgentProfiles` orders by
  `sort_order ASC, created_at DESC, id ASC`.
- `store/sqlite_profile_order.go`: `ReorderAgentProfiles` (set-equality check,
  per-agent PostgreSQL advisory transaction lock before the first read,
  `sort_order` write, revision upsert in one transaction) and
  `GetAgentProfileOrderSnapshots`, which reads ordered profile rows plus the
  revision map in one read-only transaction. PostgreSQL uses
  `sql.LevelRepeatableRead`; SQLite relies on the WAL snapshot established by
  the transaction's first query. Create `agent_profile_orders` in `sqlite.go`.
- `store/store.go`: add the repository methods; update every `Repository` fake.
  `GetAgent` and `ListAgents` use the snapshot result rather than separate reads.
- `controller/profile_order.go`: `Controller.ReorderAgentProfiles`,
  `ErrProfileOrderStale`, Dynamic-agent rejection; returns saved IDs, revision,
  and changed flag. `controller/agent_crud.go` and `dto/dto.go` add
  `profile_order_revision`; both `GetAgent` and `ListAgents` build profile DTOs
  and revisions from `GetAgentProfileOrderSnapshots`. Tests in
  `controller/agent_crud_snapshot_test.go` give the legacy profile getter
  deliberately different rows and assert each GET path uses the combined
  snapshot rows and revision without calling that getter.
- `handlers/handlers.go` and `handlers/profile_handlers.go`:
  `PUT /agents/:id/profiles/order` with `cfg` and `h.interlock`; the handler
  broadcasts `ws.ActionAgentProfilesReordered` (payload with `revision`) with a
  `//ws:global` comment.
- `pkg/websocket/actions.go`: `ActionAgentProfilesReordered = "agent.profiles.reordered"`.
- Persistence ownership: add `agent_profile_orders` to
  `requiredstores/catalog.go` and its catalog test, plus the fixed
  `storeconformance` adapter coverage. Update the tagged `v0.93.0` upgrade
  manifest with post-upgrade assertions for `agent_profiles.sort_order = 0`
  for both SQLite and PostgreSQL; keep the historical SQL fixtures unchanged.
- Global profile membership changes use the same per-agent transactional lock
  as reorder: create, duplicate, soft-delete, agent deletion with profile
  cascade, and full-row profile updates. A profile deletion locks its candidate
  owner and stable profile ID even when the preliminary row is workspace-scoped,
  then rereads owner and scope under lock and retries if either changed.
- Add deterministic multi-connection SQLite and Postgres tests for create/delete
  and competing A→B versus A→C or workspace/global ownership moves against
  reorder, in both lock acquisition orders. Stale membership rejects with `409`
  and no event; reorder-first commits against the exact set validated. Also
  cover a workspace-profile deletion racing promotion from A to B and reorder
  of B, proving deletion retries under B's lock without disturbing saved order.

### Frontend (Task 02)

- `lib/settings/agent-profile-order.ts`: `reorderIds`,
  `insertFirstInAgentGroup`; remove `sortProfileIdsByName` and obsolete flat-list
  Settings-order helpers.
- `lib/settings/agent-profile-selector-order.ts`: normalize timestamp metadata
  and restore per-agent selector baselines without moving workspace-scoped or
  unstamped option slots.
- `app/actions/agents.ts`: `reorderAgentProfilesAction` returns the saved order
  response and throws `ApiError` for HTTP failures. The queue handles `409`
  through its conflict path; requests use a 15 s abort timeout.
- Settings slice: per-agent `ProfileOrderSync` (`revision`, `order`, `inFlight`,
  `queued`), `acceptServerOrder`, `reconcileAgentOrders` applied inside
  `setSettingsAgents` and `hydrateSettings`, plus
  `applyAgentListSnapshot(agents, expectedProfileVersion)` as the membership
  freshness fence for fetched lists. It atomically rejects mismatched client
  epochs and reconciles `ProfileOrderSync` before applying accepted results.
  Profile events advance the client-local epoch; route bootstrap retries until
  stable, and browser GET consumers use the guarded action. Order revision
  protects ordering only; profile membership changes do not increment it, and
  order changes do not bump `agentProfiles.version`.
- Keep the persisted order in the Settings projection. The flat
  `agentProfiles` selector list and direct selector options retain the
  pre-feature per-agent baseline (`created_at DESC, id ASC`); existing
  context-specific recency ranking and default selection remain unchanged.
- `lib/settings/profile-order-queue.ts` and `hooks/domains/settings/use-profile-order.ts`:
  per-agent save queue and its thin accessor.
- `lib/ws/handlers/agents.ts`, `lib/types/backend.ts`: handle
  `agent.profiles.reordered` and read `profile_order_revision` from agents; place
  created profiles first in the Settings projection and preserve the selector
  list's existing newest-first baseline in `handleProfileCreated`,
  `use-profile-duplicate.ts`, `hooks/domains/settings/use-agent-creation-store-sync.ts`,
  and `app/office/setup/agent-profile-setup-controls.tsx`.
- Audit every selector source (`agentProfiles`, `settingsAgents`, and direct
  `GET /agents` flattening) so the saved Settings order does not change its
  option order, recency, or default selection.
- `components/settings/agents/agent-profiles-section.tsx`: reorderable rows and
  drag handle.
- `app/settings/agents/page.tsx`: remove the sort button and client-side sorting
  handler from `InstalledAgentsHeader`.
- `hooks/domains/settings/agent-list-resource.ts` and direct browser
  `listAgents` consumers (`app/settings/agents/page.tsx`,
  `app/settings/agents/[agentId]/profiles/[profileId]/use-agent-profile-settings.ts`):
  reject a full-list response captured before a profile membership event.
- `src/locales/*/agents.json`: retain `dragProfile`, `profileSortable`, and
  `profileOrderSaveFailed`; remove `sortProfilesByName` from every locale.
- `docs/public/agents-and-profiles.md`: describe manual reordering and remove the
  automatic-sorting instruction; limit the ordering claim to Settings.

## ASCII UI preview

### UI-01: Settings > Agents, desktop, administrator

Entry: Settings > Agents. The installed-agent toolbar keeps its existing
actions; Rescan stays immediately before agent creation, which remains the
rightmost action (`AC-AGENTS-SETTINGS-PROFILE-LAYOUT-001.4`). A drag handle leads
each profile row; handles are absent when an agent has fewer than two profiles
or the caller cannot manage agents. Spacing and icons are illustrative.

```text
Installed agents          [Terminal] [Rescan] [Add TUI agent]
+--------------------------------------------------------------------------+
| Claude                                                                   |
|--------------------------------------------------------------------------|
| [::] * Work profile           [sonnet] [No fallback]        [dup] [del]  |
| [::] * Personal               [opus]   [Next model]         [dup] [del]  |
| [::] * Review (disabled)      [haiku]  [No fallback]        [dup] [del]  |
+--------------------------------------------------------------------------+
| Codex                                                                    |
|   * Default                   [gpt-5]  [No fallback]        [dup] [del]  |
+--------------------------------------------------------------------------+
[::] = drag handle (mouse, touch, keyboard). Codex has one profile: no handle.
The toolbar has no automatic profile-sorting action.
```

### UI-02: Dragging a row (states)

```text
| [::] * Work profile      ...                                             |
| [##] * Personal  (lifted, follows pointer, drop slot shown below)        |
| - - - - - - - - - - - - - - drop here - - - - - - - - - - - - - - - - - |
| [::] * Review (disabled) ...                                             |
```

Save failure: the rows return to the saved order and an error toast reads
"Could not save the profile order".

### UI-03: Phone

Same composition as UI-01 with these differences: the header actions wrap, the
profile action menu replaces the inline buttons, and the handle has a 44 px
touch target. The handle has `touch-none`, so a touch drag on the handle starts
after 8 px of movement, and the page still scrolls when the touch starts outside
the handle.

```text
Installed agents
[Terminal]
[Rescan]   [Add TUI agent]
+------------------------------+
| Claude                       |
|------------------------------|
| [::] * Work profile      [:] |
|      [sonnet] [No fallback]  |
| [::] * Personal          [:] |
|      [opus] [Next model]     |
+------------------------------+
```

## Tests

| Criteria                                                  | Evidence                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| --------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `AC-AGENTS-PROFILE-LIST-ORDERING-003.1`, `003.4`, `003.9` | `store/sqlite_profile_order_test.go`: order persists, default order is newest first, `updated_at` unchanged, revision increments only when the order changes                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `003.5` (server)                                          | same file: a profile created after a reorder lists first and the revision is unchanged                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| `003.6`, `003.10`                                         | same file, `TestReorderAgentProfiles_RejectsMismatchedSet` and deleted/workspace-scoped rows; `controller/profile_order_test.go`, Dynamic agent rejected, DTO carries `profile_order_revision`; `handlers/profile_order_handlers_test.go`, `409` and `400`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `003.8`                                                   | `handlers/agent_settings_org_scope_test.go` and `handlers/interim_settings_interlock_test.go` (route added to both tables); `handlers/profile_order_handlers_test.go`, `TestReorderProfilesRequiresConfigPermission`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `003.2` (server)                                          | `handlers/profile_order_handlers_test.go`, `TestReorderBroadcastsEvent` (payload carries revision), `TestStaleReorderDoesNotBroadcast`, and agent-deletion `404` with no reorder event; no event on unchanged order                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| Migration and schema ownership                            | `store/sqlite_migration_test.go`: fresh install, same-DB replay, legacy `CHECK(model)` recreation preserving `sort_order`, `agent_profile_orders` created; `store/postgres_schema_test.go`: fresh and replay (DSN-gated); `requiredstores/catalog_test.go` and `storeconformance` assert both order tables; tagged upgrade manifest checks migrated `sort_order` on the pinned SQLite/Postgres fixtures                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| `003.6` serialization                                     | Two-connection SQLite and env-gated Postgres tests exercise both lock winners for global create/duplicate, profile soft-delete, `DeleteAgent` cascade, A→B versus A→C updates (including pre-read A, concurrent A→C commit, retry with sorted locks {B,C}), and workspace→global/global→workspace moves against reorder; assert final membership/order, `409` or `404` with no reorder event when mutation wins, and exact committed order/event when reorder wins. A deterministic race pauses deletion after reading a workspace-scoped profile, promotes it from A to B, commits a reorder of B, and proves deletion retries under B's lock without disturbing saved order                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `003.12`                                                  | `store/profile_order_snapshot_test.go`: file-backed SQLite WAL and DSN-gated Postgres tests call the production snapshot implementation through a package-private after-profile-query barrier, commit a reorder using an independent writer while the read transaction is paused, and assert old-order/old-revision in flight plus new-order/new-revision on the next snapshot. `controller/agent_crud_snapshot_test.go` gives the legacy profile getter different rows and proves both `GET /agents` and `GET /agents/:id` source DTO profiles and revision from the combined snapshot without calling the legacy getter                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `003.13`                                                  | `hooks/domains/settings/agent-list-resource.test.ts`: dispatch profile-created and profile-deleted events after a pre-event list request starts, resolve its stale response at the same order revision, assert it is not applied, the created profile stays first, the deleted profile stays absent, and a fresh response is applied. `app/settings/agents/page.agent-list-snapshot.test.tsx`: trigger the custom-TUI refresh, defer its `listAgents` response, dispatch create/delete events before resolving the old response, and assert both mirrored lists preserve event-known membership. `app/settings/agents/[agentId]/profiles/[profileId]/use-agent-profile-settings.test.tsx`: trigger the missing-profile fallback, defer its GET, dispatch create/delete events, resolve the pre-event response, and assert neither list loses or resurrects membership. `lib/settings/profile-order-queue.test.ts`: during a deferred `409` refetch, dispatch each event, resolve a pre-event response at the unchanged order revision, assert neither mirrored list is partially replaced, then accept a fresh resource response and replay queued intent with the created profile first and deleted profile absent. `lib/state/slices/settings/settings-slice.test.ts` verifies `applyAgentListSnapshot` atomically rejects a mismatched client epoch. `lib/state/hydration/hydrator.test.ts` verifies pre-event create and delete bootstrap snapshots cannot erase or resurrect event-known membership and leave agent loading incomplete for retry |
| `003.14`                                                  | `agent-profile-selector-order.test.ts` preserves global selector baselines and workspace-scoped option slots; `task-create-dialog-options.test.tsx` and the applicable context selector tests cover unchanged recent-use/default selection after a saved Settings reorder                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `001.9`                                                   | `agent-profile-layout.spec.ts` and `mobile-agent-profile-layout.spec.ts` assert the admin toolbar contains only the existing Terminal, Rescan, and agent-creation actions                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `001.8`, `001.5`, `003.7`, `003.11`                       | `lib/settings/profile-order-queue.test.ts`: coalescing to latest drag; older-revision snapshots and out-of-order events cannot replace known order; own echo is ignored; higher-revision foreign order beats own `200`; an earlier save failure with a newer queued drag submits the queued order without rollback; on a deferred `409` refetch, create/delete events interleave at unchanged order revision, stale response changes neither list, fresh resource response reconciles and replays queued intent with the new profile first and deleted profile absent; terminal failure without newer intent restores server order; other agents' overlays survive                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `003.11`, `003.2`, `003.5` (client)                       | `lib/state/hydration/hydrator.test.ts` preserves known order for older snapshots in both lists; `lib/ws/handlers/agents.test.ts` applies order patches without changing membership, preserving later creates and ignoring deleted IDs, and applies a remote `agent.profile.created` event to a second store after manual order exists, asserting the new profile is first in both `settingsAgents` and its group in `agentProfiles`; `lib/settings/agent-profile-order.test.ts` preserves orphan/newer options and prepends new profiles; duplicate, agent-save-helper, provider-helper, and Office setup tests cover profile creation flows                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `001.3`, `001.4`, `001.7`                                 | `agent-profile-order.spec.ts` covers a cross-agent drop and drag-handle behavior; the auth profile-order spec covers the no-handle permission boundary                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |

## E2E tests

`apps/web/e2e/tests/settings/agent-profile-order.spec.ts` (project `chromium`)
and `mobile-agent-profile-order.spec.ts` (project `mobile-chrome`). Both create
dedicated profiles through the API and delete them in cleanup, and restore any
changed order through `PUT /agents/:id/profiles/order`, because the e2e backend
is worker-scoped and never resets global profiles; other specs pick
`profiles[0]` and must not see a leaked order.

- Drag a profile with the pointer and with the keyboard, reload, and confirm the
  order persists (`AC-AGENTS-PROFILE-LIST-ORDERING-001.1`, `001.2`, `003.1`).
- Clicking a drag handle does not navigate; a drop onto another agent's row
  changes no order, and opening, duplicating, and deleting a profile still work
  while handles are rendered (`001.3`, `001.6`).
- The admin toolbar contains no automatic profile-sorting control (`001.9`).
- Task/session/chat/handoff selectors retain their existing option order and
  context-specific recent-use/default selection after a Settings reorder
  (`003.14`).
- A second page sees the reorder without reload, and the navigation tree
  matches (`003.2`, `003.3`).
- A newly created profile appears first without reload (`003.5`).
- A non-administrator sees no handles (`001.4`), in
  `e2e/tests/auth/agent-profile-order-member.spec.ts` (project `auth`), because
  the `chromium` project runs with authentication disabled and every identity
  is an administrator.
- Mobile: touch drag on the handle using CDP `Input.dispatchTouchEvent`, following
  `e2e/tests/task/mobile-subtask-reparent-drag-drop.spec.ts`, and a touch scroll
  that starts outside the handle and scrolls the page (`001.2`).

Existing `agent-profile-layout.spec.ts` and `mobile-agent-profile-layout.spec.ts`
assert the unchanged toolbar test IDs:
`open-host-shell`, `rescan-agents-button`, `new-agent-button`.

## Work orders

- [ ] [Task 01: Profile order backend](task-01-profile-order-backend.md) (in progress: PostgreSQL fixture correction)
- [ ] [Task 02: Profile reorder UI without sorting](task-02-profile-order-ui.md) (in progress)

## Verification results

The results below are historical contributor reports from before integration
with current main. They are not verification of the combined review head.

Task 01's focused backend tests, SQLite race coverage, SQL guard, backend lint,
and store-package race suite passed. Two unchanged
`internal/testutil/envscan_test.go` failures remain in the broad backend suite;
PostgreSQL DSN-gated migration and concurrency tests were not run because
`KANDEV_TEST_POSTGRES_DSN` was unset.

The revised Task 02 implementation removes automatic name sorting and keeps
persisted order in Settings > Agents and its navigation tree. Selectors retain
their creation baseline, context-specific recency/default behavior, and
workspace-scoped option slots. Under pinned Node 24.19.0, the focused
profile-order suite passed 20 files and 233 tests. Full web lint, typecheck,
i18n checks and ratchet, formatting, desktop/mobile E2E, public-doc validation,
and specification validation passed. The implementation subtask reported the
auth E2E flow passing.

The full `mise exec -- pnpm test` run failed: 44 files, 70 tests, 21,543
passed, and 4 skipped. Reported failures did not include changed profile-order
test files; they included timeouts and test-fixture connection errors. The
focused profile-order suite, selector E2E, and reorder E2E passed. Leave the
full-suite failure visible rather than treating it as a green check.

Round-five review found workspace-scoped profile deletion could miss the
membership lock when promotion committed between its reads. The deletion path
now locks the candidate owner and stable profile identity, rereads, and retries
when owner or scope changes.

The full store-package race suite and focused regression passed; changed-backend
golangci-lint reported zero issues. The PostgreSQL case was skipped because
`KANDEV_TEST_POSTGRES_DSN` is unset. The plan remains `in_progress` because the
revised UI work order and environment-dependent checks remain open.

## Risks

- Adding repository methods breaks every fake of `store.Repository`; Task 01
  must update them all.
- The `sort_order = 0` default makes new profiles appear first after a manual
  order exists. This matches today's newest-first rule but may surprise a user
  who placed a profile first.
- Every flat-list and direct `GET /agents` selector projection must retain the
  creation baseline; otherwise Settings order can change a first-profile default.
- Order changes do not bump `agentProfiles.version`, so the agent-list
  resource's cached `response` can carry an older order. Every writer of the
  agent lists, including the hydrator, must reconcile with the known per-agent
  order revision; a writer that bypasses it reintroduces the stale-order bug.
- Each reorder adds a row to `agent_profile_orders` and a field to the agent DTO;
  clients older than the field treat the revision as `0` and keep today's behavior.
- A dnd-kit drag handle inside a row with an overlay link needs the handle above
  the link (`z-10`) so it does not open the profile.

## Integration review validation

The review fixup preserves current-main catalogue publication, enabled-intent
persistence, capability/runtime metadata, and Settings model-picker behavior.
It adds regression coverage for creation baselines during pending drags and
late acknowledgements, terminal rollback, the 15-second request timeout,
malformed timestamp handling, Office option preservation, and localized drag
announcements. Desktop handles use the shared 28 px size; phone and coarse
pointer handles retain 44 px hit targets. Backend regressions cover canceled
repository reads, equal/missing ranks, and create/delete after saved ordering.

Focused frontend tests run in a credential-free, nonroot, network-disabled
container. Broad frontend/backend, race, lint, build, typecheck, and rendered
desktop/mobile checks remain assigned to hosted CI. Host hooks and contributor
tooling are not executed during this review. The historical broad-suite
failures above remain visible; they are not attributed to unrelated tests
without exact source and log evidence.

## PostgreSQL validation recovery

PR #4373 merged externally before exact-head CI was terminal. The final
PostgreSQL job failed when expanded pools opened connections outside the owned
schema and membership subcases reused unique agent names. Task 01 is reopened
for the fixture correction and physical-connection regression in #4399. The
existing profile-order requirements and design remain authoritative; no
production behavior changes are part of that correction. Historical reports
above remain historical, and hosted corrective-head results are required.


## Corrective follow-up PR #4399

The original PR merged while this session's hosted gates were pending. Its final
head failed PostgreSQL race fixtures, frontend catalogue tests and selector E2E.
Task 01 repairs per-connection schema selection and per-subcase isolation. Task
02 repairs accepted new-owner publication after independent updates, guards
observed owner removal during save, reconciles Settings-only test expectations
and drives Quick Chat dismissal through its actual Escape interaction. Both
work orders remain in progress until the corrective head passes hosted checks.
