---
id: "01-shared-interface"
title: "Shared interface: flag, store, routes, constants, event and client"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-COORDINATORS-001
  - REQ-COORDINATOR-COORDINATORS-002
  - REQ-COORDINATOR-COORDINATORS-003
  - REQ-COORDINATOR-PROPOSALS-004
acceptance_criteria:
  - AC-COORDINATOR-COORDINATORS-001.1
  - AC-COORDINATOR-COORDINATORS-001.2
  - AC-COORDINATOR-COORDINATORS-001.3
  - AC-COORDINATOR-COORDINATORS-002.1
  - AC-COORDINATOR-COORDINATORS-002.2
  - AC-COORDINATOR-COORDINATORS-002.3
  - AC-COORDINATOR-COORDINATORS-002.4
  - AC-COORDINATOR-COORDINATORS-002.5
  - AC-COORDINATOR-COORDINATORS-002.6
  - AC-COORDINATOR-COORDINATORS-002.9
  - AC-COORDINATOR-COORDINATORS-003.1
  - AC-COORDINATOR-COORDINATORS-003.2
  - AC-COORDINATOR-COORDINATORS-003.3
  - AC-COORDINATOR-PROPOSALS-004.1
system_design:
  - ../../specs/coordinator/system-design/coordinators.md
  - ../../specs/coordinator/system-design/proposals.md
  - ../../specs/coordinator/system-design/needs-you.md
---

# Task 01: Shared Interface (WP-1)

## Summary

Land the contract every later work order builds against: the release toggle,
all three tables and their store methods, the coordinator CRUD, proposals read
and stalls read routes, request and response types for every route, the
shared constants, the `coordinator.updated` event and forwarder, and the typed
web client. The behaviour behind the remaining routes lands in the work orders
that own it. After this merges, changing any part of the contract goes through
the coordinators and proposals system designs, not a local edit.

## In scope

- `features.coordinator` through `/runtime-feature-flags`: registry entry in
  `internal/runtimeflags/registry.go` (restart required), `profiles.yaml`
  (`prod`/`dev` `"false"`, `e2e` `"true"`), config, the client `features`
  type, contract tests.
- `internal/coordinator` models and store with all three tables
  (`coordinators`, `coordinator_proposals`, `coordinator_stalls`) and their
  store methods: coordinator CRUD; proposal insert, conditional claim,
  settle, list and get; stall upsert (fenced on a strictly newer
  `last_event_at`), delete and list. No later work order adds a migration.
- `persistence/requiredstores` entry and an upgrade test from `v0.93.0` on
  SQLite and PostgreSQL.
