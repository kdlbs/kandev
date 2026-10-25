---
status: draft
system: tasks
requirements:
  - REQ-TASKS-ORPHANED-WORKSPACE-005
---

# Guarded Workspace Metadata Write System Design

## Purpose and boundaries

This design owns REQ-TASKS-ORPHANED-WORKSPACE-005 alone: making every write to a
task's `metadata.workspace` sub-map compare-and-set, so a marker, a `mode`
normalization, and a clear cannot last-writer-win over one another.

It is a sibling of
[orphaned-workspace-task-indicator](orphaned-workspace-task-indicator.md), which owns
REQ-001 and REQ-002 (the derived DTO boolean and the board marker), and of
[orphaned-workspace-marker-startup-repair](orphaned-workspace-marker-startup-repair.md),
which owns REQ-003. The split is by subject, not by size: the primitive defined
here is a **repository write** shared by four call sites across three service files,
and three of the four have nothing to do with rendering a marker — site 4 is the
parent-delete reparent path. Anything touching `$.workspace` should read this file,
including work that never renders anything.

What this design does **not** change: the producer's predicate (which tasks get
marked) or its call sites, both shipped in `84c83c3d5` (#3235) and frozen by the
requirements' `## Out of scope`. Narrowing an unguarded whole-sub-map write to a
guarded one changes how a decision is recorded, never which tasks get marked. No
call site is added or removed, and no key name changes.

**One rule carries that claim, and it is the seam a reviewer should press on.** A
`Require*` clause is set where it only *enforces* the frozen predicate at write time
instead of one query earlier, and omitted where it would *change* which tasks get marked.
The predicate is the requirements' `## Terminology` sentence — *"a live, non-archived task
whose `metadata.workspace.mode` is `inherit_parent`, **whose parent is archived**, and
which has no `task_environments` row of its own"* — so a child reparented, given its own
environment, or archived between `ListChildren` and the write no longer satisfies it, and
a clause that no-ops the write excludes only tasks the predicate already excluded. The one
place the rule omits a clause is mark site 1's `RequireNoOwnEnvironment`, because
`HandoffService` skips that check entirely when its `envRepo` is nil. Each of the three
clauses is worked through, per caller, below the clause table.

The startup-repair design (REQ-003) is the heaviest consumer of the primitive defined
here; its per-pass guard values are specified there, the clause contract they must satisfy
below.

### Concurrency on the workspace sub-map

AC-005.2 is a real, reproducible defect. Both
`HandoffService.updateWorkspaceMetadata` and `Service.updateTaskWorkspaceMetadata`
read `task.Metadata["workspace"]` into a Go map, mutate it, and write the **whole
sub-map** back via `SetTaskMetadataKey(ctx, task.ID, "workspace", workspace)`. The
statement is atomic, but the *value* came from a stale read, and mark and clear both
target `$.workspace` with no version column, compare-and-set predicate, or mutex.
Two interleaved archive/unarchive operations therefore last-writer-win over the whole
sub-map, which can resurrect a cleared marker, drop a fresh one, or lose a concurrent
`mode` write.

#### The mechanism, pinned

Add **one** guarded write and use it from all four callers above — a
compare-and-set variant of the existing primitive, modeled on
`SetTaskMetadataKeyIfNotArchived`, whose doc comment states the principle: *"the
check and the write are one statement."*

```go
// OrphanWriteGuard is the state the caller observed. The two CAS fields are
// ALWAYS compared, including when empty: "" is the meaningful expected value
// "no string was stored", never an omission. Each of the five Require* fields
// adds one AND only when it is non-zero.
type OrphanWriteGuard struct {
	ExpectedOrphanedParentID  string // CAS, always compared; "" means "no string claim was stored"
	ExpectedMode              string // CAS, always compared; "" means "no string mode was stored"
	RequireParentArchivedID   string // "" omits the clause
	RequireParentUnarchivedID string // "" omits the clause
	RequireParentID           string // "" omits; CAS on tasks.parent_id
	RequireNoOwnEnvironment   bool   // false omits the clause
	RequireTaskNotArchived    bool   // false omits; stamping paths only, never clearing
}

// SetTaskWorkspaceMetadataIfUnchanged writes the whole $.workspace object only
// when every clause the guard asks for still holds. Reports whether it landed.
func (r *Repository) SetTaskWorkspaceMetadataIfUnchanged(
	ctx context.Context, taskID string,
	guard OrphanWriteGuard, value map[string]interface{},
) (bool, error)
```

The `SET` clause is **unchanged** from `SetTaskMetadataKey`: the same top-level
`$.workspace` / `ARRAY['workspace']` write with the same
`CASE WHEN metadata IS NULL OR metadata = 'null' OR metadata = '' THEN '{}'` guard.
Only the `WHERE` grows. The two CAS clauses are always present — an empty
`ExpectedOrphanedParentID` or `ExpectedMode` is compared against `''`, not dropped — and
the **five** `Require*` clauses appear only when their field is non-zero.

**Do not "optimize" an empty CAS field into an omitted clause.** Empty is the dominant
case, not an edge: a first-time mark has no stored claim, so `ExpectedOrphanedParentID`
is `""` on every one of them. Dropping the clause there is invisible to the obvious test
— the write still lands, because a guard with fewer clauses matches *more* rows, not
fewer — while AC-005.3's "a losing writer leaves the stored marker untouched" is
silently gone. The truth table below exists for exactly this reason: `''` on both sides
is a comparison that must happen, not a clause that may be skipped.

```sql
-- SQLite. json_valid() is required, not decorative: json_extract on a
-- metadata of '' raises "malformed JSON" and fails the whole statement.
-- json_type(...) = 'text' is equally required: see "The CAS is type-aware".
WHERE id = ?
  AND COALESCE(CASE WHEN json_valid(metadata)
        AND json_type(metadata,'$.workspace.orphaned_parent_id') = 'text'
        THEN json_extract(metadata,'$.workspace.orphaned_parent_id') END,'') = ?
  AND COALESCE(CASE WHEN json_valid(metadata)
        AND json_type(metadata,'$.workspace.mode') = 'text'
        THEN json_extract(metadata,'$.workspace.mode') END,'') = ?
  -- when RequireParentID != "":            AND parent_id = ?
  -- when RequireParentArchivedID != "":
  AND EXISTS (SELECT 1 FROM tasks p WHERE p.id = ? AND p.archived_at IS NOT NULL)
  -- when RequireParentUnarchivedID != "":  same EXISTS, archived_at IS NULL
  -- when RequireNoOwnEnvironment:
  AND NOT EXISTS (SELECT 1 FROM task_environments e WHERE e.task_id = ?)
  -- when RequireTaskNotArchived:          AND archived_at IS NULL
```

**The CAS is type-aware, and must be.** Callers fill both CAS fields with Go comma-ok
string assertions (`ws[k].(string)`, as `taskWorkspaceMode`
`handoff_context.go:161-168` does), which yield `""` for a stored value that is absent,
null, numeric, or an object. Plain `json_extract` returns that value *typed*, so a
metadata carrying `"orphaned_parent_id": 42` would compare SQL integer `42` against
`''`, match zero rows, and lose the guard on **every** boot, leaving a qualifying task
permanently unstamped behind a Debug line. The indicator design's `## Security` states
why a non-string `orphaned_parent_id` is reachable: task metadata is user-writable
through the generic PATCH surface, which preserves only the deferred-launch key.
Gating each extraction on `json_type(...) = 'text'` makes SQL mirror the Go assertion:
any non-string collapses to `''` on both sides, so the guard compares like with like.

Postgres needs the `SET` clause's own three-way `CASE` as its base — not a bare
`metadata::jsonb`, which errors on `''` — and the same type gate. **The gate must sit
INSIDE the value expression, not beside it as a conjunct.** Written as an ordinary
`AND jsonb_typeof(...) = 'string'`, an *absent* claim fails the gate instead of collapsing
to `''`, and absent is the dominant case: every first-time mark stores no
`orphaned_parent_id`, so every mark on Postgres would lose its guard forever behind a
Debug line. Pinned form, once per CAS key, with `M` standing for the `SET` clause's
`CASE WHEN metadata IS NULL OR metadata = 'null' OR metadata = '' THEN '{}'::jsonb ELSE metadata::jsonb END`:

```sql
COALESCE(CASE WHEN jsonb_typeof(M -> 'workspace' -> '<key>') = 'string'
              THEN M #>> ARRAY['workspace','<key>'] END, '') = ?
```

`<key>` is a **literal written into the SQL**, not a bind parameter — there are exactly
two, `orphaned_parent_id` and `mode` — matching the SQLite branch, which writes its
`'$.workspace.orphaned_parent_id'` paths literally for the same reason. Only the compared
value on the right-hand side binds.

Both dialects then agree, value for value. This table is the contract Build tests against;
`''` on both sides is what lets a `""` guard match everything that is not a stored string.

| Stored `$.workspace.<key>` | SQLite clause | Postgres clause |
| --- | --- | --- |
| absent, or `$.workspace` absent / not an object | `''` | `''` |
| JSON `null` | `''` | `''` |
| number, bool, object, array (e.g. `42`) | `''` | `''` |
| string `"p1"` | `'p1'` | `'p1'` |
| `metadata` NULL, `''`, or `'null'` | `''` | `''` (via `M`) |
| `metadata` unparseable non-empty text | `''` (`json_valid` is false) | **raises** — no writer produces this shape; same residual as `detachTaskQuery` |

Then the same `EXISTS` clauses. `SetTaskMetadataKeyIfNoActiveSession` already carries a
`NOT EXISTS (SELECT 1 FROM task_sessions ...)` in both dialects, so this is no new
capability.

**Who passes which clauses** — the contract; getting it wrong is how a frozen
predicate changes by accident.

| Caller | CAS claim + mode | Parent-state clause | `RequireParentID` | `RequireNoOwnEnvironment` | `RequireTaskNotArchived` |
| --- | --- | --- | --- | --- | --- |
| Mark site 1 (`HandoffService`) | yes | `Archived` = archiving parent | **yes** | **no** | **yes** |
| Mark site 2 (`Service`) | yes | `Archived` = archiving parent | **yes** | **yes** | **yes** |
| Repair, stamping pass | yes | `Archived` = archived parent | **yes** | **yes** | **yes** |
| Repair, clearing pass | yes | `Unarchived` = named parent | no | no | **no** |
| Clear site 3 | yes | `Unarchived` = named parent | no | no | **no** |
| Delete normalization, site 4 | yes | no | no | no | **no** |

**`RequireTaskNotArchived` splits stamping from clearing, and that split is the whole
rule.** Every stamping path sets it; no clearing path may. The predicate opens with "a
**live, non-archived** task" (requirements `## Terminology`), and `WHERE id = ?` pins
which row, never its state — so without this clause a child archived between selection
and write is still stamped, badging a task no board renders. Both mark sites select
through `ListChildren`, which already filters `archived_at IS NULL`, so the clause
enforces a predicate they already hold rather than changing it, exactly as
`RequireParentID` does. The three clearing rows withhold it deliberately: AC-003.9a has
the repair's clearing pass reach archived children, and site 3 and site 4 both read
through `ListChildrenIncludingArchived`, so setting it there would strand precisely the
markers those paths exist to retract or invalidate.

**`RequireNoOwnEnvironment` is withheld from mark site 1 ALONE. `RequireParentID` is
withheld from nothing — every stamping path sets it.** These were bundled two ways over:
first both clauses under one rationale, then both mark sites in one table row. Neither
bundle survives the code, and the second is why site 1 and site 2 now have their own rows.

`RequireNoOwnEnvironment` is withheld only where the Go check is **conditional**.
`HandoffService` skips the own-environment lookup entirely when its type-asserted
`envRepo` is nil (`handoff_workspace_orphan.go:96`, `if envRepo != nil`), so an
unconditional SQL clause on site 1 would change *which tasks get marked* in that wiring —
the one predicate change the freeze genuinely forbids.

**Site 2 is the opposite case, and gets the clause.**
`Service.markOrphanedInheritParentChild` (`service_tasks.go:2405`) dereferences
`s.taskEnvironments.GetTaskEnvironmentByTaskID` unconditionally; it has no nil-skip path,
so the clause cannot change which tasks it marks — it only enforces, at write time, a
check the Go code already always performs. What it closes is the same check-then-act
window `RequireParentID` closes on that same caller: the child acquires its own
`task_environments` row between the Go lookup and the write, and an unguarded stamp lands
a marker on a task that fails the requirements' `## Terminology` predicate ("no
`task_environments` row of its own") at the instant the marker is written. Site 2 runs on
every scheduled auto-archive and every workflow delete (callers named under
`### The unguarded write has FOUR callers, not two`), so that window is reached by
unattended machinery rather than being hypothetical. The repair
sets the clause for a third reason: this spec, not the
producer, defines the repair's predicate, and defines it in SQL.

`RequireParentID` is a different animal. Neither mark site re-checks `parent_id`: it comes
from `ListChildren(archived.ID)` at selection time, which is exactly the selection-time
identity the next paragraph argues is insufficient. Adding `AND parent_id = ?` does not
change the predicate — absent a reparent it is trivially true for every row `ListChildren`
returned — it *enforces* the predicate the Go read already established, at the instant of
the write. The only case where it bites is a reparent inside that window, and there
marking is wrong by the requirements' own `## Terminology` definition, which requires the
task's parent to be archived — not merely that *some* archived task was its parent when a
list was built. So the freeze on "which tasks get marked" is honored by adding the clause,
not by omitting it; `## Purpose and boundaries` states this reconciliation in full.

**Why every stamping path needs `RequireParentID`.** The guard proves the named parent is
archived, not that it is still *this child's* parent. The reparent that defeats the CAS
writes **no metadata at all**: `ReparentDirectChildren`
(`sqlite/task.go:2012`) is a bare `UPDATE tasks SET parent_id = ?, updated_at = ? WHERE
parent_id = ?`, called from `handoff_cascade.go:913`. It leaves `$.workspace`
byte-identical, so both CAS clauses still match and the write would stamp
`orphaned_parent_id` with a parent the task no longer has — a marker the clearing pass
cannot reach, since it only clears markers naming an *unarchived* parent. A permanent
false badge, closed by `AND parent_id = ?` in the same statement.

**The other reparent path is already blocked, and the test must NOT be built on it.**
`UpdateTask` (`sqlite/task.go:627`) writes `parent_id` and `metadata` in one statement,
but the only service path reparenting through it (`service_tasks.go:1917`) calls
`normalizeWorkspaceModeAfterReparent` five lines later at `:1922`, flipping `mode` to
`shared_group`. Every candidate row was selected *because* its `mode` is
`inherit_parent`, so there `ExpectedMode` already matches zero rows without
`RequireParentID`. A reparent-race test wired through `UpdateTask` passes for the wrong
reason and makes this clause look redundant. Build it on `ReparentDirectChildren`.
The window is identical on the repair's stamping pass and on mark sites 1 and 2 (a list
built one query earlier either way), so one clause closes both, and **the value is already
in hand:** the repair passes each row's selected `c.parent_id`; mark sites 1 and 2 pass
`archived.ID`, the same id they pass as `RequireParentArchivedID`, since
`ListChildren(archived.ID)` is what put the child in the list.

