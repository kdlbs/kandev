---
created: 2026-09-09
status: draft
requirements:
  - REQ-OFFICE-WAKE-WAVE-IDENTITY-001
  - REQ-OFFICE-WAKE-WAVE-IDENTITY-002
  - REQ-OFFICE-WAKE-WAVE-IDENTITY-003
  - REQ-OFFICE-WAKE-WAVE-IDENTITY-004
system_design:
  - ../../specs/office/system-design/parent-wake-wave-identity.md
legacy_specs: []
---

# Implementation Plan: Parent Wake Wave Identity

## Overview

Regenerated 2026-09-09 against the frozen spec (round-2 amend). The prior
plan under this directory (session `d0842a54`) predates both spec amends: it
had one column/encoding, a wider wave-member predicate, one classification
site, and no coalescing exclusion or payload-parity obligation. This
supersedes it in full.

Give the `task_children_completed` wake a single durable identity, computed
identically by all four producers, persisted on the run in two encodings, and
compared by the backstop instead of a timestamp. Implementation order:

1. **Foundation** (Task 01): the pure wave-string/wave-key derivation and the
   new wave-member repository read every producer needs. No behavior change.
2. **Persistence** (Task 02): the two `runs` columns, the partial unique
   index, the second unique-violation classifier, and `runs/service`'s
   coalescing exclusion. Still no producer sets these columns yet, so this is
   also behavior-inert until Task 03 lands — verified by service-level tests
   that construct requests directly.
3. **Producers**, one work order per identity site, in dependency order
   forced by the shared engine seam:
   - Task 03: P1 cascade (`office/scheduler`) — direct insert path, needs its
     own classification site and its own coalescing guard, and owns the
     payload-parity read (AC-...-002.16) since it never touches the engine.
   - Task 04: the engine seam plus P2 edge and P3 backstop, which share it.
     `OnChildrenCompletedPayload` gains the two encodings; `QueueRunCallback`
     reads them off the trigger payload it already carries end-to-end
     (`ActionInput.Payload` is `HandleInput.Payload` verbatim — no new
     parameter threading through `HandleInput`/`ActionInput`/engine
     `QueueRunRequest` is needed beyond the two new fields on that one
     request struct and its `runsServiceEngineAdapter` copy).
   - Task 05: P4 orchestrator, built on Task 04's seam. No new read; re-sorts
     the read it already has.
4. **Admission** (Task 06): `ListStuckParents` compares wave identity instead
   of timestamp, once producers actually write it.
5. **Closing coverage** (Task 07): the race test, cross-producer parity,
   coalescing, read-skew, and admission suites that need every producer and
   the admission rewrite to exist simultaneously.

Tasks 03, 04 and 05 are not parallel-safe with each other even though they
touch different packages: all three add wiring in
`internal/backendapp/main.go` (03 wires a new `WorkflowStepGetter` setter for
payload parity; 04 changes `runsServiceEngineAdapter`; 05 depends on 04's
`OnChildrenCompletedPayload` fields). Land them in order.

## Scope

### In scope

- `internal/office/waveidentity` (new leaf package): `WaveString` and
  `WaveKey`, pure functions of `(parentID string, memberIDs []string)`
  (REQ-001).
- A new office-repository read returning wave members `(id, state)` ordered
  by id, applying the shared not-archived/not-ephemeral/not-automation-origin
  predicate (REQ-001, REQ-002.15).
- `runs.wake_wave_key` / `runs.wake_wave_string` columns, partial unique index
  `idx_run_wake_wave(wake_wave_key, agent_profile_id)`, `models.Run` fields,
  `CreateRunTx` column list, a second classifier
  (`IsWakeWaveUniqueViolation`), `runs/service`'s dedupe-on-conflict handling,
  and its coalescing exclusion for wave-carrying requests/rows (REQ-002).
- Threading the two encodings through all four producers, including P1's own
  `CreateRun` classification site (`office/scheduler.SchedulerService`) and
  its coalescing guard, and the shared engine seam
  (`OnChildrenCompletedPayload` → `ActionInput.Payload` → `QueueRunCallback`
  → `engine.QueueRunRequest` → `runsServiceEngineAdapter` →
  `runs/service.QueueRunRequest`) for P2/P3/P4 (REQ-002, REQ-004).
