---
id: coordinator-watch-projects-design
title: Watches by project design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-30
last_updated: 2026-09-30
requirements:
  - REQ-COORDINATOR-PERMISSIONS-005
---

# Watches by project System Design

## Purpose and boundaries

Phase 2 narrows a coordinator to some workflows. This design adds a second
narrowing by repository, the workspace's project (a
`RepositorySet` or a single repository, not an Office project), enforced wherever the workflow watch
is enforced. It is one filter module on the server and one on the client, and
each existing path calls the module instead of comparing repositories itself.
The full-size permissions design is not edited; it keeps the workflow watch and
this design adds the second predicate.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-PERMISSIONS-005` | All sections |

## Storage

`coordinators` gains `project_scope` (`all` default, or `selected`) and
`include_no_repository` (bool, default false: a coordinator that starts selecting projects does not watch repository-less tasks until the manager turns the toggle on). New table
`coordinator_watch_projects(coordinator_id, entry_kind, entry_id)` with
`entry_kind` in `repository_set` or `repository`, primary key on all three,
stored as a list of filters so a later group filter is one more `entry_kind`
(`005.1`). Repository deletion is soft and `DeleteRepository` and `DeleteRepositorySet`
only publish events, so the coordinator service subscribes to those events and
removes the entries of every coordinator that lists the deleted set or
repository; deleting the coordinator removes its rows (`005.6`). An entry that
outlives its target (a missed event or a race) matches no task: a `repository_set` entry
because membership is resolved live from the set service, and a `repository` entry
because the resolver intersects the listed repository ids with the repositories of the
workspace it reads in the same request or batch (soft-deleted and out-of-workspace repositories are
absent from that read, and task repository rows are not removed when a repository is
deleted, so without the intersection a deleted repository would keep matching). The server-built
`repository_ids` of the read shape is computed with the same intersection. Such an entry is
dropped by the next Projects save without failing it. A
`selected` list that becomes empty with the toggle off means the coordinator
watches no task; it is never switched to `all`. A set that loses its last
repository stays listed and matches nothing.

## The filter

`watch.Predicate` extends the effective watch set with `{ScopeAll bool,
Sets set, RepoIDs set, IncludeNoRepo bool}` and one method,
`InProjects(taskRepoIDs []string) bool`:

- scope `all`: true;
- scope `selected`: true when any repository of the task is in `RepoIDs`, or is
  a member of any set in `Sets` at that moment, or the task has no repository
  and `IncludeNoRepo` is on.

Set membership comes from `ListRepositorySets` (the repository set service,
which already excludes soft-deleted and out-of-workspace repositories) and is
resolved by `watch.Resolver`, which reads the sets of the workspace once per
request or per batch and never stores the result on the coordinator. The
workspace's live repositories, which the `repository` entries are intersected
with, are read in the same request or batch through the task service's
`ListRepositories(ctx, workspaceID)`, the call `list_repositories_kandev`
already makes (`mcp/handlers/handlers.go`, `handleListRepositories`) and the
settings page already uses, so the resolver adds no new repository read path;
that call may prune and publish `repository.deleted` for a soft-deleted row, as it
does for every existing caller, and the cascade it triggers is idempotent and
works on stored rows only, so it never re-enters the resolver. A failure of
either read (the set listing or the repository listing) is one failure: the
resolver returns an error and the caller fails closed as the Error handling table
says. Because nothing is stored, adding
a repository to a set widens every coordinator that lists it on the next read
(`005.3`). A task in several repositories is in scope when any one matches.
`watch.Task(task)` is the single function that combines the workflow test and
`InProjects`; every path below calls it and none compares repositories
directly (`005.4`).

The predicate is compiled in and reads the stored fields whatever the phase 3.1
flag, so a stored `selected` scope is never silently widened by turning the flag
off; a coordinator that never had a scope is `all` and behaves as phase 3
(`005.9`). Only the settings member, the lists and the controls are flag-gated.

## Enforcement paths

Each path already calls the phase 2 workflow filter; the change is that the call
site uses `watch.Task`. Where a path only holds a workflow id (list workflows,
list steps), it is unchanged, since a workflow is not project-scoped.

| Path | Effect |
| --- | --- |
| `list_tasks_kandev` | tasks outside the projects are omitted from the result and `Total` is the number returned, as `list_workflows_kandev` omits unwatched workflows with `Total` the filtered count (`mcp/handlers/handlers.go`, `handleListWorkflows`, `coordinatorWatchFilter`); the call is not refused for containing out-of-scope tasks, and a resolver failure is an error result naming `projects` (Error handling) |
| `get_task_conversation_kandev`, `get_coordinator_item_kandev` | outside the projects: phase-1 not-found error |
| `list_repositories_kandev`, `list_workflows_kandev`, `list_workflow_steps_kandev` | unchanged: a repository or a workflow is not itself project-scoped, so a coordinator can still see which repositories exist |
| Delivery recheck (`delivery_recheck.go`, `holdingWakes` and `wakeHolds`) | a pending wake whose task is outside the projects is superseded like one outside the workflows, by the same `watch.Task` call (`wakeHolds` is where the workflow test sits today). The resolver is read once per delivery, next to the existing `EffectiveWatchSet` read and before any supersede: if that read fails the whole step returns the error and nothing is superseded, exactly as a failed watch-set read does today, so every pending wake stays pending for the next backstop pass; the abort increments `coordinator_delivery_aborted_total` with the closed reason `projects_read_failed` (an expvar map, like `coordinator_activity_rows_total`) and logs a WARN naming the coordinator. A failed read of one task's repositories leaves that wake pending and excluded, the existing rule for a per-wake read error (`episode read failed`) |
| Open watched count (`CountOpenWatchedTasks`, `measures.go`) | under scope `all` it stays the SQL `COUNT` it is today. Under `selected` it keeps the SQL workflow filter and selects the ids of the matching tasks, reads their repository ids in one batched read, and counts those for which `watch.Task` holds, so the repository comparison is still made by the one function and never in SQL. A failed resolver or batch read returns the error to the caller, as a failed SQL read does today |
| Propose tools | target outside the projects: refusal naming the field; `create_task`: a `repository_id` that is neither listed nor in a listed set, or empty with the toggle off, refused naming `repository_id` (`005.5`) |
| Wake recorder and backstop | a task outside the projects raises no wake; a resolver failure raises nothing for that coordinator in that pass (the recorder drops the wake, the backstop skips the coordinator) with a WARN, and the next wake or pass re-evaluates |
| Needs you, Queue, counts, stalls | filtered on the client by the same rule (below) |
| Automatic class counts | the 10-per-24h cap of `AC-COORDINATOR-AUTOMATIC-003.2` is independent of Projects: it counts every automatic approval of the coordinator in the window whatever the scope is now, so narrowing the scope or a set losing a repository never lowers the count. The projects filter applies only to counts that are of watched tasks (open watched tasks, class eligibility reads of a target task); a `create_task` proposal has no target task and is never filtered by its target, only refused at proposal time by `repository_id` (`005.5`) |
| Turn ledger snapshot and digest | snapshot and digest use `watch.Task`; the row stores `project_scope` as it was |
| Dream window and evidence | proposals of tasks outside the projects are dropped from evidence |

A `create_task` proposal names one optional `repository_id`; under `selected`
the refusal rule is the whole check, since a proposal has a single repository,
and under `all` nothing is refused for projects (`005.5`). Approve does not
re-check projects: a proposal made in scope stays approvable, as for Watches
(`005.5`).

## Client filter

The pure module `apps/web/lib/coordinator/watch-filter.ts` gains `isTaskWatched`
input: the effective watch set from `GET .../settings` now carries `projects:
{scope, repository_ids, include_no_repository}` on the wire (snake_case, like
`watches.workflow_ids` in `CoordinatorWatchDTO`, `dto.go`), where `repository_ids` is the
server-resolved union of the listed repositories and the current members of the
listed sets, so the client never resolves sets itself. The hook maps the wire member to
`projects: {scope, repositoryIds, includeNoRepository}`, as it maps `workflow_ids` to
`workflowIds` in `WatchSet` (`watch-filter.ts`), and the module's `WatchSet` type gains
`projects?: {scope: "selected"; repositoryIds: readonly string[] | null; includeNoRepository: boolean}`.
A set membership change publishes no
`coordinator.updated`, so the watch-set hook of the
[client watch filter](permissions-ui.md#client-watch-filter) also re-reads on the workspace's
`repository_set.created`, `repository_set.updated`, `repository_set.deleted` and
`repository.deleted` events (in addition to mount, **Try again** and `coordinator.updated`),
whenever it is active at all (the phase-2 flag on), whatever the phase 3.1 flag or the stored scope, so there is
one rule and no flag state to keep in step; the extra read is one small GET on a rare event.
A task's own repositories arrive with its snapshot. `AttentionTask` gains `repositoryIds: string[] | null`, set from
the snapshot. The rule is the server's: workflow watched and `InProjects`. An
unknown repository list is treated as not watched, the same fail-closed rule as a
missing workflow id. Unknown and empty are different shapes: `repositoryIds`
`null` (in `AttentionTask`, or `repository_ids: null` in the settings `projects` member when the server
could not resolve the sets; the type admits it) means unknown, and `[]` means known and empty, which
matches only when `includeNoRepository` is on. A `projects` member with `repositoryIds` `null` is
treated as not watching any task (fail closed) and shows the phase-2 "Could not load which boards
this coordinator watches." banner line pattern's sibling line "Could not load which projects this
coordinator watches." with **Try again**, so an unresolved set is never shown as an empty queue. The task payload omits an empty
`repositories` list on the wire (`omitempty` on the task DTO), so the one
function that builds an `AttentionTask` from a loaded task payload maps
a present non-empty `repositories` to its repository ids; when `repositories` is absent or
empty it uses `[repositoryId]` if the payload or store task carries a main `repositoryId`
(the store can hold `repositoryId` with `repositories` undefined after the main repository
changes, `lib/ws/handlers/task-repositories.ts`), and `[]` only when it carries neither; `null`
is reserved for an `AttentionTask` built without a task payload (a stall row or
a snapshot whose task failed to load). Tests: a loaded task with neither `repositories` nor `repositoryId` is watched under `selected`
with the toggle on and not watched with it off; a store task with `repositoryId` set and
`repositories` undefined is matched by that repository.

**With the flag off.** The stored scope is still enforced on the client. The
effective watch set in `GET .../settings` carries the `projects` member whenever
the stored scope is `selected`, whatever the flag, because it is enforcement
state read by the Needs you, Queue, counts and stalls paths, not a field of the
Projects feature; when the scope is `all` (always the case for a coordinator that
never had a scope) the member is absent and the payload is byte-identical to
phase 2's. `005.9`'s "no Projects field, route or control" therefore refers to
the settings write member, the lists, the routes and the controls, which stay
flag-gated, and a test asserts both the flag-off `selected` payload and the
flag-off `all` payload. The "not available" and "watches
nothing" states of the phase 2 screens apply unchanged; the "watches nothing"
notice also covers the empty project selection.

## Save

The settings request that carries May do and Watches also carries `projects`
(`{scope, entries: [{kind, id}], include_no_repository}`), only when it differs from
the stored value. Validation refuses with 400 naming `projects` a `selected`
list with no entry and the toggle off, more than 50 entries, a duplicate, or a
set or repository outside the workspace; a save that leaves Projects as stored is not
refused for a stored empty list (`005.2`). Error codes follow the Watches table of the
[permissions design](permissions.md): `projects_empty`, `projects_too_many`,
`projects_duplicate`, `projects_foreign_entry`, `invalid_projects`, all 400 with
the member named `projects`. An entry is checked against the sets and
repositories of the workspace, read through the repository set and repository
services before the lock (a failed read is 500 with nothing stored). The body is
first compared with the stored Projects as sent (equal means absent). Otherwise
a body entry that is in the stored list and no longer exists is dropped; any
other entry that does not exist in the workspace is `projects_foreign_entry`; a
list left empty by dropping with the toggle off is `projects_empty`. A stored
stale entry therefore never fails a save that resends it (`005.2`, `005.6`).
Equality ignores entry order and compares the scope, the entry set
`(kind, id)` and the toggle when the body scope is `selected`. When the body scope is `all`
the entries and the toggle are inert and are ignored: a body that sends `all` while the stored
scope is `all` is equal whatever entries or toggle value it carries (a toggle-only change under
`all` is therefore not a change: it archives nothing, raises no `policy_revision` and stores
nothing), and a body that sends `all` while the stored scope is `selected` is a change that
stores only the scope and leaves the stored entries and toggle as stored, so a no-op save never archives. Switching `selected` to `all`
keeps the entries and the toggle in storage (they are inert) so switching back
restores them. A change archives the conversation and
raises `policy_revision` once for the whole request (`005.7`). The request
applies in one transaction with the other members.

Request shape and precedence. `projects` absent or `null` means unchanged. `entries` absent or
`null` is the empty list; an `entry.kind` other than `repository_set` or `repository`, an empty `id`,
a missing `scope` or a `scope` other than `all` or `selected` is `invalid_projects`. The first
failing check in this order is the code: `invalid_projects`, `projects_empty`, `projects_too_many`,
`projects_duplicate`, then `projects_foreign_entry`. When the body switches to or keeps `all` its
entries are ignored: they are not validated, counted or stored, and the stored entries stay as they
are. The page puts the message in the single error region above the save bar prefixed "Projects: "
(naming the member like "Watches: "), keeps the draft, and on `projects_foreign_entry` refreshes
the set and repository listings and keeps the draft with the missing entries removed, as Watches
does for a deleted board; when that leaves a `selected` draft with no entry and the toggle off it
says "Projects: Keep at least one project in scope or include tasks with no repository."

Concurrency with the delete cascade. The cascade removes an entry under the same per-coordinator
lock a save takes and runs in its own transaction. A save validates against listings read before
the lock, so an entry removed by the cascade between that read and the save's transaction is
written back as a stale entry: this is accepted, since it matches no task (see Storage) and is
dropped by the next Projects save. A save that starts after the cascade finds the entry absent from
the listings and not in the stored list and answers `projects_foreign_entry`. Two saves on one
coordinator serialise on the lock; the later one compares with the value stored by the earlier.

## Read shape

`GET .../settings` carries, whenever the stored scope is `selected` (flag-free,
enforcement state), `projects: {scope, repository_ids, include_no_repository}` as
above (`repository_ids` null when the sets or repositories could not be read). While the phase 3.1 flag is effective the same payload also carries the
editor member `projects_config: {scope, entries: [{kind, id}],
include_no_repository}` for every coordinator, `all` included (scope `all`,
`entries: []` for one that never had a scope), with entries ordered by `kind`
(`repository_set` before `repository`) then `entry_id`. The editor compares its
draft with `projects_config` to decide whether Projects differs from stored
(`004.3`, `005.7`) and sends the `projects` write member only then. `projects_config`
is absent with the flag off, like the other Projects write-side fields.

The coordinator read `GET .../coordinators/:id` (the one the copilot hint reads, `getCoordinator` in
`coordinator-api.ts`) carries the same enforcement `projects` member inside `watches` under the
same rule (present while the stored scope is `selected`, whatever the flag), with one more
field, `names`: the display names of the listed sets and repositories in the Screens order
(sets by `LOWER(name)` then `id`, then loose repositories by `LOWER(name)` then `id`), omitted
when a listing read failed. While the phase 3.1 flag is effective the member is also present for
scope `all` as `{scope: "all"}` alone.

## Deletion cascade effects

Removing an entry by the delete-event cascade is a data repair, not a manager
save: it publishes `coordinator.updated` once per affected coordinator (so open
Configure and Needs you refresh) and does not archive the
conversation or raise `policy_revision`. The tool profile therefore keeps naming
nothing by project; the copilot hint reads the coordinator GET again on its own triggers (panel open, route
key change, coordinator switch), so it follows the repair at the next of those. The cascade is idempotent: a second
delivery of the same event removes nothing and publishes nothing. A cascade that fails (a store
error) logs a WARN naming the coordinator and the deleted entry and is not retried: the leftover
entry matches nothing and is dropped by the next Projects save.

## Screens

The Watches section keeps its board part and adds after it a **Projects** part
([watches UI](permissions-ui.md#watches-ui)):

```
Projects
[ ] Watch every project, including new ones     (on = all)
    Include tasks with no repository   [ ]  (shown on entering `selected`; on only if stored on)
    Sets
      Payments  (repo-a, repo-b)   In scope   [Take this project out of scope]
      Mobile    (repo-c)           Out        [Put this project in scope]
    Repositories not in a set
      repo-d                       Out        [Put this project in scope]