**Why the clearing pass needs `RequireParentUnarchivedID`.** The mirror case: the
selection sees the named parent unarchived, it is re-archived before the write, and an
unguarded clear removes a marker that had become valid again. Self-healing on the next
boot is not the contract — the guarantee is per-statement, and the clear side gets the
same treatment as the mark side rather than an unexplained asymmetry.

**Site 3 sets it too**, against a premise that does not survive the code: its clear is
*not* atomic with the parent's own unarchive. `handoff_cascade.go` unarchives one task,
then calls `clearOrphanedInheritParentChildren`, which does one
`ListChildrenIncludingArchived` read followed by **one write per child**
(`handoff_workspace_orphan.go:149`, `:171`); `unarchiveManualRoot` has the same shape, so
an arbitrary number of statements separates the parent's unarchive from any given child's
clear write. A re-archive landing in that window stamps a fresh, legitimate marker naming
the same parent with the same `mode` — both CAS clauses still match — and an unguarded
clear strips it, leaving a task that cannot start and carries no marker: the one state
this capability exists to prevent. The clause costs nothing absent a race, since site 3
runs precisely because the parent was just unarchived, so the `EXISTS` holds. A lost guard
here is AC-005.5's no-op success, correctly so: the marker left standing is the valid one,
and the parent's next unarchive clears it. Site 3 passes
`RequireParentUnarchivedID = parentID`, the parent whose unarchive triggered the flow and
the same id the existing Go check compares the stored claim against
(`clearOrphanedInheritParentChild`, `handoff_workspace_orphan.go:164`), so the clause
names the parent the marker itself names, never a different ancestor's.

