---
status: draft
system: tasks
requirements:
  - REQ-TASKS-ORPHANED-WORKSPACE-003
---

# Orphaned Workspace Marker Startup Repair System Design

## Purpose and boundaries

This design owns REQ-TASKS-ORPHANED-WORKSPACE-003 only: the boot-time pass that stamps
the marker onto the historical population no archive event will ever reach, and retracts
markers a crashed clear left behind. The read side (deriving `workspace_orphaned` and
delivering it on every task payload) is
[orphaned-workspace-task-indicator](orphaned-workspace-task-indicator.md); the write
primitive this pass is the heaviest consumer of is
[orphaned-workspace-guarded-metadata-write](orphaned-workspace-guarded-metadata-write.md),
which defines `SetTaskWorkspaceMetadataIfUnchanged` and every `OrphanWriteGuard` clause
named below. Read all three before changing any.

This is **not** a schema migration and must not live in `runMigrations()`. It adds no
table, column, index, or retention rule; the marker is already durable, so restart
behavior is unchanged. The feature-level failure table, including which bullets name this
pass as recovery owner, stays in the indicator design's `## Failure and recovery`.

## Components and responsibilities

| Component | Responsibility |
| --- | --- |
| `internal/task/service/handoff_workspace_orphan_repair.go` (new) | `RepairOrphanedWorkspaceMarkers`: both passes, their guard values, the publish |
| `internal/task/repository/sqlite/task.go` | `ListOrphanRepairCandidates`, `ListStaleOrphanMarkers`, the malformed-`metadata` count, and each one's Postgres branch |
| `internal/backendapp/helpers.go` | Invoke the repair inside `registerRoutes`, after `SetTaskEventPublisher` |

No frontend component changes: the repair reaches the client through the existing
`task.updated` payload and the boot payload, both owned by the indicator design.

## Startup repair