```

Sets and loose repositories come from the workspace's repository set and
repository listings for the workspace the settings page shows, with a skeleton
while loading, "Could not load projects" and **Try again** on failure (never
allowing a `selected` save, and keeping the switch on `all` only if it already
was), and the boards' 50 limit and inline message pattern ("At most 50 projects
can be watched.", "Keep at least one project in scope or include tasks with no
repository."). Each set shows its current repositories and says that a
repository added to it is included at once. The page sorts the two lists itself:
sets by `LOWER(name)` then `id`, loose repositories (those in no set when the editor loads) by
`LOWER(name)` then `id`; it does not rely on the listing endpoints' own order (the repository
listing returns newest first). Under `all` repository-less tasks are watched, and switching to `selected` with the toggle off stops
watching them, so the part says so beside the toggle ("Tasks with no repository are watched only
when this is on.") and the toggle shows the stored value (off for a coordinator that never had a
scope). Switching from `all` to `selected` starts the draft from the stored
entries; a coordinator that never had entries starts with every set and every
loose repository in scope (as the boards start with every board in scope): one
`repository_set` entry per set and one `repository` entry per loose repository, and no entry for a
repository that is only a member of a set, which the set already covers. A project is a set or a
loose repository, so the 50-entry limit counts those entries, and a workspace with more than 50
projects starts with the first 50 in the order sets then loose repositories, each by `LOWER(name)`
then `id`, and shows the inline limit message until the manager takes some out. A stored `repository` entry whose repository has since
joined a set is shown under that set as In scope and is stored as listed; taking
the set out of scope does not take the repository entry out, so the row shows the
repository as In scope through its entry, with its own **Take this project out of
scope** control. A stored entry that neither listing returns any more (a missed delete event) has no row and is
removed from the draft when the editor loads it; it counts neither toward the 50 limit nor toward
"the draft has an entry", and its removal makes the draft differ from `projects_config` so a save
sends the cleaned list. Save is disabled with that reason
while the draft has no entry and the toggle is off. A reader sees everything
disabled (`005.8`, `005.9`).

**Phone.** On a phone the Projects part is one column below the boards: the switch and the
"Include tasks with no repository" toggle are full-width rows with 44 px touch targets, each set
and each loose repository is a card (name, its repositories for a set, In scope or Out) with its
**Put this project in scope** / **Take this project out of scope** button as a full-width control
at the card's end, and the inline messages sit above the save bar like the boards' (UI-31-04 in
[the plan](../../../plans/workspace-coordinator-p3-1/plan.md#ui-31-04-projects-part-of-watches)).

**Empty selection notice.** The "watches nothing" notice of
[Watches UI](permissions-ui.md#watches-ui) covers the project half through the same component
(`watches-none-notice.tsx`, `watchesNoBoard`): when the stored scope is `selected` and the
effective boards are empty it keeps the existing sentence "This coordinator watches no board." with
**Choose boards**; otherwise, when the stored `projects` member is known (`repository_ids` is not
null) and the resolved `repository_ids` is `[]` with `include_no_repository` off (an empty list, or
sets that have lost their last repository), it shows the new sentence "This coordinator watches
no project." with **Choose projects** (a link to the same Configure Watches section, no button in the
Watches section itself, managers only for the link). Only one notice shows, boards first. The
trigger follows the stored value, never the draft, and an unknown `repository_ids` shows no notice (the banner line "Could
not load which projects this coordinator watches." of the [client
filter](#client-filter) covers it). The new copy is the keys `watchesNoneProjects`, `watchesChooseProjects`
and `watchesProjectsLoadFailed` in `coordinator.json`, beside `watchesNone` and
`watchesChooseBoards`, translated in all six locales, with no em dash.

**Guided setup.** `POST /coordinators/setup` (`setup.go`, `decodeSetupRequest`) gains an optional
`projects` member with the settings request's shape (`{scope, entries: [{kind, id}],
include_no_repository}`), read only while the phase 3.1 flag is effective; with the flag off the
member is ignored like any other unrecognised top-level member. Absent means scope `all`, toggle
off, no entry. Validation reuses the Save checks and codes (`invalid_projects`, `projects_empty`,
`projects_too_many`, `projects_duplicate`, `projects_foreign_entry`) and runs right after the
`watches` step in the existing order (identity, watches, projects, goal, context, may-do), failing
with 400 naming `projects` and the step `projects` (a new `setupStepProjects`, the wizard's optional
Projects step); a stale-entry drop does not apply, since a new coordinator has no stored list.
The entries are written in the same transaction as the coordinator row and the rest of setup, or none
of them. Setup is not a save: a new coordinator has no conversation to archive, so it archives nothing and
raises no extra `policy_revision`. The step is skipped by leaving it untouched, which sends no member.

**Copilot hint.** The copilot's scope hint is the existing "Not watched by this coordinator" hint
and its host (`use-coordinator-watches.ts`, `copilot-everywhere.md`, `002.4`), which reads
`watches` from the coordinator GET and re-reads on panel open, route key change and coordinator
switch only; it is extended, not replaced, and it subscribes to no event. It shows "Not watched by
this coordinator" when the loaded `watches` says the context is outside by either half: the
workflow is not watched (unchanged), or `watches.projects` is present with scope `selected` and the
context is a task whose repositories are known (from `kanban.tasks`, by the one function that
builds the repository list from a task payload, see [Client filter](#client-filter)) and
`InProjects` is false for them; a context with no known repository list, a board context, or a
`projects` member whose `repository_ids` is null does not show it, like the existing no-known-workflow
rule. Beside it, whenever `watches.projects` is present, a second line names the projects (`005.8`): under
`selected` the sentence "Projects: " followed by the `names` of the member in their order, at most 5
then "and N more", plus "and tasks with no repository" when `include_no_repository` is on; under `all`
"Projects: all". It is omitted when `names` is absent (a listing failure) and never blocks the
first line. The text lives in the `coordinator` or `copilot` namespace through `t()` with the six
locales, no em dash.

## Error handling

| Failure | Behaviour |
| --- | --- |
| Task repository read fails | Task treated as outside the projects (fail closed), logged |
| Repository list fails to load in settings | Failure state, no `selected` save |
| Set or repository deleted | Entry removed from every list; may leave a coordinator watching nothing |
| Set or workspace-repository listing read fails (the resolver) | Task treated as outside the projects (fail closed), logged. A conversation or item read tool then answers with the phase-1 not-found shape, and `list_tasks_kandev` returns an error result naming `projects` (not an empty board) so the agent can tell a failed read from an empty one; the wake recorder drops that wake with a WARN log and no retry, and the next wake of the task re-evaluates; the backstop skips that coordinator for the pass; the delivery recheck aborts without superseding anything (Enforcement paths); the server-built `repository_ids` of a read is null and the copilot names list is omitted |

## Testing

Predicate matrix (all, listed, none listed, no repository with the toggle on and
off, multi-repository), each enforcement path with a task outside, a repository added to and removed
from a listed set, `create_task` repository refusals, save validation, set and
repository deletion cascade, stale
proposal stays approvable, the flag-off path identical to phase 3, and the
client filter mirror of the server matrix.

Also covered:

- `list_tasks_kandev` omits out-of-scope tasks and `Total` counts only the
  visible ones.
- A delivery recheck whose resolver or listing read fails aborts without
  superseding the wake, leaves it pending for the next backstop pass and
  increments `coordinator_delivery_aborted_total` with reason
  `projects_read_failed`.
- The open watched count takes the Go path under `selected` and the SQL path
  under `all`.
- The setup `projects` member: validated after watches, step `projects` on error,
  ignored when the flag is off.
- A toggle-only change of `include_no_repository` under `all` is not a Projects
  change (no archive, no `policy_revision` raise).
- The watches hook re-reads on repository and set events while active.
- Page-side ordering by `LOWER(name)` then `id`, and the initial draft of one
  entry per set plus one per loose repository.
- The copilot hint second line, the empty-selection notice and the failed-load
  banner copy in all six locales.