Why this mechanism is pinned rather than optional:

- **AC-005.2 holds in one statement, not eventually.** `RequireParentArchivedID`
  closes it. Without that clause, *archive selects the child → unarchive commits and
  finds no marker to clear → archive writes a marker naming a now-unarchived parent*
  passes both CAS predicates and lands the state AC-005.2 forbids. Inside the same
  `UPDATE` there is no window: an unarchived parent means zero rows matched.
- **AC-003.10's write-time re-check is structural**, with no preceding read — but
  only for the values the guard actually carries. That is the whole of the repair's
  stamping predicate *because* `RequireParentID` and `RequireNoOwnEnvironment` were
  added alongside the parent-state clause; a guard missing any one of them re-opens
  the window for that value. Add a clause whenever the predicate grows.
- **AC-005.3 holds structurally**, because one statement writes the whole marker,
  so two concurrent markers cannot interleave into a mixed tuple. The rejected
  alternative — four independent leaf-path writes — cannot satisfy it, and needs a
  nested-path capability that does not exist: `SetTaskMetadataKey` passes the key as
  `jsonPath(key)` in SQLite and a *single element* of `ARRAY[?]` in Postgres.
- **`workspace.mode` cannot be lost**: a concurrent `mode` change fails the CAS and
  the write no-ops. A lost write is a no-op success for the three marker writers
  (AC-005.5), an error for the fourth.

