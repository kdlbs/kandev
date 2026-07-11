---
status: done
created: 2026-06-23
owner: cfl
---

# TanStack Query Server State

## Why

Kandev's web UI currently stores most server-owned data in Zustand and refreshes
it through a mix of boot hydration, component fetch effects, WebSocket handlers,
and page-local refetch callbacks. That makes live data freshness hard to prove:
an event can reach the browser but update a store path the mounted UI no longer
reads, or a first-paint payload can be followed by a redundant fetch that races
with newer WebSocket state.

This feature makes TanStack Query the client authority for server state while
keeping Zustand for client-only UI state. The user-visible goal is that every
route still paints from Go boot data, stays live through WebSocket updates, and
recovers from missed events through explicit refetch/invalidation paths without
manual refresh.

## What

- Server-state reads use TanStack Query query keys and query option factories,
  not component-local fetch effects or Zustand server-state slices.
- Go boot payloads and `/api/v1/app-state` route bootstraps seed the query cache
  before dependent page effects run, preserving first-paint data behavior.
- WebSocket notifications update or invalidate the matching query cache entry.
  The UI reads the cache entry that the WebSocket bridge writes.
- Mutations update query cache through optimistic updates, mutation responses, or
  targeted invalidation. Failed optimistic updates roll back and surface the same
  toasts/errors as the current UI.
- Zustand remains available for client-only state: active route/sidebar state,
  dock layout, editor state, transient form state, terminal affordances, local
  preferences, and in-browser persistence that is not server-owned.
- WebSocket event accounting is enforced in E2E: strict runs detect events the
  backend sent but the frontend did not parse, and bridge audit detects parsed
  server-state events that did not touch the query cache unless explicitly
  allowlisted.
- The migration lands as one PR, but implementation is organized by domain so
  each domain can be reviewed and tested independently inside that PR.
- Existing desktop and mobile workflows continue to use the same pages, routes,
  and visible controls. The migration must not introduce desktop-only behavior.

## Data Model

This feature introduces no durable database tables. It changes frontend runtime
state ownership.

### Query Cache

TanStack Query owns server-state cache entries in the browser. Keys are array
tuples with stable domain prefixes:

```text
["features"]
["workspaces"]
["workspaces", workspaceId, "repos"]
["kanban", "workflows"]
["kanban", "workflows-list", workspaceId]
["session", sessionId, "messages"]
["session", sessionId, "turns"]
["session", "byTask", taskId]
["session", "byId", sessionId]
["office", workspaceId, "dashboard"]
["office", workspaceId, "tasksPaginated", filters]
["github", workspaceId, "prs"]
["settings", "userSettings"]
```

The implementation may add additional keys, but every server-state key MUST have
a typed key factory and a query option factory. Filter objects in keys MUST be
JSON-serializable. Sorting/grouping that is purely presentational SHOULD be done
through selectors rather than new fetch keys.

### Zustand State

Zustand is reduced to client-only state plus transitional indexes needed by UI
logic. Any remaining server-state path MUST be listed in the implementation
plan as either migrated, explicitly retained as client-only, or temporarily
retained with a removal task.

Examples of client-only retained paths include:

- route and selection state such as active task/session/workspace
- sidebar view order, collapsed subtasks, preview panel, dockview layout
- editor/comment draft state and sessionStorage-backed annotations
- local terminal interaction affordances
- WebSocket connection status

### WebSocket Accounting State

E2E-only accounting state is in-memory:

- backend `wsSentLog`: bounded ring buffer of sent WebSocket envelopes
- frontend `WsAccount`: bounded ring buffers of parsed envelopes by connection
  and by session
- frontend bridge audit: bounded ring buffer of query-bridge handler outcomes

No accounting buffer survives backend/browser restart.

## API Surface

No product HTTP API is added for TanStack Query itself. Existing HTTP and
WebSocket contracts continue to serve the same route data.

### Frontend Query Surface

Each domain exposes:

```ts
qk.<domain>.<keyFactory>(...)
<domain>QueryOptions.<resource>(...)
```

Components and hooks call `useQuery`, `useInfiniteQuery`, `useMutation`, and
`useQueryClient` through domain hooks rather than ad hoc fetch effects.

### Boot Payload Seeding

`window.__KANDEV_BOOT_PAYLOAD__` and `/api/v1/app-state?path=...` provide route
data and `Partial<AppState>` today. During this migration they also seed the
query cache from the same data before child hooks can decide to fetch.

The Go boot payload remains the first-paint source. A hard reload on a route
with preloaded data MUST NOT show an empty state that currently paints from
boot-hydrated Zustand.

### WebSocket Envelope Accounting

Strict E2E accounting adds fields to outbound WebSocket envelopes:

