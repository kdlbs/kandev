---
status: current
system: tasks
requirements:
  - REQ-TASKS-TASK-CREATE-RELATIONSHIPS-001
  - REQ-TASKS-TASK-CREATE-RELATIONSHIPS-002
  - REQ-TASKS-TASK-CREATE-RELATIONSHIPS-003
  - REQ-TASKS-TASK-CREATE-RELATIONSHIPS-004
  - REQ-TASKS-TASK-CREATE-RELATIONSHIPS-005
created: 2026-11-05
owners:
  - kandev
---

# Task-create relationship establishment System Design

## Purpose and boundaries

This design adds a new non-blocking relationship kind ("related to") and
exposes it, plus the existing parent and depends-on relationships, through
every task-creation path: the main New Task dialog, the New Subtask dialog,
and `create_task_kandev`. It does not change `task_blockers` semantics,
cycle checks, auto-start, or the launch gate defined in
[`task-dependencies.md`](task-dependencies.md). It does not add any editing
surface for an established relationship; the task graph stays read-only per
[`task-relationship-graph.md`](task-relationship-graph.md), whose
REQ-TASKS-TASK-RELATIONSHIP-GRAPH-011 renders the new edge kind this design
introduces.

The task system owns this design because it owns task creation, the
`task_blockers` table, and the `parent_id` column that the new capability
extends.