**The wrappers change on both sides: they take a guard, and they report whether the
write landed.** `HandoffService.updateWorkspaceMetadata` and
`Service.updateTaskWorkspaceMetadata` take `(ctx, task)` and return a bare `error` today,
and all three marker call sites publish on a nil error
(`handoff_workspace_orphan.go:111-118` and `:171-178`; `service_tasks.go:2422-2427`).
Under a CAS "no error" no longer means "it landed", and the guard has to reach the
repository through this seam, so both signatures become:

```go
func (s *HandoffService) updateWorkspaceMetadata(
	ctx context.Context, task *models.Task, guard OrphanWriteGuard,
) (landed bool, err error)

func (s *Service) updateTaskWorkspaceMetadata(
	ctx context.Context, task *models.Task, guard OrphanWriteGuard,
) (landed bool, err error)
```

**Every publish moves inside `if landed`** — publishing a lost write broadcasts a marker
that was never stored, which `preserveOmittedField` then pins client-side.

**There is no `UpdateTask` fallback.** Both wrappers today fall back to
`s.tasks.UpdateTask(ctx, task)` when the `taskMetadataKeySetter` assertion misses.
That is an **unguarded whole-row** write from a stale read, silently voiding AC-005.2
and AC-005.3. The CAS setter gets its own optional interface, and a `s.tasks` without
it **skips the write and logs Warn** with `landed = false`: no event, and the next
repair pass stamps the task. Production wires the concrete SQLite repository and
`handoff_workspace_orphan_test.go` uses the real one, so this path is unreachable
there.

