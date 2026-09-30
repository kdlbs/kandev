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
`include_no_repository` (bool, default true). New table
`coordinator_watch_projects(coordinator_id, entry_kind, entry_id)` with
`entry_kind` in `repository_set` or `repository`, primary key on all three,
stored as a list of filters so a later group filter is one more `entry_kind`
(`005.1`). Deleting a set or a repository removes its entries in the deletion
path's existing hook (the one that prunes set membership and publishes the
changed sets), and deleting the coordinator removes its rows (`005.6`). A
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
request or per batch and never stores the result on the coordinator, so adding
a repository to a set widens every coordinator that lists it on the next read
(`005.3`). A task in several repositories is in scope when any one matches.
`watch.Task(task)` is the single function that combines the workflow test and
`InProjects`; every path below calls it and none compares repositories
directly (`005.4`).

The predicate resolves to the stored fields when the phase 3.1 flag is
effective and to "no project filter" otherwise, so flag-off behaviour is
phase 3 exactly (`005.9`).

## Enforcement paths

Each path already calls the phase 2 workflow filter; the change is that the call
site uses `watch.Task`. Where a path only holds a workflow id (list workflows,
list steps), it is unchanged, since a workflow is not project-scoped.

| Path | Effect |
| --- | --- |
| `list_tasks_kandev`, `get_task_conversation_kandev`, `get_coordinator_item_kandev` | outside the projects: phase-1 not-found error |
| Propose tools | target outside the projects: refusal naming the field; `create_task`: a `repository_id` that is neither listed nor in a listed set, or empty with the toggle off, refused naming `repository_id` (`005.5`) |
| Wake recorder and backstop | a task outside the projects raises no wake |
| Needs you, Queue, counts, stalls | filtered on the client by the same rule (below) |
| Automatic class counts | count only proposals whose target is in the projects at the time of counting |
| Turn ledger snapshot and digest | snapshot and digest use `watch.Task`; the row stores `project_scope` as it was |
| Dream window and evidence | proposals of tasks outside the projects are dropped from evidence |

A `create_task` proposal names one optional `repository_id`; the refusal rule is
the whole check, since a proposal has a single repository. Approve does not
re-check projects: a proposal made in scope stays approvable, as for Watches
(`005.5`).

## Client filter

The pure module `apps/web/lib/coordinator/watch-filter.ts` gains `isTaskWatched`
input: the effective watch set from `GET .../settings` now carries `projects:
{scope, repositoryIds, includeNoRepository}`, where `repositoryIds` is the
server-resolved union of the listed repositories and the current members of the
listed sets, so the client never resolves sets itself and a set change reaches
open screens through `coordinator.updated` and the repository set events. `AttentionTask` gains `repositoryIds: string[]`, set from
the snapshot. The rule is the server's: workflow watched and `InProjects`. An
unknown repository list (snapshot lacks it) is treated as not watched, the same
fail-closed rule as a missing workflow id. The "not available" and "watches
nothing" states of the phase 2 screens apply unchanged; the "watches nothing"
notice also covers the empty project selection.

## Save

The settings request that carries May do and Watches also carries `projects`
(`{scope, entries: [{kind, id}], include_no_repository}`), only when it differs from
the stored value. Validation refuses with 400 naming `projects` a `selected`
list with no entry and the toggle off, more than 50 entries, a duplicate, or a
set or repository outside the workspace; a save that leaves Projects as stored is not
refused for a stored empty list (`005.2`). A change archives the conversation and
raises `policy_revision` once for the whole request (`005.7`). The request
applies in one transaction with the other members.

## Screens

The Watches section keeps its board part and adds after it a **Projects** part
([watches UI](permissions-ui.md#watches-ui)):

```
Projects
[ ] Watch every project, including new ones     (on = all)
    Include tasks with no repository   [x]
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
repository added to it is included at once. Save is disabled with that reason
while the draft has no entry and the toggle is off. A reader sees everything
disabled (`005.8`, `005.9`). The guided setup adds an optional Projects step
with the same choice; the copilot's scope hint names the projects (`005.8`).

## Error handling

| Failure | Behaviour |
| --- | --- |
| Task repository read fails | Task treated as outside the projects (fail closed), logged |
| Repository list fails to load in settings | Failure state, no `selected` save |
| Set or repository deleted | Entry removed from every list; may leave a coordinator watching nothing |
| Set listing read fails | Task treated as outside the projects (fail closed), logged |

## Testing

Predicate matrix (all, listed, none listed, no repository with the toggle on and
off, multi-repository), each enforcement path with a task outside, a repository added to and removed
from a listed set, `create_task` repository refusals, save validation, set and
repository deletion cascade, stale
proposal stays approvable, the flag-off path identical to phase 3, and the
client filter mirror of the server matrix.