- The terminality-confirming wave-member read (AC-...-002.15): P1/P2/P3 read
  `(id, state)` as their last child-state read before queueing and queue
  nothing if any member is non-terminal; P4 already satisfies this via
  `readyChildCompletionRows`.
- Payload parity (AC-...-002.16): the cascade resolves the parent's current
  workflow step, reads its `on_children_completed` `queue_run` action
  payload (via `engine.CompileStep`, already used by
  `orchestrator.workflowStore.LoadStep`), and merges it into its own run
  payload; queues without it, logging the omission, when it cannot resolve.
- `internal/office/repository/sqlite.ListStuckParents` rewrite: a
  dialect-aware wave-string aggregate (new `internal/db/dialect` helper), a
  wave-member existence gate, and the three-clause, status-qualified
  admission arm from the system design (REQ-003).
- `parent_wake_deduped_total` expvar, incremented at both classification
  sites.
- Regression coverage per the spec's Testing section.

### Out of scope

(Mirrors the requirements document's "Out of scope" verbatim; not repeated
here beyond a pointer.) Reopen-and-recomplete re-waking; id-set-reuse via
unarchive; task `updated_at` stamping; the queued-or-claimed arm's
whole-second comparison (`b7e29d7c`); porting `ListStuckParents`' SQLite-only
`json_extract`/aggregate expressions elsewhere in the same query to dialect
form (`53c24173`, and this work must not widen that gap); `on_children_completed`
trigger firing conditions, payload shape, or editor surface; UI.
`wakeOperationID` (P2/P3's existing operation id) is **not** deleted or
redefined — the round-2 spec keeps every producer's existing identifier
untouched, unlike the stale plan this supersedes.

## Technical approach

### Foundation: `internal/office/waveidentity` and the wave-member read

New package, dependency-free beyond `crypto/sha256`, `encoding/hex`,
`strings`:

```go
func WaveString(parentID string, memberIDs []string) string
func WaveKey(parentID string, memberIDs []string) string // = WaveKey(WaveString(...))
```

`WaveString` = `parentID + "|" + strings.Join(memberIDs, ",")`. `WaveKey` =
`"task_children_completed:" + parentID + ":" + hex(sha256(waveString))`.
Callers order `memberIDs` ascending by `tasks.id` and pass only wave members;
the functions do no I/O and return no error (AC-...-001.1, .2, .10, .11).
Sibling of `internal/office/costs/modelsdev`, which `internal/orchestrator`
already imports (`orchestrator/model_info.go`) without crossing the
`orchestrator`/`office/service` boundary.

New read, `ListWaveMembers`, in `internal/office/repository/sqlite/blockers.go`
beside `ListChildStates` (same file, same `ChildState`-shaped row):

```go
func (r *Repository) ListWaveMembers(ctx context.Context, parentID string) ([]ChildState, error)
```

```sql
SELECT id, COALESCE(state, '') AS state FROM tasks
WHERE parent_id = ? AND archived_at IS NULL AND is_ephemeral = 0
  AND COALESCE(origin, '') != 'automation_run'
ORDER BY id
```

This is the predicate `task/repository/sqlite.ListChildCompletionRows`
already applies (`andNotAutomationOrigin`), reused here because P4 forces the
choice (design, "Which children count"). A cross-producer test in Task 01
asserts this method and `ListChildCompletionRows` return the same id set for
one fixture parent — the equality AC-...-001.8 depends on.

### Persistence

- `internal/office/repository/sqlite/base.go`, `createRunTables` (~line
  329-387): add `wake_wave_key TEXT NOT NULL DEFAULT ''` and
  `wake_wave_string TEXT NOT NULL DEFAULT ''` to the inline `CREATE TABLE
  runs`, plus `CREATE UNIQUE INDEX IF NOT EXISTS idx_run_wake_wave ON
  runs(wake_wave_key, agent_profile_id) WHERE wake_wave_key <> ''` beside
  `idx_run_idempotency`.
- `internal/office/repository/sqlite/base_migrations.go`: new
  `migrateWakeWaveColumns()`, following `migrateContinuationScope`'s shape
  (two `r.migrate.Apply` ALTERs + one partial-index `CREATE UNIQUE INDEX IF
  NOT EXISTS`, no backfill), called from `runMigrations()` next to
  `migrateParentWakeReceiptColumns()`.
- `internal/office/models/models.go`, `Run` struct (~line 381): add
  `WakeWaveKey string \`db:"wake_wave_key"\`` and `WakeWaveString string
  \`db:"wake_wave_string"\``.
- `internal/runs/repository/sqlite/runs.go`, `CreateRunTx` (~line 31-52): add
  both columns to the `INSERT INTO runs` list and bound args.
- `internal/runs/repository/sqlite/idempotency_violation.go`: add
  `IsWakeWaveUniqueViolation(err error) bool`, mirroring
  `IsIdempotencyKeyUniqueViolation` — PostgreSQL: typed `pgErr.ConstraintName
  == "idx_run_wake_wave"`; SQLite: message substring `"UNIQUE constraint
  failed: runs.wake_wave_key, runs.agent_profile_id"` (confirm go-sqlite3's
  exact composite-index message empirically before hardcoding — this is the
  same risk the superseded plan flagged, now scoped to one new function
  instead of a rename).
- `internal/runs/service/service.go`: `QueueRunRequest` (~line 85-92) gains
  `WakeWaveKey`, `WakeWaveString string`; `insertRun` (~line 249-274) sets
  them on the `*models.Run{}` literal; `insertRun`'s `CreateRun` error
  handling gains a second branch — `IsWakeWaveUniqueViolation(err)` returns a
  second sentinel (e.g. `errWakeWaveKeyConflict`), and `QueueRun` (~line
  221-235) maps it to `QueueOutcomeDeduped` exactly like
  `errIdempotencyKeyConflict`, with no error and no retry
  (AC-...-002.4/.5/.6). `shouldCoalesceRun` (~line 316-318) gains `&&
  req.WakeWaveKey == ""` (AC-...-002.14, first half).
- `internal/runs/repository/sqlite/runs.go`, `CoalesceRun` (~line 449-488):
  add `AND (wake_wave_key = '' OR wake_wave_key IS NULL)` — actually the
  column is `NOT NULL DEFAULT ''`, so `AND wake_wave_key = ''` — to the
  candidate-row `WHERE`, closing AC-...-002.14's second half (a queued
  wave-carrying run is never merged into, regardless of which caller
  invokes `CoalesceRun`).