```json
{
  "connection_id": "server-generated socket id",
  "connection_seq": 42,
  "session_seq": 7
}
```

`session_seq` is present only when the server can identify a session scope.
The backend exposes an E2E-only endpoint under `/api/v1/_test/ws-sent` for
fixture comparison. The endpoint is mounted only when the E2E mock/test harness
is enabled.

### Bridge Audit

When E2E mock mode is enabled, the browser exposes bounded audit helpers:

```ts
window.__kandev_bridge_audit__?.()
window.__kandev_bridge_audit_clear__?.()
```

The audit records action, session/task ids when present, whether the bridge
touched the query cache, and mutation count. Production builds do not expose or
populate these helpers.

## State Machine

### Query Data Lifecycle

| State | Entered when | Outgoing transitions |
|---|---|---|
| `seeded` | boot payload or route bootstrap writes initial query data | `fresh`, `stale`, `inactive` |
| `fresh` | `staleTime` has not elapsed or data was just written | invalidation -> `stale`; no observers -> `inactive` |
| `stale` | key is invalidated or `staleTime` elapses | observer refetch -> `fresh`; no observers -> `inactive` |
| `inactive` | no mounted observers use the key | remount -> `fresh` or refetch; `gcTime` elapsed -> removed |
| `optimistic` | mutation writes a temporary value | success -> `fresh`; failure -> rollback + error |

Default query behavior is conservative for Kandev:

- normal server state defaults to `staleTime: 30_000`
- active session chat/turn state uses a longer stale window so background
  refetches do not clobber live streams
- global refetch-on-window-focus/reconnect is disabled by default; specific
  hooks opt in when recovery requires it
- mutations do not retry unless the mutation is known to be idempotent

### WebSocket Event Lifecycle

1. Backend stamps an outbound envelope and records it in `wsSentLog`.
2. Browser parses the envelope and records it in `WsAccount`.
3. WebSocket client dispatches notification handlers.
4. Query bridge writes or invalidates the matching query cache.
5. Mounted UI observes the same query key and re-renders.
6. E2E fixture compares sent vs parsed envelopes and parsed vs applied bridge
   entries at test teardown.

## Failure Modes

- **Boot payload missing or malformed**: route falls back to the current
  `/api/v1/app-state` or domain fetch path, then seeds the query cache.
- **HTTP query failure**: query exposes loading/error state and the existing UI
  error/toast behavior remains visible.
- **Mutation failure**: optimistic cache changes roll back and the same user
  draft/input restoration rules remain in place.
- **WebSocket reconnect**: client resubscribes to task/session/run/user/system
  topics and invalidates affected query keys when reconnect recovery requires a
  server snapshot.
- **WebSocket event dropped in E2E**: strict accounting fails the test and names
  the missing event/session.
- **WebSocket event parsed but not applied**: bridge audit fails the test unless
  the action is documented in the bridge skipped allowlist.
- **High-frequency streams**: shell/process/terminal streams do not write every
  chunk through TanStack Query if that would create a render bottleneck. They
  use bounded stream buffers or retained terminal transport state, and the
  exception is documented in the bridge allowlist.

## Persistence Guarantees

- Query cache is in-memory only. It is reseeded from Go boot payloads, app-state
  fetches, HTTP queries, and WebSocket notifications after reload.
- User-visible durable state remains on the backend, in existing tables/files,
  or in already-approved browser storage paths.
- E2E accounting buffers are test-only and reset between browser contexts and
  backend restarts.
- WebSocket subscriptions are not durable. On reconnect, the client resends
  subscription intent and refetches/invalidate query keys as needed.

## Scenarios

- **GIVEN** a task page served by Go boot payload, **WHEN** the React app mounts,
  **THEN** the task, task sessions, messages, worktree data, repositories, and
  relevant settings are visible from the query cache before child hooks issue
  duplicate fetches.

- **GIVEN** a kanban board already hydrated from the boot payload, **WHEN**
  `task.updated` arrives over WebSocket, **THEN** the matching kanban query cache
  entry updates and the visible card changes without a manual refresh.

- **GIVEN** an office dashboard is open, **WHEN** an agent run creates activity
  and updates metrics, **THEN** dashboard query keys are invalidated or patched
  and cards/activity update without polling.

- **GIVEN** an office tasks list has loaded multiple pages, **WHEN** a task is
  created, moved, or updated, **THEN** the paginated query cache invalidates or
  reconciles without losing pagination state or current filters.

- **GIVEN** a session receives `session.message.added`, **WHEN** the event is
  parsed, **THEN** the message-list query cache updates and both desktop and
  mobile chat surfaces render the new message.