A new ADR ([ADR-2026-11-05 task related-to storage](../../../decisions/2026-11-05-task-related-to-storage.md))
records the choice of a separate `task_relations` table over a flag on
`task_blockers`.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-TASK-CREATE-RELATIONSHIPS-001` | [Parent selector](#parent-selector) |
| `REQ-TASKS-TASK-CREATE-RELATIONSHIPS-002` | [Related-to storage](#related-to-storage), [Related-to selector](#related-to-selector) |
| `REQ-TASKS-TASK-CREATE-RELATIONSHIPS-003` | [Candidate ranking](#candidate-ranking) |
| `REQ-TASKS-TASK-CREATE-RELATIONSHIPS-004` | [MCP tool](#mcp-tool) |
| `REQ-TASKS-TASK-CREATE-RELATIONSHIPS-005` | [Dependency chip third section](#dependency-chip-third-section) |

## Related-to storage

A new `task_relations` table stores the symmetric link, bootstrapped in
`internal/office/repository/sqlite/base.go` next to the `task_blockers`
`CREATE TABLE IF NOT EXISTS` statement, the same way `task_blockers` itself
needs no numbered migration:

```sql
CREATE TABLE IF NOT EXISTS task_relations (
  task_id_low  TEXT NOT NULL,
  task_id_high TEXT NOT NULL,
  created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (task_id_low, task_id_high),
  CHECK (task_id_low < task_id_high)
);
```

Storing the pair with the lexicographically lower ID first makes the
relationship inherently symmetric and unique: inserting `(a, b)` and `(b, a)`
normalize to the same row, so a duplicate insert is a no-op rather than a
second row. This mirrors the `CHECK(task_id != blocker_task_id)` guard on
`task_blockers` (`internal/office/repository/sqlite/base.go:766-771`) but adds
the ordering check instead of a directed self-reference guard, since
related-to has no direction to preserve.

A `RelationModel{TaskIDLow, TaskIDHigh, CreatedAt}` type lives beside
`TaskBlocker` in `internal/office/models/models.go`. A `RelationRepository`
interface (mirroring `BlockerRepository`,
`internal/task/service/service_office.go:15-25`) declares:

```go
type RelationRepository interface {
    AddRelation(ctx context.Context, taskA, taskB string) error
    ListRelationsForTask(ctx context.Context, taskID string) ([]string, error)
    ListRelationsForTasks(ctx context.Context, taskIDs []string) (map[string][]string, error)
    RemoveRelationsForTask(ctx context.Context, taskID string) error
}
```

`internal/office/repository/sqlite/relations.go` implements it, normalizing
`(taskA, taskB)` into `(task_id_low, task_id_high)` before every query, the
same file layout as `internal/office/repository/sqlite/blockers.go`.
`RemoveRelationsForTask` is used only by task deletion's existing cleanup
pass (which already removes the task's `task_blockers` rows), not by any new
editing surface.

## Task creation integration

`Service.CreateTask` (`internal/task/service/service_tasks.go:251`) gains a
`RelatedTaskIDs []string` field on its create-arguments struct, alongside the
existing `ParentID` and `BlockedBy` fields. `create_task_references.go`'s
`validateBlockerReferences` (line 58) is extended with a parallel
`validateRelationReferences` check: each related ID must resolve to a
non-archived task in the same workspace, using the same lookup the blocker
validator already performs, and returns the same validation-error shape named
in AC-TASKS-TASK-CREATE-RELATIONSHIPS-004.3 so `blocked_by` and the new
related-task field fail the same way for an unresolvable ID.

`finalizeCreatedTask` (`service_tasks.go:482`) extends its existing
one-edge-at-a-time loop (`:487-490`, `AddBlocker` per blocker) with a second
loop calling `RelationRepository.AddRelation` per related ID, after the
blocker loop. Task creation remains non-transactional, as it is today;
`rollbackPartialTask` (`create_task_references.go:113`) is extended to also
call `RemoveRelationsForTask` for the newly created task ID during
compensation, matching its existing blocker-edge rollback.

Parent assignment for a parent selected in the main create dialog uses the
same `ParentID` field the New Subtask dialog already populates
(`apps/web/components/task-create-dialog-types.ts:579,94` →
`task-create-dialog-submit.tsx:730` → `helpers.ts:270,258`), so no backend
change is needed for REQ-TASKS-TASK-CREATE-RELATIONSHIPS-001 beyond making
that existing field settable from the main dialog's new Parent selector.

## Task payload exposure

The task read/projection model gains a `related: string[]` field, populated
the same way `depends_on`/`blocks` are populated today: `BuildDependencyViews`
(`internal/task/service/service_dependencies.go:77-84`) is extended to also
accept the relation map from `ListRelationsForTasks` and attach
`DependencyView.Related` so one batched read continues to produce the
board/chip projection, the task-detail payload, and (via the graph service's
reuse of this projection) the `related` IDs the task-graph live-refresh
signature now tracks.

## MCP tool

`create_task_kandev`'s schema in `internal/mcp/server/server.go` gains
`related_task_ids` (array of strings, optional), declared next to the
existing `parent_id` (line 1609) and `blocked_by` (lines 1625-1628)
parameters. The handler passes it through to `Service.CreateTask`'s new
`RelatedTaskIDs` field unchanged. An unresolvable ID in `related_task_ids`
fails task creation the same way an unresolvable `blocked_by` ID does today,
satisfying AC-TASKS-TASK-CREATE-RELATIONSHIPS-004.3.

## Candidate ranking

A shared ranking function, `rankCreateDialogCandidates`, replaces the direct
use of `compareDependencyCandidates`
(`apps/web/components/task-create-dialog-dependency-search.ts:24-39`) as the
sort passed to `.sort()` in `useDependencyPickerData`
(`task-create-dialog-dependencies.tsx:292-364`, applied at lines 355-359).
It wraps the existing comparator as its final tiebreaker and adds, in order:

1. **Same swimlane/column** — a candidate whose board column or swimlane ID
   matches the creation context (passed down the same way the create dialog
   already receives its originating board context) sorts first.
2. **Recently viewed/created this session** — a candidate present in the
   existing client-side recent-tasks list (already tracked for navigation
   history) sorts next.
3. **Title search match** — once the user has typed search text, for the
   Depends-on and Related-to selectors only, a title-text match sorts above
   an id-only or change-request-number-only match. The Parent selector keeps
   its existing search-then-filter behavior unchanged, since a parent choice
   is less frequently decided by free-text search.
4. **Existing fallback** — `compareDependencyCandidates`'s current
   most-recently-updated → most-recently-created → title order, unchanged,
   covers every remaining case.

This single function is shared by the Depends-on selector (replacing its
current direct comparator), the new Parent selector, and the new Related-to
selector, so all three present consistent suggestions
(AC-TASKS-TASK-CREATE-RELATIONSHIPS-003.5). It lives in
`task-create-dialog-dependency-search.ts` beside the comparator it wraps.

## Parent selector

A new `TaskCreateParentSelector` component, modeled on
`TaskCreateDependencies` (`task-create-dialog-dependencies.tsx:366`), is added
to `TaskCreateAdvancedSettingsContent`'s option grid
(`task-create-dialog-advanced-settings.tsx:258`), following the same
conditional-row pattern as the existing workflow-agent-overrides row
(`:303-315`): it renders only when the dialog is in create mode and no
`parentTaskId` was supplied externally by the caller
(AC-TASKS-TASK-CREATE-RELATIONSHIPS-001.4). It is a single-select searchable
picker (no "No dependency"-style multi-select; a task has at most one
parent), using `rankCreateDialogCandidates` and excluding archived tasks and
tasks that would create a cycle in the parent hierarchy (a task cannot become
its own descendant's parent — checked client-side against the already-loaded
board tree, with the existing server-side parent validation as the
authoritative guard). Selecting a candidate sets the existing `parentTaskId`
form field; the trigger shows the selected task's title or a localized
"No parent" default, matching the Depends-on trigger's label pattern.

The New Subtask dialog does not gain this selector: its parent is always
externally supplied (`subtask-dialog-parent-defaults.md`), unchanged by this
design.

## Related-to selector

A new `TaskCreateRelatedSelector` component is added to the same option grid,
always rendered in create mode (main dialog) and in the New Subtask dialog's
equivalent advanced area. It is a multi-select searchable picker, structurally
identical to `TaskCreateDependencies` but backed by the new `relatedTaskIds`
form field, and excludes from its candidate list: the task's own prospective
identity, every task currently selected in the Depends-on selector, and the
task currently selected as parent (AC-TASKS-TASK-CREATE-RELATIONSHIPS-002.5).
Its trigger shows "No related tasks" by default or a localized count,
matching the Depends-on trigger's multi-select label pattern. Submission adds
`relatedTaskIds` to the create payload as `related_task_ids`
(`helpers.ts`, alongside the existing `blocked_by` wiring at line 290).

## Dependency chip third section

`DependencyChipData` (`hooks/domains/task/use-task-dependencies.ts:6-11`)
gains a `related: TaskSummary[]` field, populated from the task's new
`related` projection field the same way `dependsOn`/`blocks` are populated
today (lines 57-58, 104-105). `DependencyLists`
(`components/task/task-dependency-chip.tsx:107`) renders a third "Related"
section (alongside the existing "Waiting on" at line 121 and "Blocks" at line
137) when `related` is non-empty, with no blocked/resolved iconography since
a related-to link carries no status. `ChipTriggerContent` (line 163) extends
its summary to mention a related-to count when depends-on/blocks are both
empty but related-to is not, so the chip itself becomes visible per
AC-TASKS-TASK-CREATE-RELATIONSHIPS-005.2.

## Testing

- Go: `RelationRepository` CRUD and symmetry (insert either order produces one
  row); `validateRelationReferences` rejecting an archived or unknown ID;
  `finalizeCreatedTask`/`rollbackPartialTask` relation-edge coverage;
  `BuildDependencyViews` `Related` population; `create_task_kandev` handler
  test for `related_task_ids` validation and pass-through.
- Vitest: `rankCreateDialogCandidates` ordering for each signal and its
  fallback; `TaskCreateParentSelector` and `TaskCreateRelatedSelector`
  rendering, exclusion rules, and payload wiring; `DependencyChipData`/
  `DependencyLists`/`ChipTriggerContent` third-section coverage.
- Playwright: extend the existing `task-dependencies.spec.ts` flow to select a
  parent and a related task from the main create dialog and assert the
  resulting chip and graph show the new relationships.

## Out of scope

Everything listed under [`task-create-relationships.md`](task-create-relationships.md#out-of-scope)
applies to this design unchanged.
