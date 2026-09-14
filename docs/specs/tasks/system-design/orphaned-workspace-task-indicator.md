---
status: draft
system: tasks
requirements:
  - REQ-TASKS-ORPHANED-WORKSPACE-001
  - REQ-TASKS-ORPHANED-WORKSPACE-002
---

# Orphaned Workspace Task Indicator System Design

## Purpose and boundaries

The `tasks` system owns the read side of the orphan marker: deriving a task-level
boolean from `tasks.metadata.workspace.orphaned` and delivering it on every task
payload.

**What is frozen, and what is not.** The requirements' `## Out of scope` freezes the
producer's *predicate* and its *call sites*, both shipped in `84c83c3d5` (#3235).
REQ-005 changes only the *metadata-write primitive* those call sites use: narrowing an
unguarded whole-sub-map write to a guarded one changes how a decision is recorded,
never which tasks get marked. No predicate, call site, or key name changes.

**Prior art** (receipts in `## Prior art`). `interrupted-task-indicator.md` is the
direct precedent, and this design reuses its shape: a derived DTO boolean from a
metadata key, no schema migration, no new route or event type, an explicit
icon-precedence rule, and a named recovery owner for every failed write. Two
departures: rendering is **board-only** (matching `auto_start_failed`), and there is a
**historical population to repair** (REQ-003).

Adjacent contracts this design uses but does not own:
`internal/task/service/handoff_workspace_orphan.go` (producer of the four marker keys,
and owner of the clear-on-unarchive path this design constrains for concurrency only),
and the icon precedence chain in `apps/web/lib/ui/state-icons.tsx`, shared with the
`interrupted` and `auto_start_failed` markers. `v1.TaskContext` is untouched: the
detail-panel banner was cut at the round-7 review (see
`## Deferred: explanation surface`).

## Prior art

Split from the requirements, at its byte cap. **Leg 1, our compiled wiki: DID NOT
RUN** — `@henry` resolved the vault from `~/.obsidian-wiki/config.henry` but `ls`
returned `Operation not permitted` (macOS TCC, which also blocks the grep fallback),
and QMD could not substitute (`QMD_TRANSPORT` unset, no `mcp__qmd__*`, no `qmd` on
`PATH`). A skipped step, not an empty one. **Leg 2, `saas-kb`: DID NOT RUN** — tool
absent; intended queries were unstartable-task affordance and blocked-card badge, no
claim made. **Leg 3, this repository: RAN** — searched `docs/specs` and
`docs/decisions` for `auto_start_failed|interrupted_at|orphan`, read producer commit
`84c83c3d5`. [interrupted-task-indicator](../requirements/interrupted-task-indicator.md)
is the direct precedent whose shape this reuses; PR #3235 shipped no spec.

## Requirement mapping