**Guard values are captured BEFORE the in-memory mutation, and the wrapper is not
where they are read.** Every call site mutates the task's own `workspace` map *in place*
and only then calls the wrapper: `stampOrphanedWorkspaceMetadata(workspace, archived.ID)`
then `updateWorkspaceMetadata` (`handoff_workspace_orphan.go:108-111`,
`service_tasks.go:2418-2422`); `clearOrphanedWorkspaceMetadata(workspace)` then the same
call (`:167-171`); `workspace["mode"] = shared_group` **plus, under AC-005.6, a
`clearOrphanedWorkspaceMetadata(workspace)` beside it** then the same call
(`handoff_cascade.go:901-904`). Site 4 now mutates twice, so capture the guard before
the FIRST of the two, not between them: reading it after the clear would compare `''`
against the stored claim and match zero rows, the same failure this paragraph describes
for the other sites. The wrapper re-derives `workspace` from `task.Metadata`,
so what it can see is the map *after* the mutation — precisely the state the CAS must not
be given. `ExpectedOrphanedParentID` is the field a stamp and a clear each overwrite;
`ExpectedMode` is the field site 4 overwrites.

Filling the guard inside the wrapper is therefore wrong, and wrong in the worst available
way. A first-time mark would compare the parent id it just wrote against a stored `''` and
match **zero rows**; a clear would compare `''` against the stored claim and match zero
rows. Measured on SQLite 3.54.0 against the `WHERE` above, for a child carrying no prior
marker: guard `''` matches 1 row, guard `'p1'` matches 0. Because AC-005.5 makes a lost
marker write a **no-op success** logged at Debug with no event, every mark and every clear
would stop working with no error, no Warn, and nothing in the existing tests failing.