- Service, validation and routes behind the flag: coordinator CRUD with the
  name, context, passthrough and missing-profile checks; `profileStatus` in
  `internal/coordinator/validate.go` and the `agent_profile_status` and
  `executor_profile_status` fields on the coordinator GET
  ([coordinators design](../../specs/coordinator/system-design/coordinators.md#validation));
  `open_proposals` on the coordinator list; the proposals list and get
  routes (without the stale-claim recovery on read, which is task 07's); the
  stalls route.
- Request and response types for every route of the three system designs,
  including the conversation, approve and reject routes whose handlers land in
  tasks 03 and 07, and the 409 body `coordinator_profile_unavailable`.
- Constants, declared and unused until their owners wire them:
  `TaskOriginCoordinator`, the `coordinator_id` metadata key,
  `mcpmode.Coordinator`, `SurfaceCoordinator`, the
  `coordinator.propose_task` action name.
- The `coordinator.updated` event type and payload and its gateway forwarder
  `internal/gateway/websocket/coordinator_notifications.go`. Publishing sites
  land with tasks 03, 04 and 07.
- `internal/backendapp/coordinator.go`: the store always initialised; service,
  routes and forwarder only when the flag is on; the startup pass's start
  time `T0` recorded before the routes register and the pass run in the
  background ([coordinators design](../../specs/coordinator/system-design/coordinators.md#flag-and-wiring)).
  One named registration function per later work order (conversation for
  task 03, subscribers for task 04, decisions for task 07), each empty here, so
  parallel PRs add lines without editing each other's. The startup pass itself
  is a fixed-order call to each of these three functions' startup hook (task
  03's conversation cleanup, task 04's stall pruning and `workspace.deleted`
  handling, task 07's stale-claim recovery); here every hook is a no-op, so the
  pass runs and does nothing until a later work order fills in its own
  function. No later work order edits the pass's call site or another order's
  function.
- Coordinator delete removes the row and its proposals in one transaction;
  conversation-task deletion is added in task 03.
- Web: `apps/web/lib/api/domains/coordinator-api.ts` with the types and one
  client function for every route, and the `coordinator.updated` payload type.

## Out of scope

- Any page or screen (tasks 02, 04, 06, 08).
- The conversation route, session and tool surface (task 03).
- Stall and workspace-deletion subscribers and startup pruning (task 04).
- Approve, reject and recovery (task 07).

`AC-COORDINATOR-COORDINATORS-001.2` is owned here for the routes; each UI work
order checks that its own surface is absent with the flag off.

## Mockup screenshots and scenarios

Screenshots: none. This work order has no UI; the requirement docs' screenshots
are cited by the work orders that render them.

Mockup scenario specs to port: none (see the plan's
[Mockup scenario to repo test](plan.md#mockup-scenario-to-repo-test)).

## Acceptance

- Every table, type, route shape, constant and event the later work orders
  need exists; the CRUD, proposals read and stalls read routes work through
  the API.
- With the flag off every coordinator route returns 404 and the store is still
  initialised with its rows intact.
- A `workspace.read` member lists and reads; its writes get 403.

## Verification

```bash
cd apps/backend && go test ./internal/coordinator/... ./internal/runtimeflags/... ./internal/persistence/... ./internal/backendapp/... ./internal/gateway/websocket/...
cd apps/backend && make lint
cd apps/web && pnpm test -- lib/api/domains/coordinator-api.test.ts
cd apps/web && pnpm run typecheck
```

Go tests cover: CRUD; name 1 to 60 after trimming; context at most 4,000;
passthrough refused; missing agent or executor profile refused; list order and
empty list; 403 for readers; foreign id 404; repeated delete 404; two
concurrent PATCH requests against the same coordinator commit last-write-wins,
so the next GET returns whichever request's fields committed last
(`AC-COORDINATOR-COORDINATORS-002.6`); flag off 404
with the store initialised and rows kept across a restart; `profileStatus`
over agent `missing`, agent `passthrough`, executor `missing` and a read error
other than not found (500, never `missing`); the store methods (the claim is
conditional on status and claim token, proposal list order and the 50-row
limit for `status=all`, the stall upsert fence); store and upgrade
conformance on SQLite and PostgreSQL; the forwarder delivers
`coordinator.updated` to the workspace's subscribers. Vitest covers the API
client.

## Likely files

- `profiles.yaml`
- `apps/backend/internal/runtimeflags/registry.go`
- `apps/backend/internal/coordinator/{models,store,repository,service,validate,handlers,dto,events}.go` and tests
- `apps/backend/internal/task/models/` (origin, surface constants)
- `apps/backend/internal/common/mcpmode/`
- `apps/backend/internal/persistence/requiredstores/catalog.go`
- `apps/backend/internal/persistence/storeconformance/` upgrade test
- `apps/backend/internal/gateway/websocket/coordinator_notifications.go`
- `apps/backend/internal/backendapp/coordinator.go`
- `apps/web/lib/api/domains/coordinator-api.ts` and test

## Dependencies

- WP-0 (this design package) has passed Review. G0 does not block building;
  this work order's upstream PR opens only after G0 is met (ADR G0 Status).
  While G0 is open the branch starts from WP-0's branch and rebases onto main
  when WP-0 merges.

## Risks

- The upgrade fixture must be created from the `v0.93.0` schema, not the
  current one, or the test proves nothing.
- The flag is restart-required; e2e toggles it through the profile, not a
  runtime override.
- A contract gap found later means a spec change and a follow-up to this work
  order, not a local edit in a dependent branch.
