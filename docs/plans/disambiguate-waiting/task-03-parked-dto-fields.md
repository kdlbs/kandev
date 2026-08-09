---
task: 03
title: "Parked DTO/wire/TS fields, hardcoded false/0"
wave: 0
status: done
spec_acs: AC-36, AC-37, AC-27, AC-77, AC-83, AC-84
---

# Task 03 — Parked DTO/wire/TS fields (hardcoded false/0)

Add all parked projection fields to every serialization boundary, hardcoded to `false`/`0`.
This lets the frontend compile and lets subsequent waves turn the values real without
further structural changes. Hardcoded fields make AC-36 / AC-37 / AC-27's DTO clause
vacuously green — that is expected for Wave 0 and documented in the spec's dependency note.

## Backend: session DTO

**`internal/task/dto/dto.go`** — `TaskSessionDTO` and `TaskSessionSummaryDTO`:

```go
// Parked-on-background-work projection (runtime-only, never persisted).
ParkedOnBackgroundWork bool   `json:"parked_on_background_work"`
ParkedEpoch            int64  `json:"parked_epoch"`            // backend process-start ns
ParkedRevision         uint64 `json:"parked_revision"`
```

**`internal/task/dto/cancellation_pending.go`** (or new `parked.go` alongside it):

Add `EnrichParked` and `EnrichParkedSummary` helpers, mirroring `EnrichCancellationPending`
/ `EnrichCancellationPendingSummary`. Define a `ParkedProjectionProvider` interface:

```go
type ParkedProjectionProvider interface {
    ParkedProjectionSnapshot(sessionID string) (parked bool, epoch int64, revision uint64)
}

func EnrichParked(session *TaskSessionDTO, provider ParkedProjectionProvider) {
    if session == nil || provider == nil {
        return
    }
    session.ParkedOnBackgroundWork,
        session.ParkedEpoch,
        session.ParkedRevision = provider.ParkedProjectionSnapshot(session.ID)
}
```

For Wave 0, the provider is nil (orchestrator doesn't implement it yet), so
`EnrichParked` no-ops. Task-07 wires the real provider.

## Backend: task DTO

**`internal/task/dto/dto.go`** — `TaskDTO`:

```go
// Task-level OR of all session parked states.
ParkedOnBackgroundWork bool   `json:"parked_on_background_work"`
ParkedRevision         uint64 `json:"parked_revision"`
```

No `parked_epoch` on the task DTO — the epoch is per-backend-process, not per-session.
The client uses `parked_epoch` from the session notification to detect restarts (AC-77).

Add an `EnrichTaskParked` helper and a `TaskParkedProvider` interface in the same file or
`parked.go`:

```go
type TaskParkedProvider interface {
    TaskParkedSnapshot(taskID string) (parked bool, revision uint64)
}

func EnrichTaskParked(task *TaskDTO, provider TaskParkedProvider) { ... }
```

## Backend: wire fields in `pkg/api/v1/`

If the project exposes protobuf or openapi wire types, add the fields there too. If tasks
are serialized directly from `TaskDTO`, this may be a no-op. Verify by grepping for where
`CancellationPending` is wired at the HTTP layer and mirror that pattern for parked fields.

## Frontend: Wire/DTO Task types

**`apps/web/lib/types/backend.ts`** — Wire `Task` (snake_case fields as received from
the backend HTTP/WS payload):

```ts
parked_on_background_work: boolean
parked_epoch: number           // int64 as JS number (ns — only epoch-comparison, no arithmetic)
parked_revision: number        // uint64 as JS number
```

**`apps/web/lib/types/http.ts`** — DTO `Task` (camelCase):

```ts
parkedOnBackgroundWork: boolean
parkedEpoch: number
parkedRevision: number
```

Session-scoped notification carries the same three fields; add to the session notification
wire type if one exists separately from the task wire type.

## Frontend: Board / Kanban shape

**`apps/web/lib/state/slices/kanban/types.ts:84`** — `KanbanState["tasks"][number]`:

```ts
parkedOnBackgroundWork: boolean
parkedRevision: number
```

(No `parkedEpoch` on the board shape — the board does not need restart detection; that
lives at the merge-helper layer added in task-06.)

## Frontend: TaskSwitcherItem shape

**`apps/web/components/task/task-switcher-types.ts:14`** — `TaskSwitcherItem`:

```ts
parkedOnBackgroundWork: boolean
```

## Hardcoded defaults everywhere

All producers (`toKanbanTask`, kanban.ts projections, `snapshotToState`, `buildSidebarItem`,
`toSheetItem`) default to `parkedOnBackgroundWork: false`. Task-06 replaces the defaults
with real reads from the wire fields.

`mergeTaskUpdate` and `mergeCancellationProjection` analogues: add field-scoped discard
logic stub (returns false) that task-06 fills in with the epoch-comparison rule.

## Tests to write

- **DTO JSON roundtrip**: verify `TaskSessionDTO` serializes `parked_on_background_work`,
  `parked_epoch`, and `parked_revision`; verify `EnrichParked` with a nil provider leaves
  them at zero values; verify with a real `ParkedProjectionProvider` returning non-zero
  values.

- **TS type compilation**: add parked fields to one fixture object in each test file that
  constructs a full `Task` wire object — the TypeScript compiler will catch omissions.

## Acceptance criteria closed (vacuously)

- **AC-36** (session DTO carries parked fields): fields present, hardcoded false/0.
- **AC-37** (task DTO carries task-level parked field): fields present, hardcoded false/0.
- **AC-27** (DTO clause): field present.
- **AC-77** (epoch present): `parked_epoch` field present in session DTO.
- **AC-83**, **AC-84** (producer completeness): producers exist with `false` defaults —
  these ACs fully close when task-06 wires real values and task-07 makes them non-trivial.