So each caller builds its guard from the map as read, before touching it:

```go
// called on the workspace map as read — before stamp / clear / normalize mutates it
func observedWorkspaceGuard(workspace map[string]interface{}) OrphanWriteGuard {
	claim, _ := workspace[orphanedParentIDKey].(string)
	mode, _ := workspace["mode"].(string)
	return OrphanWriteGuard{ExpectedOrphanedParentID: claim, ExpectedMode: mode}
}
```

Those comma-ok assertions are exactly what the type-aware CAS mirrors, so a stored
non-string yields `""` on the Go side and `''` on the SQL side and the two still compare
equal. A `nil` map is safe to pass and needs no guard of its own: Go reads from a nil map
return zero values, so `observedWorkspaceGuard(nil)` is the all-`""` guard, which is the
correct observation of "nothing was stored". The caller then sets whichever optional
clauses its row of the table above assigns, and passes the guard alongside the
already-mutated map. The repair builds its guard the
same way, from the `$.workspace` object its selection returned, before stamping the four
keys onto it.

**The observable consequence, which Build must test:** an uncontended guarded write
LANDS. `SetTaskWorkspaceMetadataIfUnchanged` returning `landed = false` when nothing
changed concurrently is a defect, not a tolerated outcome — and asserting it is the only
cheap way to catch a guard filled from post-mutation values.