- `internal/office/scheduler/run.go`, `SchedulerService.QueueRun` (~line
  249-303): the same two guards for P1's own coalesce-and-insert path — skip
  `ss.repo.CoalesceRun` when a wave key is supplied, and classify
  `ss.repo.CreateRun`'s error with `IsWakeWaveUniqueViolation` into a nil
  return + debug log (mirroring the existing `CheckIdempotencyKey` hit
  handling), not the `enqueue run: %w` wrap that would make .002.4 fail for
  P1 specifically (this is why Task 03 needs its own classification work,
  not just Task 02's).
- `internal/office/service/wake_metrics.go`: add `parentWakeDedupedTotal =
  expvar.NewInt("parent_wake_deduped_total")`, incremented at both
  classification branches above. `runs/service` does not import
  `office/service`; wire the increment via a small exported callback var (or
  equivalent seam already used for `parent_wake_*` counters — check
  `wake_metrics.go`'s existing wiring pattern before choosing) rather than a
  new cross-package dependency.

### P1 cascade (Task 03)

`internal/office/scheduler/reactivity.go`, `cascadeChildrenCompleted`
(~line 384-421): keeps `ListChildStates` and its terminal-state loop
unchanged (narrowing that read would change cascade's readiness check,
forbidden by AC-...-004.7) and adds, after that loop confirms all-terminal:

1. Call `ListWaveMembers(ctx, task.ParentID)` — the terminality-confirming
   last read (AC-...-002.15). Any non-terminal member (a race with the
   `ListChildStates` read) or a read error: queue nothing, return.
2. Empty result: queue nothing, return (AC-...-001.7; live once the
   predicate is narrower than `ListChildStates`, e.g. an all-ephemeral child
   set).
3. Derive `waveidentity.WaveString`/`WaveKey` from the sorted member ids.
4. Resolve payload parity: `GetTaskWorkflowStepID(ctx, task.ParentID)`
   (`office/repository/sqlite/participants.go:55`, already used elsewhere in
   this package's blast radius) → a new small interface,
   `WorkflowStepGetter.GetStep(ctx, stepID) (*wfmodels.WorkflowStep, error)`
   (satisfied by `workflow/service.Service.GetStep`, wired via a new
   `SchedulerService.SetWorkflowStepGetter` setter, called once at boot next
   to `SetTaskStarter`/`SetResolver`) → `engine.CompileStep` (already
   exported, already used by `orchestrator/workflow_store.go:226`) →
   look up the step's `TriggerOnChildrenCompleted` actions for one of kind
   `queue_run` with a non-nil `Payload`. Any failure at any of these steps:
   log the omission at debug and continue without a merged payload — the
   wake is unconditional (AC-...-004.2) and outranks the optional payload.
5. `RunContext` (~line 90-122) gains `WaveKey string`, `WaveString string`
   (JSON `"-"`, like `IdempotencyKey`) and a merge point for the resolved
   action payload with the same override precedence `queueRunPayload`
   applies (workflow-authored keys win over the trigger's own). Since
   `RunContext` marshals as a fixed struct rather than a generic map, the
   simplest implementation is: marshal `RunContext` first, then if a merged
   payload map is non-empty, unmarshal to `map[string]any`, overlay the
   merged keys, re-marshal — implemented inside `encodeRunContext` or a
   thin wrapper around it, whichever keeps `QueueRunCtx` readable.
6. `QueueRunCtx` → `QueueRun` (~line 246-324) thread `WaveKey`/`WaveString`
   as **new required parameters** (do not smuggle them through `RunContext`
   alone into the low-level `QueueRun`, which today takes discrete scalar
   args) onto the inserted `models.Run{WakeWaveKey, WakeWaveString}`.
   `childrenCompletedIdempotencyKey` (~line 436-443) is unchanged and stays
   the run's `idempotency_key` — additive, not replaced (design,
   "Producers").

### Engine seam, P2 edge, P3 backstop (Task 04)

- `internal/workflow/engine/payloads.go`: `OnChildrenCompletedPayload`
  (~line 38) gains `WaveKey string`, `WaveString string`. Not read by
  `idempotencyKey` or `queueActionDigest` (phase2_callbacks.go) — those stay
  untouched, per the design's "narrow seam" decision.
- `internal/workflow/engine/adapters.go`, `QueueRunRequest` (~line 45-52):
  add `WakeWaveKey`, `WakeWaveString string`.
- `internal/workflow/engine/phase2_callbacks.go`, `QueueRunCallback.Execute`
  (~line 81-107): after building `req := QueueRunRequest{...}`, type-assert
  `in.Payload.(OnChildrenCompletedPayload)` and copy `WaveKey`/`WaveString`
  onto `req` when present. This is the only engine-package call site that
  changes — `HandleInput`, `ActionInput`, and `Engine.executeCallback`
  already carry `Payload any` end-to-end unmodified
  (`engine.go:429-437` copies `in.Payload` verbatim into `ActionInput`), so
  no new field needs to thread through those three types.
- `internal/backendapp/main.go`, `runsServiceEngineAdapter.QueueRun`
  (~line 1810-1822): copy `WakeWaveKey`/`WakeWaveString` from the engine's
  `QueueRunRequest` to the runs-service `QueueRunRequest`.
- `internal/office/service/event_subscribers.go`, `queueChildrenCompletedRun`
  (~line 1116-1151): after `AreAllChildrenTerminal`, add the wave-member read
  (`ListWaveMembers`) as the terminality confirmation; on empty or
  non-terminal, queue nothing (return nil, matching the existing
  `!allDone` early return). `childSetKey`/`wakeOperationID` computation is
  **unchanged** — the operation id keeps its existing state-inclusive
  derivation. Build `engine.OnChildrenCompletedPayload{ChildSummaries:
  summaries, WaveKey: ..., WaveString: ...}` from the new read's sorted ids.
- `internal/office/service/scheduler_wake_reconciler.go`,
  `reconcileOne`/`buildPayload` (~line 93-177): same addition — call
  `ListWaveMembers` before dispatch (in addition to, not instead of, the
  existing `GetChildSetKey` re-reads that guard the receipt write — those
  keep using the state-inclusive key per the design's "Receipts" section),
  reject on non-terminal/empty, and set `WaveKey`/`WaveString` on the
  payload `buildPayload` returns. `wakeOperationID` is unchanged.

### P4 orchestrator (Task 05)

`internal/orchestrator/event_handlers_children_completed.go`:
`childCompletionOperationID` (~line 325-345) is **unchanged** — the
operation ledger keeps its existing per-row state/step/terminal/`updated_at`
derivation (AC-...-004.3). `childCompletionPayload` (~line 306-316) gains a
`parentID` parameter; before building `ChildSummaries` it copies `rows`,
sorts the copy ascending by `ID` (the source orders by `created_at, id`), and
sets `WaveKey`/`WaveString` from `waveidentity.WaveString/WaveKey(parentID,
sortedIDs)`. No new read and no new filter — `readyChildCompletionRows`
already rejects `len(rows) == 0` and already establishes terminality from
these same rows (AC-...-002.15 satisfied by construction, per the design).

### Backstop admission (Task 06)

`internal/office/repository/sqlite/wake_receipts.go`, `ListStuckParents`
(~line 101-175):

- New `internal/db/dialect` helper beside `JSONExtract` (e.g.
  `OrderedIDConcat(driver, table, parentCol, idCol string) string` or similar
  — name it to fit the file's existing signature style), branching:
  SQLite `GROUP_CONCAT(c.id, ',')` fed by an `ORDER BY id` subquery (the
  `child_set_key` idiom already in this file); PostgreSQL `string_agg(c.id,
  ',' ORDER BY c.id)`. Only this new fragment is dialect-branched — the
  query's existing `json_extract`/unbranched `GROUP_CONCAT` stay as they are
  (out of scope per `53c24173`).
- Add a wave-string CTE column: `p.id || '|' || COALESCE(<helper output over
  the wave-member predicate>, '')`, computed for every row (a pre-gate
  intermediate for a parent with no members, per Terminology).
- Add the wave-member existence gate: a second `EXISTS` beside the existing
  archived-only one, over `archived_at IS NULL AND is_ephemeral = 0 AND
  COALESCE(origin, '') != 'automation_run'` (AC-...-003.9).
- Rewrite the second `NOT EXISTS` arm into three named clauses (design,
  "Backstop admission"): `status IN ('queued','claimed')` blocks
  unconditionally; `status IN ('finished','failed','cancelled') AND
  wake_wave_string = <current wave string>` blocks; `NOT EXISTS (... WHERE
  wake_wave_key <> '') AND status IN ('finished','failed','cancelled') AND
  requested_at >= newest_child_updated_at` is the parent-scoped legacy
  fallback (AC-...-003.5/.6/.10) — the "no keyed run at all for this parent"
  condition must be evaluated once per parent, not per row, per .003.6.
- `StuckParentCandidate` (~line 21-27) gains no new field unless the caller
  needs the wave string for logging; the comparison lives entirely inside
  the SQL per AC-...-003.8.

### Observability

Already covered under Persistence (`parent_wake_deduped_total`).

## Tests

| Acceptance criteria | Test |
| --- | --- |
| AC-...-001.1-.3, .5, .6, .9-.13 | `internal/office/waveidentity/waveidentity_test.go`: deterministic and order-sensitive per the caller contract; `|`-separated, not NUL; digest-of-string relationship; empty slice documented as never called; no I/O. |
| AC-...-001.4, .5 | `internal/office/scheduler/reactivity_children_completed_test.go` (existing terminal-edit and non-state-edit cases) continue to pass unmodified. |
| AC-...-001.7, .8 | New cross-producer parity test: `ListWaveMembers` and `ListChildCompletionRows` return the same id set for one fixture parent, including an ephemeral/automation-origin child; an all-excluded child set derives no wave. |
| AC-...-002.1-.3, .7-.9, .11-.13 | `internal/runs/service/service_test.go`: `TestQueueRun_DedupesOnWakeWaveKeyIndexRace` (mirrors `TestQueueRun_DedupesOnIdempotencyIndexRace`, ~line 578); a different agent and a different wave-member set each get their own row; retry (`ScheduleRetry`) is untouched by the constraint. |
| AC-...-002.4, .5 | `internal/runs/repository/sqlite/idempotency_violation_test.go` (new, pattern from the existing file) for `IsWakeWaveUniqueViolation`; `internal/runs/service/service_postgres_test.go`: Postgres twin of the race test. |
| AC-...-002.6 | Same race test asserts no time bound (insert the second request well after the first). |
| AC-...-002.10, .002.16 | Payload-parity test: cascade attaches the workflow-authored `queue_run` payload byte-for-byte matching what the engine path would attach for an equivalent step; and still queues without it when the step lookup fails. |
| AC-...-002.14 | Coalescing test: a second request for a different parent, same agent, inside the coalescing window, with a wave key, does not merge into the first parent's queued (also wave-keyed) run — two rows exist. |
| AC-...-002.15 | Read-skew test: a wave-member read returning one non-terminal member queues nothing and records no identity; a later genuine wave for the same parent is still delivered. |
| AC-...-003.1-.10 | `internal/office/repository/sqlite/wake_receipts_test.go`: delivered-and-unchanged wave is not a candidate; non-state child edit after delivery is not a candidate; wave-member change after delivery is a candidate (and an id-set-reuse-to-a-delivered-identity is listed but queues nothing downstream); queued/claimed run blocks regardless of wave; a legacy run with an empty `wake_wave_key` is judged by the timestamp rule, scoped per parent even alongside a keyed run for the same parent; a failed run's wave-scoped block survives an unrelated child edit and is lifted only by a wave-member change; a parent whose only children are excluded is not a candidate. |
| AC-...-004.1-.7 | The full race test: cascade (P1) racing one reconciler (P3) tick for the same parent through `office/scheduler`'s own `QueueRun` path (not only `runs/service`), asserting exactly one `runs` row and no failure logged for the loser. Existing `TestWakeOperationID_UnifiedAcrossEdgeAndReconcilerPaths` and the orchestrator's operation-ledger tests continue to pass unmodified, confirming P2/P3's `wakeOperationID` and P4's `childCompletionOperationID` are untouched. |

## E2E tests

None. No user-visible control, view, or setting (requirements document,
"Out of scope": "User interface"). The effect is observable only as the
absence of a duplicate or spurious run in the Office Runs list, which the
spec's E2E decision explicitly leaves to Go-level coverage.

## Work orders

- [x] [Task 01: Wave identity primitives](task-01-wave-identity-primitives.md)
- [x] [Task 02: Persist wave identity and classify its unique violation](task-02-wave-identity-persistence.md)
- [x] [Task 03: Wire cascade (P1) onto wave identity](task-03-wire-cascade-producer.md)
- [ ] [Task 04: Wire engine-routed producers (P2, P3) onto wave identity](task-04-wire-engine-routed-producers.md)
- [ ] [Task 05: Wire orchestrator (P4) onto wave identity](task-05-wire-orchestrator-producer.md)
- [ ] [Task 06: Backstop admission compares wave identity](task-06-backstop-admission.md)
- [ ] [Task 07: Race, parity, and regression coverage](task-07-race-and-regression-coverage.md)

## Verification results

Pending.

## Risks

- **SQLite composite-unique-index violation message shape.**
  `IsWakeWaveUniqueViolation`'s SQLite branch needs the exact go-sqlite3
  message for a two-column unique index, confirmed empirically in Task 02
  before hardcoding (same risk the superseded plan flagged for a
  single-column rename; now isolated to one new function).
- **Cross-package `parent_wake_deduped_total` increment.** `runs/service`
  does not import `office/service`; Task 02 picks the least-invasive seam
  (existing `wake_metrics.go` wiring pattern, if one already crosses this
  boundary, otherwise a small callback var).
- **Payload-parity merge shape.** `RunContext` is a fixed struct, not a
  generic map, so merging an arbitrary workflow-authored payload into it
  needs a marshal/overlay/re-marshal step Task 03 must design carefully to
  avoid disturbing `CoalesceRun`'s existing byte-for-byte payload comparison
  for non-wave-carrying runs.
- **Three sequential, non-parallel producer work orders (03, 04, 05)** share
  touches to `internal/backendapp/main.go`. Land them one at a time.

## Open questions

None outstanding — the frozen spec resolves every prior open question (see
the spec's own review history for the closed rounds).