Each `REQ-TASKS-ORPHANED-WORKSPACE-NNN`: **001** → [Data and
contracts](#data-and-contracts) + [Delivery paths](#delivery-paths); **002** →
[Frontend components](#frontend-components). There is no REQ-004: it was cut at the
round-7 spec review and what a follow-up cannot cheaply re-derive is preserved under
`## Deferred: explanation surface`.

**003 and 005 are owned by sibling designs.**
[orphaned-workspace-marker-startup-repair](orphaned-workspace-marker-startup-repair.md)
owns the boot-time stamping and clearing passes (003).
[orphaned-workspace-guarded-metadata-write](orphaned-workspace-guarded-metadata-write.md)
owns `SetTaskWorkspaceMetadataIfUnchanged`, its `OrphanWriteGuard` clause contract, and
the four `$.workspace` call sites that must adopt it (005). The repair is that
primitive's heaviest consumer. Read all three before changing any.

## Measurement receipts

Queries behind the requirements' `## Measured population`. Run 2026-09-05 against
`file:/Users/henry/.kandev/data/kandev.db?mode=ro`, reproduced at spec review. The
table is `task_workspace_groups`, not `workspace_groups`.

```sql
-- (1) live children of an archived parent, by mode. inherit_parent 1, ws 55.
-- Third column checks the own-environment carve-out: every candidate returned 0.
SELECT COALESCE(json_extract(c.metadata,'$.workspace.mode'),'<null>'), COUNT(*),
       (SELECT COUNT(*) FROM task_environments e WHERE e.task_id = c.id)
FROM tasks c JOIN tasks p ON c.parent_id = p.id
WHERE c.archived_at IS NULL AND p.archived_at IS NOT NULL GROUP BY 1;
-- (2) tasks carrying any orphan marker. Returned: 0.
SELECT COUNT(*) FROM tasks
WHERE json_extract(metadata,'$.workspace.orphaned') IS NOT NULL;
```

The one orphaned task (`b7c01447`, `metadata.workspace` exactly
`{"mode":"inherit_parent"}`) is unmarked because it was born stranded, created after
its parent's archive. The creation-time guard that would now reject it post-dates the
row, so the population is historical, not growing.

## Components and responsibilities

### Backend

| Component | Responsibility |
| --- | --- |
| `internal/task/models/models.go` `ToAPI` | Derive `WorkspaceOrphaned` for the `v1` DTO |
| `internal/task/dto/dto.go` `FromTaskWithSessionInfo` | Derive it for `TaskDTO` |
| `pkg/api/v1/task.go` | Carry `workspace_orphaned` on `v1.Task` |
| `internal/backendapp/boot_state_routes.go` `mapKanbanTaskState` | Add `workspaceOrphaned` to the camelCase whitelist |
| `internal/task/service/service_events.go` `publishTaskEventNow` | Emit explicit `true`/`false` |

Six more backend files change under the sibling designs and are not restated here.
`handoff_workspace_orphan.go`, `service_tasks.go` and `handoff_cascade.go` change only
to adopt the guarded write (REQ-005). `handoff_workspace_orphan_repair.go` (new),
`repository/sqlite/task.go` (the two selection queries) and `backendapp/helpers.go` (the
invocation) are REQ-003, owned by
[orphaned-workspace-marker-startup-repair](orphaned-workspace-marker-startup-repair.md).

### Frontend

| Component | Responsibility |
| --- | --- |
| `lib/types/http.ts` | `workspace_orphaned?: boolean` on the Task type |
| `lib/kanban/map-task.ts` | `workspaceOrphaned: source.workspace_orphaned` — a **bare pass-through**, see below |
| `lib/ssr/mapper.ts` | Boot-payload resolution, mirroring `resolveAutoStartFailed` |
| `lib/state/slices/kanban/types.ts` | `workspaceOrphaned?: boolean` on the store task |
| `lib/ws/handlers/tasks.ts` | `preserveOmittedField` entry |
| `lib/ws/handlers/kanban.ts` | Carry-forward on the `kanban.update` merge |
| `lib/ui/state-icons.tsx` | Third branch in `getMarkerIconOverride`, plus the icon component |
| `components/kanban-card-content.tsx` | Board render site **and an edit**: `renderTaskStatusIcon`'s early-return gate must name the new marker, or the card stays blank |
| `components/kanban/graph2-step-node.tsx` | Board render site, no edit: it calls `getTaskStateIcon` ungated |

### Frontend components

`getMarkerIconOverride` (`state-icons.tsx`) gains a third branch after
`autoStartFailed`:

```ts
if (TERMINAL_TASK_STATES.has(state)) return null;
if (interrupted) return TASK_INTERRUPTED_ICON;
if (autoStartFailed) return TASK_AUTO_START_FAILED_ICON;
if (workspaceOrphaned) return TASK_WORKSPACE_ORPHANED_ICON;
return null;
```

**Do not reorder the enclosing chain.** `getTaskStateIconConfig` calls
`getMarkerIconOverride` *last*, after pending permission, pending clarification,
`generating`/`background` activity and `isWaitingForInputState`. Terminal state is
checked **inside** `getMarkerIconOverride`, so it gates the three markers but does
**not** outrank those four earlier affordances, which AC-002.3 preserves. The
requirements' precedence list describes the resulting order; it is not an order to
re-sort.

**That branch alone renders NOTHING on the kanban card — the edit most likely to be
missed.** The two board surfaces reach `getTaskStateIcon` differently.
`graph2-step-node.tsx:181` calls it unconditionally, so the new branch suffices there.
`kanban-card-content.tsx` does not: `renderTaskStatusIcon` opens with

```ts
if (!showRunningSpinner && !needsMe && !hasActivity && !showInterrupted && !showAutoStartFailed) {
  return null;
}
```

returning **before `getTaskStateIcon` is called**. For the population REQ-002 exists to
surface — a live `CREATED`/`TODO` subtask, no session, no activity, no pending prompt —
all five are false: `shouldShowTaskRunningSpinner` (`state-icons.tsx:268-279`) is false
for `TODO`, and with a null `primarySessionState` true only for
`IN_PROGRESS`/`SCHEDULING`. So add `showWorkspaceOrphaned` to that gate and thread the
value into the options beside `autoStartFailed` (`:339-345`). The gate already naming
`showInterrupted` and `showAutoStartFailed` individually is the proof each marker must
be added here too. AC-002.1 requires both surfaces so a test on one cannot stand in for
the other.

`getTaskStateIcon` dispatches marker configs by identity comparison to a wrapper
component owning the tooltip and `aria-label`, so the marker needs a
`WorkspaceOrphanedTaskIcon` beside `InterruptedTaskIcon` and
`AutoStartFailedTaskIcon`, plus a matching identity branch. Icon identity is what
AC-002.1 is asserted against, mirroring `kanban-card-status-icon.test.tsx`. Both
existing markers are `STYLE_ERROR` red and differ only by shape, so prefer a distinct
hue *and* shape: a muted or slashed glyph reads as "cannot run", not "run failed".

Copy lives in `common.json` beside `interruptedByRestart` and `autoStartFailed`, in
`en`, `pt-pt`, `zh-cn`, `zh-hk`, `zh-tw`, plus `pseudo` (`pnpm run i18n:pseudo`;
`pnpm run i18n:zh-hant` for the Traditional pair). No U+2014 (AC-002.6).

## Data and contracts

The marker is unchanged, and REQ-001 reads it, never writes it:
`orphaned` (bool `true`), `orphaned_reason` (string `"parent_archived"`),
`orphaned_parent_id` (string), `orphaned_at` (string, RFC 3339 UTC), all under
`tasks.metadata.workspace`.

Derivation, per AC-001.2, is a strict boolean-`true` test, **not** the `!= nil`
presence test `interrupted` and `auto_start_failed` use:

```go
func workspaceOrphaned(meta map[string]interface{}) bool {
	ws, ok := meta["workspace"].(map[string]interface{})
	if !ok {
		return false
	}
	v, _ := ws[orphanedWorkspaceKey].(bool)
	mode, _ := ws["mode"].(string)
	return v && mode == workspaceModeInheritParent
}
```

**The `mode` conjunct is AC-001.9**, and it is what keeps the marker honest against
the writers this spec does not guard. Three paths change `mode` off `inherit_parent`
and leave the four keys standing: the delete path (`handoff_cascade.go:903`),
`normalizeWorkspaceModeAfterReparent` (`service_tasks.go:2077`, called `:1922` on any
`parent_id` change, reachable from `task_http_handlers.go:958` and
`task_ws_handlers.go:194`), and `detachTaskQuery` (`sqlite/task.go:1497`, raw SQL
behind `POST /tasks/:id/detach`). All three leave the task startable, and without the
conjunct each strands a badge AC-003.9 cannot retract — its clearing CTE needs a
present, unarchived parent, so a marker naming a still-archived or deleted one never
matches — while AC-003.4 forbids clearing on predicate failure. Re-parenting is the
operator's obvious response to the badge, so the marker would punish the action it
prompts. The conjunct closes all three, and any future one, in the single place the
value is derived; AC-005.6 still clears the keys on the delete path as row hygiene
rather than as the only guard.

The two existing markers can use a presence test because their clear path removes the
key. This one clears the same way, but the key is a `bool` whose false value is
meaningful, so a presence test would report `true` for `"orphaned": false`. The type
assertion also satisfies AC-001.3: a non-object `workspace` yields `false`, not a
panic.

**This helper is the single derivation site.** `ToAPI`, `FromTaskWithSessionInfo`,
`publishTaskEventNow` and the repair all call it rather than re-testing the key,
making AC-001.2 and AC-001.9 structural. The repair's SQL predicate mirrors it
(see the startup-repair design) — the one place the rule is expressed twice,
and the two must change together. Both CAS clauses of the guarded write mirror the
comma-ok *string* assertions the same way.

No schema migration, column, route, WS action, or event type (AC-001.7).

### Delivery paths

Three independently hand-built maps carry task fields to the client. All three are
required; each has been the site of a real omission bug.

1. **HTTP DTO** — `internal/task/dto/dto.go` (`TaskDTO`) and
   `internal/task/models/models.go` (`ToAPI` for `v1.Task`, beside the
   `AutoStartFailed` assignment), which already compute both existing markers.
2. **Boot payload** — `mapKanbanTaskState` is an explicit camelCase whitelist writing
   straight into the frontend store shape, so a new DTO field is invisible to first
   paint until listed there. It carries **no `metadata` key at all**, which is why
   REQ-001 specifies a derived boolean rather than letting the client read
   `metadata.workspace.orphaned`.
3. **WS `task.updated`** — `publishTaskEventNow`, as an explicit boolean, per the
   `auto_start_failed` precedent: `preserveOmittedField` pins the previous value when
   a key is absent, so an omitted key would make a clear as invisible as the set it
   undoes. (`interrupted` is deliberately *not* on this path; do not copy that.) It is
   shared by `task.created`, `task.updated` and `task.state_changed`, so one edit
   meets AC-001.5.

`models.PublicTaskMetadata` clones metadata and redacts only deferred-launch
attribution, so `workspace` already reaches the HTTP and WS payloads.

**The two frontend mappers take OPPOSITE null conventions, and the difference is
load-bearing.** `lib/ssr/mapper.ts` normalizes — `resolveAutoStartFailed` is
`task.auto_start_failed ?? false` (`:15`) — because it feeds a FULL boot snapshot.
`lib/kanban/map-task.ts` must NOT: it passes `interrupted` and `autoStartFailed`
through with no `?? false` (`:245-246`), unlike its `?? false` / `?? undefined`
neighbours, because `preserveOmittedField` (`ws/handlers/tasks.ts:82`) detects an
omitted wire key by testing `nextTask[field] === undefined`. Normalize there and that
test never fires, so AC-001.6's preserve-on-omission is silently gone on the WS delta
path — a false clear on exactly the partial delta it exists to cover, with nothing
failing fast.

## Deferred: explanation surface

REQ-004 (a banner on the task detail context panel naming the archived parent) was
**cut at the round-7 spec review**; deferred, no follow-up filed. The requirements'
`## Out of scope` holds the decision and why. This section holds only what a follow-up
cannot cheaply re-derive.

- **The panel is not on the kanban route.** `TaskDetailContextPanel` is mounted in
  exactly one place, `TaskContextSection` (`OfficeSimplePane.tsx:526`), rendered only by
  `/office/tasks/[id]`; `/tasks/[id]` (`kanban-task-shell.tsx:66-98`) never mounts it.
  So either mount it there too, or explain inside the marker's own tooltip, which is
  already localized and already on both board surfaces. The second is cheaper.
- **A parent title needs a same-workspace gate.** `orphaned_parent_id` is user-writable
  and `Repository.GetTask` (`task.go:509-517`) filters on `t.id` alone, so render a
  title only when the resolved parent's `workspace_id` matches the subject's; otherwise
  treat it as a miss, as also when the id is absent, empty, non-string or unresolvable.
  Extend `resolveWorkspaceFields` (`handoff_context.go:106-118`) rather than adding a
  second `GetTask`, and do not reuse `workspace_status` or `blocked_reason`.
- **The panel will not converge on a producer-driven mark.** Neither marker wrapper
  refreshes `task.UpdatedAt` while `SetTaskMetadataKey` (`sqlite/task.go:1110`) bumps
  `updated_at` in the DB, and `useTaskContext` (`use-task-context.ts:44`) refetches only
  when `task.updatedAt` changes. The repair's re-read (AC-003.5) is unaffected.
- Its early return (`if (!showRelations && !showDocs && !showWorkspace) return null`)
  sits above `WorkspaceStatusBanner`, so a `requires_configuration` status alone renders
  nothing today.

## Failure and recovery

- A failed repair write logs a Warn naming the task and continues (AC-003.6); the next
  startup retries it, since the pass is idempotent and re-derives its work list. A
  *lost guard* is not a failed write: it is a silent skip, publishing nothing
  (AC-005.5), logged at Debug.
- A failed repair *selection query* logs a Warn and skips that pass for the boot
  without failing startup (AC-003.11). The repair enhances an already working board;
  it must never stop the process coming up.
- A failed producer *mark* already logs and continues upstream; unchanged here. The
  result is a task with no marker until the next repair pass.
- A failed or crashed producer *clear* leaves a marker naming a parent that is no
  longer archived, and nothing in the producer revisits it: the clear runs once, on
  unarchive, and that unarchive has already happened. **The repair is the recovery
  owner** (AC-003.9), the one case in which it may clear — and it must reach archived
  children to be that owner, since their markers have no other revisit path.
- An unresolvable, absent, empty, or non-string `orphaned_parent_id` never affects the
  derived boolean, which reads `orphaned` and `mode` only. It narrows what AC-003.9 can
  retract, nothing else; the marker is still correct, since the task cannot start.
- A task whose `mode` is changed off `inherit_parent` stops reporting orphaned at once
  (AC-001.9), even though its four keys persist. That is the recovery owner for the
  reparent, detach and delete paths, none of which AC-003.9 can reach.
- A `task.updated` that omits the key must not clear a set marker client-side
  (AC-001.6) — `preserveOmittedField`, which the WS-side explicit boolean makes
  unnecessary in practice but which guards the `kanban.update` merge path.

## Persistence

No schema change: the four marker keys are already durable in `tasks.metadata`, and the
read side adds no table, column, index, or retention rule. The historical population that
no archive event will reach is stamped by a boot-time pass owned by a sibling design,
[orphaned-workspace-marker-startup-repair](orphaned-workspace-marker-startup-repair.md),
which also owns REQ-003's clearing pass, its dialect branches and its logging contract.

## Security

None beyond existing task-visibility rules. The marker is display metadata on a task
the viewer can already read; it exposes no filesystem path, branch name or credential,
`orphaned_parent_id` names a task in the same workspace, and the context fields ride
the already-authorized `GET /api/v1/tasks/:id/context`. Task metadata is user-writable
through the generic metadata update surface (`protectedTaskMetadataUpdate` preserves
only the deferred-launch key), so every value this capability reads is
attacker-chosen in the ordinary sense. Setting `orphaned` on your own task only badges
a card you own, matching the interrupted marker's accepted posture. Nothing here
projects `orphaned_parent_id` or anything derived from it: with REQ-004 cut, the id is
read only by the repair's clearing JOIN, which compares it to a row id and returns no
attacker-visible value. A surface that ever renders the parent's title must add a
same-workspace gate first, because `Repository.GetTask` applies no workspace predicate
— see `## Deferred: explanation surface`. The same user-writability is why two rules
are load-bearing: AC-001.2's strict value test (a hand-written `"orphaned": false` is
reachable) and the CAS's `json_type` gate (so is a non-string `orphaned_parent_id`).

## Observability

- The producer already logs `marked inherit_parent child orphaned by parent archive`
  and `cleared inherit_parent child orphan marker after parent unarchive` at Info with
  `task_id` and `parent_task_id`; reuse it.
- A lost guard is Debug, not Warn: a concurrent writer won, which is correct behavior
  rather than a fault.
- No new metric: the population is bounded, and the logs answer "did the pass run and
  what did it touch". The repair pass's own logging contract (AC-003.12) is in its
  sibling design.

## Related decisions

No ADR. This adds no architectural boundary: it extends the derived-marker pattern
in [interrupted-task-indicator](../requirements/interrupted-task-indicator.md) and
the guarded-metadata-write pattern already present as
`SetTaskMetadataKeyIfNotArchived` and `SetTaskMetadataKeyIfNoActiveSession`.