Testing: assert `landed == true` and one row affected for a mark on an unmarked task and
for a clear on a marked one; then unit tests for each guard field's win/lose branches on
SQLite — a stored non-string `orphaned_parent_id`, a reparent between select and write
(built on `ReparentDirectChildren`, not on `UpdateTask`, for the reason given above), a
parent whose archive state flips, and a child that acquires its own `task_environments`
row between the Go lookup and the write, and — for `RequireTaskNotArchived` — a child
archived between selection and write, which must match zero rows on all three stamping
paths and must still be written on the clearing paths. AC-005.6 needs its own test: a
marked `inherit_parent` child whose parent is non-cascade deleted ends with
`mode = shared_group` AND no orphan keys, in one write. The archive-state-flip case is **scoped to clear
site 3 as well as the repair's clearing pass**, since both carry
`RequireParentUnarchivedID`; the own-environment case is **scoped to mark site 2 as well
as the repair's stamping pass**, since those are the two callers that set
`RequireNoOwnEnvironment`. Drive the site-2 test through `Service.ArchiveTask` directly,
never through the MCP archive tool, which reaches site 1 instead. Mark site 1, which does
not set the clause, is the negative case: it
still marks such a child when its `envRepo` is nil, and a test asserting otherwise would
be asserting the predicate change the freeze forbids. Plus an env-gated Postgres test,
the pairing `task_launch_error_postgres_test.go` uses for `RemoveTaskMetadataKeyIfStamp`.

### The unguarded write has FOUR callers, not two

The *count* is the missable part. AC-005.2 is right that there are exactly **two**
independent *mark* implementations — but the unguarded whole-sub-map primitive they
use has **four** callers, and every one must move onto the guarded write or the
race stays live on the path left behind.

| # | Call site | Owner | Writes |
| --- | --- | --- | --- |
| 1 | `handoff_workspace_orphan.go:111` | `HandoffService` mark | marker |
| 2 | `service_tasks.go:2422` | `Service` mark | marker |
| 3 | `handoff_workspace_orphan.go:171` | `HandoffService` clear | marker (clear) |
| 4 | `handoff_cascade.go:904` | `resolveDeleteSet` | **`mode`** |

All four end in the identical `SetTaskMetadataKey(ctx, task.ID, "workspace",
workspace)` over a stale in-memory read. **Site 2 is not dead code, but not for the
reason it looks like.** The MCP `archive_task_kandev` tool does *not* reach it: its
handler prefers `handoffSvc.ArchiveTaskTree` (`mcp/handlers/config_task_handlers.go:397`)
and falls back to the direct `Service.ArchiveTask` route at `:409` only when `handoffSvc`
is nil, which production wiring never is — so an MCP-driven test exercises site **1**.
Site 2's live production callers are the auto-archive scheduler
(`task/service/auto_archive.go:40`) and the workflow-delete cascade
(`service_resources.go:672`). Fixing only site 1 leaves the race live on both.

**A FIFTH writer exists, and is deliberately NOT converted.** The four above are every
caller of the whole-`$.workspace` primitive this design replaces — but they are not every
writer that can *modify* `$.workspace`, and the difference matters enough to state rather
than leave to discovery. The generic task-metadata PATCH surface replaces the **entire**
`metadata` map: `task.Metadata = protectedTaskMetadataUpdate(task.Metadata, req.Metadata)`
(`service_tasks.go:1902`, helper at `service_task_metadata.go:24`) preserves only the
deferred-launch key, then writes the whole row through `UpdateTask` from a read taken at
`:1870`. Its reparent branch also changes `workspace["mode"]`, via
`normalizeWorkspaceModeAfterReparent` (`:2077`, called at `:1922`).

It is an **accepted residual**, on two grounds that are contract rather than convenience.
First, it falls outside AC-005.2 by construction: that AC governs "an archive and an
unarchive" mutating the map concurrently, and this is a user-initiated wholesale metadata
replacement, neither of those. Second, no CAS on `$.workspace` can guard it, because it
does not write `$.workspace` — it writes `metadata`. Narrowing a caller's whole-map PATCH
into a sub-map merge would change that endpoint's contract, which is a separate card.
The consequence, stated plainly: a PATCH racing a mark or a clear can still
last-writer-win over the marker. That window is strictly narrower than the one this
design closes, it is user-initiated rather than automatic, and it is not this card's.