- **GIVEN** a sent user prompt's `session.message.added` event is intentionally
  dropped in an E2E test, **WHEN** the prompt send request resolves, **THEN** the
  accepted prompt remains visible through mutation response or refetch recovery.

- **GIVEN** two sessions are subscribed concurrently, **WHEN** WebSocket events
  arrive interleaved, **THEN** connection sequence and per-session sequence
  accounting detect missing or misrouted events.

- **GIVEN** a WebSocket event with `session_id` is parsed in strict E2E mode,
  **WHEN** no query bridge handler touches the cache, **THEN** the test fails
  unless the action is documented as Zustand-only/client-only/high-frequency.

- **GIVEN** a user switches workspaces, **WHEN** workspace-scoped query keys are
  active, **THEN** stale data from the previous workspace is not rendered for the
  new workspace.

- **GIVEN** the browser reloads while an agent session has settled, **WHEN** the
  route boots again, **THEN** query cache is reseeded from backend state and the
  chat/session UI matches the persisted session without waiting for another WS
  event.

## Out Of Scope

- Backend event replay or guaranteed catch-up after disconnect.
- Replacing Zustand for client-only UI state.
- Rewriting product UI layouts or navigation beyond state-source changes.
- Persisting TanStack Query cache to localStorage/sessionStorage.
- Production WebSocket ack metrics; this spec requires E2E accounting, not a
  production observability project.

## Verification

Rebase follow-up completed locally on 2026-06-27 after rebasing the migration
branch onto `origin/main` at `7728ddb3b`:

- Migrated the upstream task repository WebSocket preservation fix into the
  Query bridge and removed the legacy `task-repositories` WebSocket helper.
- Migrated the new session layout active-session lookup to query-backed
  `useSession`, keeping the upstream rowless-session fallback behavior.
- Updated rebased tests to seed/query through TanStack Query instead of deleted
  Zustand server-state slices.
- `rtk make fmt` passed.
- `rtk pnpm --dir apps/web typecheck` passed.
- `rtk pnpm --dir apps/web lint` passed.
- Focused web unit slice passed 16 files / 95 tests, covering Query bridge,
  session state/layout, topbar metrics, mobile selectors, PR chip, and rebased
  mobile/task components.
- Docker E2E desktop focused slice passed 8 tests with strict WS accounting:
  `rtk pnpm --dir apps/web e2e:docker tests/session/multi-session.spec.ts tests/system/ws-event-accounting.spec.ts tests/task/repository-selector-scroll.spec.ts`.
- Docker E2E mobile focused slice passed 8 tests with strict WS accounting:
  `rtk pnpm --dir apps/web e2e:docker --no-build --project mobile-chrome tests/chat/mobile-model-selector.spec.ts tests/cli-mode/mobile-passthrough-composer.spec.ts tests/pr/mobile-pr-ci-chip.spec.ts tests/mobile-zoom.spec.ts`.
- Docker E2E multi-session UX slice passed 7 tests with strict WS accounting:
  `rtk pnpm --dir apps/web e2e:docker --no-build tests/session/multi-session-ux.spec.ts`.

Final strict QA completed locally:

- `rtk make fmt` passed.
- `rtk make typecheck test lint` passed.
- `rtk pnpm --dir apps/web e2e:docker --shards 3` passed the full desktop
  Docker suite with strict WS accounting.
- `rtk pnpm --dir apps/web e2e:docker --project mobile-chrome` passed 78 mobile
  Docker tests with strict WS accounting.
- `rtk pnpm --dir apps/web e2e:docker --project routing` passed 7 routing
  Docker tests with strict WS accounting.
- `rtk env KANDEV_E2E_CONTAINERS=1 pnpm --dir apps/web e2e --project=containers`
  passed 99 container-backed Docker/SSH executor tests / 1 skipped.

Post-rebase CI follow-up completed locally on 2026-07-11 after rebasing onto
`origin/main` at `9094659a`:

- Preserved both upstream cancel-release handling and the migration's missing
  session recovery in the only rebase conflict; the lifecycle package passed
  765 tests.
- Fixed `task.plan.updated` Query bridge projection so implementation-started
  markers remain visible immediately after the toolbar action.
- Made accepted walkthrough prompts cancel an older latest-message fetch and
  update the Query cache as well as the retained live message index.
- Focused unit regressions passed 2 files / 10 tests.
- Docker E2E passed the desktop plan-toolbar implementation spec and the rebuilt
  mobile plan-toolbar/walkthrough slice (4 tests) with strict WS accounting.
- A pre-fix mobile stress reproduction also completed 20/20 focused repetitions;
  the walkthrough cache race was confirmed by the failing unit regression rather
  than a deterministic isolated browser failure.