**Owner and wiring (AC-003.1).**
`internal/task/service/handoff_workspace_orphan_repair.go`,
`HandoffService.RepairOrphanedWorkspaceMarkers(ctx) error`, beside the mark and clear
paths whose predicate and key set it shares. Invoked from
`internal/backendapp/helpers.go` inside `registerRoutes`, **after
`SetTaskEventPublisher(p.taskSvc)` at `helpers.go:784`** — not at `NewHandoffService` on
`:765` — and still before the gateway accepts clients. Any point at or after `:784`
satisfies this: the `handoffSvc.Set*` block continues past it (`SetVacatedStepReconciler`
`:785`, `SetTaskResourceCleaner` `:805`) and the repair depends on none of it.
`s.eventPublisher` is nil until `:784`, so calling the repair in the 19 lines between
would make AC-003.5's `task.updated` never *attempted* rather than merely dropped
(AC-003.8's accepted case: no subscribers yet) — identical in the logs, only one
allowed. Calling it after client accept would instead risk a client painting
an unmarked board. `registerRoutes` takes no `ctx` and returns no error: use
`context.Background()` and log, per `helpers.go:1851`.

**Where the selections live.** `HandoffService.tasks` is the interface
`repository.TaskRepository`, so the service cannot run SQL. Both are repository
methods on `internal/task/repository/sqlite/task.go` —
`ListOrphanRepairCandidates(ctx)` and `ListStaleOrphanMarkers(ctx)` — reached through
one optional capability interface type-asserted off `s.tasks`, as `handoff_cascade.go`
does for `workspaceEnvironmentRepository`. A wiring without it runs neither selection:
the AC-003.11 skip path, logged at Info because it means a test-only wiring, not a
fault.

**Stamping selection.** It returns the child's **current `$.workspace` object**, not
just its id: one read supplies both the guard values and the base map the four keys
are stamped onto, so `mode` and unknown siblings survive the whole-object write. With
only `c.id, p.id` there is nothing to put in the guard, and passing `""` for a task
carrying a stale non-empty `orphaned_parent_id` loses the CAS every boot — AC-003.1's
own defect, via the write rather than the predicate.

```sql
SELECT c.id, p.id AS parent_id,
       json_extract(c.metadata,'$.workspace') AS workspace
FROM tasks c JOIN tasks p ON c.parent_id = p.id
WHERE c.archived_at IS NULL AND p.archived_at IS NOT NULL
  AND c.is_ephemeral = 0
  AND COALESCE(c.origin,'') != 'automation_run'
  AND json_valid(c.metadata)
  AND json_extract(c.metadata,'$.workspace.mode') = 'inherit_parent'
  AND json_type(c.metadata,'$.workspace.orphaned') IS NOT 'true'
  AND NOT EXISTS (SELECT 1 FROM task_environments e WHERE e.task_id = c.id)
ORDER BY c.created_at, c.id;
```

**`is_ephemeral` and `origin` are the producer's filter, not extra caution.** Both mark
sites reach their children through `ListChildren`, whose own `WHERE` carries
`AND t.is_ephemeral = 0` plus `andNotAutomationOriginT`
(`COALESCE(t.origin,'') != 'automation_run'`). A repair selecting without them marks a
strictly wider population than any archive does, breaking AC-003.2's "orphaned by repair
and orphaned by archive are one state" — silently, since every such row looks like a
legitimate candidate. The *clearing* selection deliberately omits them: retracting a
provably false claim is correct whatever the task's origin.

Each row's guard carries `RequireParentID = c.parent_id` and
`RequireTaskNotArchived = true` alongside `RequireParentArchivedID = p.id`, so a child
reparented or archived after selection matches zero rows instead of being stamped for a
parent it no longer has, or badged while archived.

**`json_valid` is load-bearing.** SQLite's `json_extract` and `json_type` raise
`malformed JSON` and abort the statement rather than returning NULL (verified:
`json_type('','$.a')` errors; `json_valid('')` → `0`). One empty-string `metadata` row
reachable by the query fails the whole `SELECT`, AC-003.11 skips the **entire** pass,
and it recurs every boot behind one Warn nobody would connect to "the feature has
never worked." **The same guard belongs on the clearing selection and in the guarded
write's `WHERE`**, in the `CASE WHEN json_valid(metadata) THEN … END` form the clearing
CTE below and the guarded-write design's `WHERE` both use. Excluding an unparseable row *agrees with* the producer:
`taskWorkspaceMode` yields `""` for a map that failed to unmarshal.

**Excluded rows need their own query.** Both selections filter malformed rows out in the
`WHERE` / CTE, so such a row never reaches a result set and the pass cannot warn about a
row it never saw — the "the feature has never worked" failure named above would have no
signal at all. So the repair runs one bounded count first and logs a **single** Warn
carrying it when non-zero, not one per row, which is unbounded:

```sql
SELECT COUNT(*) FROM tasks
WHERE metadata IS NOT NULL AND metadata <> '' AND metadata <> 'null'
  AND NOT json_valid(metadata);
```

The three excluded literals are deliberate: `NULL`, `''` and `'null'` are shapes the
guarded write's own three-way `CASE` handles, they carry no `$.workspace.mode`, and so
could never have been candidates — counting them would warn on a healthy install. Only
genuinely corrupt non-empty text can hide a candidate. Measured on SQLite 3.54.0 against a
six-row fixture (`NULL`, `''`, `'null'`, valid JSON, `'{oops'`, `'not json at all'`):
returns **2**, the two corrupt rows. On Postgres the count is skipped with one Debug line
— no `json_valid`, `pg_input_is_valid` needs PostgreSQL 16 against a floor of 15, and the
paragraph below records that no writer produces malformed non-empty text there. A failure
of the count query itself is logged at Warn and skips neither pass: it is diagnostic, not
a precondition.

**A bare `AND json_valid(...)` protects only what textually follows it.** SQLite
short-circuits `WHERE` terms left to right, so the stamping query is safe *because of
its term order*, not because the guard is present. Measured on 3.54.0 against a
malformed row that is a live child of an archived parent: `json_valid` first returns the
one qualifying row; the same terms with `json_valid` moved last abort with
`malformed JSON`. Two rules follow, and both are contract:

1. Inside a `WHERE`, `json_valid(<col>)` shall precede every `json_extract` /
   `json_type` on that column. A reordering — by hand, or by a query builder
   appending terms — silently reintroduces the abort, so the order is not cosmetic.
2. A **JOIN key, and any value a CTE projects for an enclosing query to join on**,
   shall be wrapped in `CASE WHEN json_valid(<col>) THEN … END`, which is
   position-independent (measured: the same `CASE` placed last still returns the
   qualifying row). A join condition is computed to form pairs rather than filtered, so
   no `WHERE` term can come first, and a CTE may be flattened into the enclosing join
   rather than materialized ahead of it.

Rule 2 is narrower than "any projection", deliberately: a plain `SELECT`'s own list is
evaluated only for rows its `WHERE` already admitted. That is why the stamping selection
projects `json_extract(c.metadata,'$.workspace')` unwrapped and is safe — measured on
3.54.0, it returns the qualifying row with a malformed sibling present in `tasks`. Do not
"fix" it. Rule 1 protects its `WHERE` terms; the `WHERE`, having already excluded every
malformed row, protects its projection. A JOIN key and a flattened CTE get no such prior
filter, which is the whole of rule 2.

The clearing selection needs rule 2 twice, for its join key and its `workspace`
projection, and its blast radius is much wider: **any** malformed `metadata` row anywhere
in `tasks` aborts the pass, not merely a live child of an archived parent. Test that
wider population.

Postgres has no `json_valid`, and `pg_input_is_valid` needs PostgreSQL 16 while
`docs/ARCHITECTURE.md:530` puts the floor at 15 (CI runs 16). That branch reuses the
same three-way `CASE` as its siblings, covering every bad value reachable here:
every writer marshals from a Go map or casts, so malformed non-empty text has no
producer here. Same residual as `detachTaskQuery`.

**The `orphaned` test is a type-and-value test (AC-003.1).** `json_extract` is wrong
twice over: an `IS NULL` predicate skips `"orphaned": false`, leaving it invisible
with no recovery path; and it returns SQL integer `1` for both JSON `true` and the
*number* `1`, so `<> 1` would skip `"orphaned": 1`, which the Go `.(bool)` assertion
reports as **not** orphaned. `json_type` is exactly equivalent to the Go strict test,
yielding `'true'` only for JSON `true` (marked, not selected) and `'text'`/
`'integer'`/`'false'`/`'null'`/SQL NULL for every unmarked shape, with `IS`/`IS NOT`
supplying the NULL-safe comparison the absent case needs.

Postgres must use `jsonb` equality, not text extraction:
`#> '{workspace,orphaned}' IS DISTINCT FROM 'true'::jsonb`, since
`jsonb_extract_path_text` flattens `true` and `"true"` onto one string. Every query
here needs the usual `dialect.IsPostgres` branch and an env-gated Postgres test.
The precedent for `json_extract(metadata,'$.workspace.mode')` is the **SQLite**
branch of `detachTaskQuery` (`task/repository/sqlite/task.go` ~1519; the
Postgres branch above it uses `jsonb_extract_path_text`/`jsonb_set`, do not
pattern-match on that) and `office/repository/sqlite/tasks.go:881`.

**Staleness (AC-003.10).** The list is stale the moment it returns, and the repair
does not re-read — a second read shrinks the window, never closes it. Every value the
predicate depends on is a clause of the guarded write instead (parent identity, parent
archive state, **the child's own archive state**, own environment, `mode`, stored
claim), so AC-003.10 holds with no window; a task that no longer qualifies matches zero
rows and is counted skipped.

The child's own archive state is named explicitly because it is the clause the
predicate opens with — the requirements' `## Terminology`: "a live, non-archived task" —
and the one a reader will assume `WHERE id = ?` already covers. It does not: `id = ?` pins *which*
row, never its state. The clause is `RequireTaskNotArchived`, set by the stamping paths
only. The clearing pass must NOT set it: AC-003.9a deliberately reaches archived
children, so setting it there would strand the very markers that pass exists to
retract. `is_ephemeral` and `origin` need no clause — neither is mutable on an existing
row by any path here, so there is no window.

**Clearing (AC-003.9).** After stamping, a narrower selection finds false claims.
The join key is extracted in a subquery, guarded by `json_valid`, so no malformed
row anywhere in `tasks` can abort it:

```sql
WITH claims AS (
  SELECT id, created_at,
         CASE WHEN json_valid(metadata)
              THEN json_extract(metadata,'$.workspace') END AS workspace,
         CASE WHEN json_valid(metadata)
               AND json_type(metadata,'$.workspace.orphaned_parent_id') = 'text'
              THEN json_extract(metadata,'$.workspace.orphaned_parent_id') END AS claim_id
  FROM tasks
  WHERE json_valid(metadata)
    AND json_type(metadata,'$.workspace.orphaned') IS 'true'
)
SELECT c.id, c.workspace
FROM claims c JOIN tasks p ON c.claim_id = p.id
WHERE p.archived_at IS NULL
ORDER BY c.created_at, c.id;
```

**Archived children are included, deliberately.** Unlike the stamping selection this
carries no `c.archived_at IS NULL` filter, mirroring the producer's clear path, which
uses `ListChildrenIncludingArchived` for a documented reason: a child can be archived
at the moment its parent is restored, and **nothing else ever revisits that child's
own marker** — its own later unarchive clears only its children, never itself. Filter
archived children out here and a failed clear on one leaves a permanent false marker
with no owner, which is exactly what AC-003.9 exists to prevent. Stamping keeps the
filter, because AC-003.1 marks only live tasks.

The `JOIN` keeps it narrow: a marker whose named parent row is absent does not match
(nothing renders it), and one whose named parent is still archived
does not match either. Only a marker naming a present, unarchived parent is cleared —
what a failed or crashed unarchive-clear leaves behind. Clearing drops the four keys,
preserves `mode`, and uses the same guarded write with
`RequireParentUnarchivedID = p.id` so a parent re-archived after selection cannot have
a valid marker torn off.

**Publishing what was actually stored (AC-003.5).** Neither selection yields a
`*models.Task`, and `TaskEventPublisher.PublishTaskUpdated`
(`handoff_service.go:274`) requires one. So for each row whose guarded write
**landed**, the repair calls `s.tasks.GetTask(ctx, id)` and publishes that. The
ordering is the contract, not an implementation detail: re-reading *before* the write,
or reusing the selection row, would broadcast the pre-write metadata — an explicit
`workspace_orphaned: false` immediately after storing `true`. `preserveOmittedField`
cannot repair that, because the key is present rather than omitted, so the stale value
would pin client-side until a reload. A write that lost its guard publishes nothing
(AC-005.5), so no read is issued for it. A failed `GetTask` is logged at Warn and the
pass continues: the marker is durably stored either way, and the next boot payload
carries it (AC-003.8).

**The two selections are independent**, and AC-003.11 applies to each: either
failure warns and skips only its own pass. Run stamping first for log legibility;
the passes commute, because the clearing predicate needs an unarchived named parent
and a marker stamping wrote names an archived one.

This is **not** a schema migration and must not live in `runMigrations()`. It writes
through the guarded-write seam plus `PublishTaskUpdated` so the kanban WS view
converges. No new table, column, index, or retention rule; the marker is already
durable, so restart behavior is unchanged.

## Observability

- The repair logs one Info line carrying the counts stamped and cleared — including zero
  for both, so a healthy install is distinguishable from a pass that did not run — plus
  one Warn per task it failed to write, per failed post-write `GetTask`, per selection
  that failed and skipped its pass, and **one** Warn carrying the malformed-`metadata`
  count when that count is non-zero (one line for the whole pass, not one per row). The
  one-Warn-per-pass rule and the failed-re-read Warn are AC-003.12, so both are
  assertable rather than resting on this design's discipline alone.
- A lost guard is Debug, not Warn: a concurrent writer won, which is correct behavior
  rather than a fault.
- No new metric: the population is bounded, and the logs answer "did the pass run and
  what did it touch".