**A SIXTH writer exists, in SQL, and is likewise not converted.** `detachTaskQuery`
(`sqlite/task.go:1497`), reached from `POST /tasks/:id/detach`
(`task/handlers/task_handlers.go:193`), clears `parent_id` and rewrites
`$.workspace.mode` in one statement without reading the map into Go at all: no stale read
to compare, nothing for a CAS to hold. With the PATCH surface's reparent branch, these are
the two paths that can leave the four marker keys standing on a task whose `mode` is no
longer `inherit_parent`.

**AC-001.9 is what keeps that honest, not a guard.** The derived boolean conjoins
`orphaned` with `mode == inherit_parent`, so the moment either writer moves `mode` the
task reports `workspace_orphaned: false` and the residual keys are inert rather than a
second state. That is deliberate: every path that moves `mode` off `inherit_parent` leaves
the task startable, and none is reachable by the repair's clearing pass, whose retraction
needs a *present, unarchived* parent. Site 4 still drops the keys in its own write
(AC-005.6) because it is a converted caller and can do so atomically; for these two that
would be defence in depth over a guarantee AC-001.9 already gives.

**Site 4 differs, and it also clears (AC-005.6).** `resolveDeleteSet` is the
non-cascade parent-delete path: before
reparenting a child it normalizes `workspace["mode"]` from `inherit_parent` to
`shared_group` and writes the whole sub-map back from its own stale read, so it can
resurrect the four orphan keys a concurrent clear just removed — the marker AC-005.2
forbids. Of these four it is the only one that *changes* `mode`; the unconverted PATCH
path above changes it too. Same guarded write, with
`ExpectedMode = inherit_parent` and `ExpectedOrphanedParentID` from the read that
decided to normalize.

**It must also drop the four orphan keys in that same write**, which today it does not:
it sets `mode` and leaves the marker standing. That is not a tidy-up, it closes a
permanent false badge. Walk it: the parent is archived, so the child is marked; the
parent is then non-cascade deleted, so site 4 flips the child to `shared_group` and
`ReparentDirectChildren` moves it to root. The child is now startable — and its marker
survives, naming a row about to disappear. Nothing ever retracts it: the repair's
clearing CTE `JOIN`s on a *present, unarchived* parent, so an absent one never matches
(AC-003.9's explicit design), and the producer's clear only fires on the unarchive of a
parent that no longer exists. The result is a permanent "cannot start" icon and banner
on a task that starts fine — the exact inversion of the invariant REQ-001 to REQ-003
serve, and unreachable by every recovery path this spec defines. Reuse
`clearOrphanedWorkspaceMetadata`, which already reports whether anything changed, so an
unmarked child still takes the plain `mode`-only write. The requirements' `## Out of
scope` bullet on deleted parents covers only tasks never marked; one marked *before* the
delete is AC-005.6's business.

Its lost-guard behavior is **stricter than the marker writers', deliberately**.
AC-005.5 makes a lost marker write a no-op success because a marker is advisory;
site 4's write is a *precondition for a structural change*, and if it does not land
the child is reparented while still `inherit_parent` — the state the write exists to
prevent. A lost guard here returns the existing `normalize workspace mode for child
%s before delete` error and aborts before `ReparentDirectChildren`, exactly as a
failed write does today: the caller retries with a fresh read. Not a retry loop.

One asymmetry to preserve while editing sites 1 and 2: `HandoffService` guards a
type-asserted `envRepo` (`if envRepo != nil` = "mark without the own-environment
check") where `Service` dereferences `s.taskEnvironments` unguarded. That is the
producer's frozen predicate — do not tidy one into the other. It is also precisely why
the two sites take different `RequireNoOwnEnvironment` values in the clause table: the
guard assignment follows this asymmetry rather than erasing it. The clear path exists
only on `HandoffService`.
