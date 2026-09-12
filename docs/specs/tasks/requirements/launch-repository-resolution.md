---
status: draft
system: tasks
created: 2026-09-09
owners:
  - kandev
---
# Session Launch Repository Resolution Requirements

## Overview

A task session launch must build its launch request from the repositories the
task is attached to. Today it builds them from a snapshot on the session row,
while the persistence guard admitting the resulting environment reads the
`task_repositories` table. The two disagree whenever a task acquires its first
attachment after one of its sessions was already created with an empty
preference (the zero-to-one transition; see `## Prior art` and AC-001.3), and
every later launch of that session fails with:

```text
create task environment: create task environment: ready status requires
repository inventory for repo-backed task (task=<id>)
```

It reaches every entry point that resumes an existing session — workflow step
auto-start, a Backlog bounce, and `session.recover` with
`action: "fresh_start"` — and there is no supported recovery, because every
recovery route is itself a resume. Measured on one instance on 2026-09-09: eight
live sessions holding 6,124 conversation messages.

This system owns the contract because launch behavior is a task-system
responsibility. The [workspace system](../../workspaces) owns repositories and
worktrees; this requirement changes only which repository set a launch is built
from.

## Terminology

- **Attachment set:** the task's rows in `task_repositories`, the durable record
  of which repositories a task is bound to and on which base branch. This is the
  set [Attach Workspace Sources](attach-workspace-sources.md) writes, and the set
  the environment inventory guard reads.
- **Session repository preference:** `task_sessions.repository_id` and
  `task_sessions.base_branch`, stamped from the task's primary attachment at
  session creation and never rewritten. A preference, not a record of what the
  task is attached to.
- **Environment repository inventory:** the `task_environment_repos` rows
  describing the repository slots a materialized task environment contains. A
  ready environment with an empty inventory for a task that has attachments is
  rejected at the persistence boundary.
- **Resume launch:** any launch of an already-created session, whatever
  triggered it. All build their request through the same resume path.

## Prior art

**Our own prior reasoning (wiki).** Receipt: config resolved through
`~/.obsidian-wiki/config` -> `config.henry`, giving
`OBSIDIAN_VAULT_PATH=/Users/henry/Documents/henry/wiki` and
`QMD_WIKI_COLLECTION="wiki"`. The leg could not then run: no QMD transport, no
`qmd` or `obsidian-wiki` CLI, and the vault unreadable from this worktree
(`EPERM`, macOS privacy on `~/Documents`), which rules out the grep fallback too.
It returned nothing useful — through tool unavailability, not an empty vault, so
this is no evidence the wiki lacks a position.

**What other products shipped (saas-kb).** Receipt: the `saas-kb` MCP server and
its `search_fsm_docs` tool are absent from this session's tool list and cannot be
loaded on demand. No queries ran; it returned nothing useful.

**Prior art inside this repository, which was readable and is the source of
the decisions below.** [Tasks Without Repositories](without-repositories.md)
created repository-less tasks and explicitly excluded "attaching a repo to a
repo-less task after creation". That exclusion has never been lifted, and this
matters for how the defect is reproduced.

[Attach Workspace Sources](attach-workspace-sources.md) is the closest prior
contract: its `AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.5` promises attachment
"shall preserve task state, plans, conversations, sessions, and existing
attachments" — the promise this defect breaks one step later, when the preserved
sessions turn out not to be launchable. But that operation is **not** the path
that produces the defect: `Service.AttachWorkspaceSources` refuses a task with no
attachments yet (`task must have a repository before attaching workspace
sources`), so it cannot perform the zero-to-one transition at all. The only
unconditional writer is the generic task update, `UpdateTask` delegating to
`replaceTaskRepositories`, which recreates the whole set with no precondition on
the current count.

**This is the defect's only reachable precondition.** The session preference is
stamped once, at session creation, from the task's primary attachment, so it is
empty if and only if the task had no attachments then. The failure therefore
requires a task that was repository-less when its session was created and
acquired its first attachment afterwards — zero-to-one, through the task update
path. Any reproduction, regression test or end-to-end scenario must drive that
path; one built on `AttachWorkspaceSources` is rejected before it can set up the
state.

The same divergence was found and fixed once already in the neighbouring
`applyResumeMultiRepoConfig`, whose comment records that gating on the task
record's in-memory repository collection "silently dropped every repo but the
primary on any resume of a multi-repo task". The fix there was to read the
database-backed set; this requirement applies it to the primary, left behind.

## Requirements

### REQ-TASKS-LAUNCH-REPOSITORY-RESOLUTION-001: Launches resolve repositories from the task attachment set

**Intent:** A session launch and the guard admitting its environment must read
the same record of which repositories the task has, so attaching a repository to
an existing task makes it launchable rather than permanently unlaunchable.

**User story:** As a user who attached a repository to a task already started,
I want it to launch and keep its conversation, so repairing a repository-less
task does not cost me the work already in it.

#### Acceptance criteria

- **AC-TASKS-LAUNCH-REPOSITORY-RESOLUTION-001.1:** When a launch builds its
  request for a session that carries no session repository preference, the
  system shall resolve the primary repository from the task's attachment set,
  and shall select the same attachment that the task's primary-attachment read
  returns.
- **AC-TASKS-LAUNCH-REPOSITORY-RESOLUTION-001.2:** When the attachment set is
  ordered to select a primary, the system shall order it by `position`
  ascending, then `created_at` ascending, then `task_repositories.id`
  ascending, so that the selection is total and returns the same attachment on
  every read of unchanged data.
- **AC-TASKS-LAUNCH-REPOSITORY-RESOLUTION-001.3:** When a session carries a
  non-empty session repository preference naming a repository that is present in
  the task's attachment set, the system shall use that repository as the primary
  unchanged, and shall not re-derive the primary from the attachment set's
  ordering. A preference naming a repository absent from the attachment set is
  excluded below and is left exactly as it behaves today.
- **AC-TASKS-LAUNCH-REPOSITORY-RESOLUTION-001.4:** When a task with a non-empty
  attachment set launches a session that materializes or updates its own task's
  environment, and that session carries either no repository preference or a
  preference naming a repository present in the set, the system shall configure
  the launch with every attachment in that set, and shall record one environment
  repository inventory row per attachment, whether the set holds one attachment
  or many.
- **AC-TASKS-LAUNCH-REPOSITORY-RESOLUTION-001.5:** When a task has an empty
  attachment set and its session carries no repository preference, the system
  shall launch with no repository configuration and no environment repository
  inventory rows, and shall not report a missing inventory. A task with an empty
  attachment set whose session still carries a preference is excluded below.
- **AC-TASKS-LAUNCH-REPOSITORY-RESOLUTION-001.6:** When the read that resolves a
  launch's repositories from the task's attachment set fails, the system shall
  fail that launch and report the read failure, and shall not proceed as though
  the task had no attachments.
- **AC-TASKS-LAUNCH-REPOSITORY-RESOLUTION-001.7:** When a task session was
  created before a repository was attached to its task, the system shall launch
  that session successfully on the next launch whose attachment-set read follows
  that attachment, with its conversation, plan, and session identity intact,
  without requiring a new session, a database edit, or a data migration.
- **AC-TASKS-LAUNCH-REPOSITORY-RESOLUTION-001.8:** When a repository is
  attached to a task that already has sessions, the system shall leave every
  existing session's stored repository preference and base branch unchanged.
- **AC-TASKS-LAUNCH-REPOSITORY-RESOLUTION-001.9:** When a session recovery
  action clears the resume token and relaunches the session, the system shall
  resolve repositories by the same rules as any other launch of that session.
- **AC-TASKS-LAUNCH-REPOSITORY-RESOLUTION-001.10:** When the same session is
  launched repeatedly, the task's attachment set and the session's preference are
  unchanged between attempts, and an attempt reaches and completes repository
  resolution rather than being rejected or failing before it completes, including
  the rejections and the re-read failure under
  `AC-TASKS-LAUNCH-REPOSITORY-RESOLUTION-001.12`, the system shall resolve the
  same primary repository, the same attachment set, and the same environment
  repository inventory on every such attempt. An attempt rejected before it
  reaches resolution resolves nothing and is outside this criterion.
- **AC-TASKS-LAUNCH-REPOSITORY-RESOLUTION-001.11:** When a task's attachment set
  is empty at the moment a launch reads it and non-empty at the moment that
  launch writes its environment, the system shall fail the launch, shall not as a
  result of that launch bring any environment to ready and shall write no
  repository inventory rows, and shall allow an immediate subsequent launch to
  succeed against the new set. This criterion constrains only what the failed
  launch itself creates or transitions. A ready environment that already existed
  before the launch is left as it is, and is not required to be deleted or
  invalidated: a task that was repository-less when its session started may
  legitimately own one holding an empty inventory from that period, and such an
  environment does not prevent the subsequent launch from succeeding.
  Attachment-set changes that do not cross empty to non-empty are excluded below.
- **AC-TASKS-LAUNCH-REPOSITORY-RESOLUTION-001.12:** When two launches of the
  same session run concurrently, the system shall serialize them on the
  per-session lock rather than rejecting the second on arrival: the second launch
  shall block until the first releases the lock, and shall re-read the session's
  stored state inside the lock before deciding whether to proceed, without having
  changed the environment's repository inventory before that re-read. A re-read
  that fails, which is also how a session that no longer exists is reported,
  shall fail the launch and report that failure rather than proceed on the state
  read before the lock, resolving no repositories;
  `AC-TASKS-LAUNCH-REPOSITORY-RESOLUTION-001.10` excludes it. A re-read that
  succeeds has exactly three outcomes. The system shall report that an execution
  is already running when it shows a non-terminal session with an execution
  registered for it. It shall report the session state as superseded when it
  shows a terminal state different from the one that launch requested.
  When neither holds, and in particular when the first launch failed and restored
  the session to the same state this launch requested, the system shall reject
  nothing: the second launch shall proceed and resolve repositories by the rules
  above, because no execution is then running and the second launch is a
  legitimate retry rather than a duplicate.

### REQ-TASKS-LAUNCH-REPOSITORY-RESOLUTION-002: Base branch follows the attachment that supplied the repository

**Intent:** A primary repository resolved from the attachment set must carry
that attachment's branch, so a repaired task checks out the branch the user
configured rather than a global default.

**User story:** As a user whose attached repository uses a non-default base
branch, I want the launch to use it, so the repair does not silently start my
work from the wrong base.

#### Acceptance criteria

- **AC-TASKS-LAUNCH-REPOSITORY-RESOLUTION-002.1:** When the primary repository
  is resolved from the attachment set, the session carries no base branch, and
  the task holds exactly one attachment for that repository, the system shall use
  that attachment's base branch. When the task holds more than one attachment for
  that repository the match is ambiguous, and the system shall use the session's
  stored base branch instead — which, for a session carrying none, leaves the
  primary's base branch unset — because the session preference predates exact row
  identity and cannot name which attachment row it meant. Each attachment is
  still prepared against its own base branch under
  `AC-TASKS-LAUNCH-REPOSITORY-RESOLUTION-002.3`.
- **AC-TASKS-LAUNCH-REPOSITORY-RESOLUTION-002.2:** Under `AC-...-002.1`'s
  antecedent, its empty session base branch included: when that attachment's base
  branch is empty, the system shall use the repository's default branch. When the
  repository's default branch is also empty and the launch runs on the worktree
  executor, the system shall apply the built-in default base branch to the
  single-repository launch fields rather than launching with no branch
  configured. On every other executor type no built-in default is applied to
  those fields; and on no executor type, worktree included, is one applied to the
  per-attachment repository specifications a multi-attachment launch carries. In
  those cases the launch proceeds with that base branch unset, exactly as today;
  the gap is named under `## Out of scope` rather than closed here.
- **AC-TASKS-LAUNCH-REPOSITORY-RESOLUTION-002.3:** When the task holds more
  than one attachment for the resolved primary repository, the system shall
  prepare each attachment against its own base branch, and shall not apply one
  attachment's branch to another.
- **AC-TASKS-LAUNCH-REPOSITORY-RESOLUTION-002.4:** When a session carries a
  non-empty base branch and the task holds exactly one attachment for the
  resolved primary repository, the system shall prefer that attachment's base
  branch over the session's stored value when the attachment's is non-empty, and
  shall otherwise keep the session's stored value, preserving the existing
  recovery behavior in which a base-branch repair on the task takes effect on the
  next launch.

## Out of scope

- **Writing `task_sessions.repository_id` when a repository is attached.**
  Rejected deliberately, not overlooked: it would not repair the already-broken
  sessions, which predate any such write, and a stamped value goes stale again as
  soon as the task update replaces the whole attachment set. The defect here is
  that two writers disagree about one fact; a third writer of it makes that
  likelier, not less. The session's repository field remains a per-session
  preference.
- **A repair migration or a direct `UPDATE` on existing broken sessions.** Not
  required once launches resolve from the attachment set: the eight known rows
  heal on their next launch. A later migration is separate work with its own
  justification.
- **Any launch whose session preference names a repository that is not in the
  task's attachment set.** This covers two states, and both are excluded for the
  same reason. First, a *detached* preference: the task still has attachments but
  the preferred repository is no longer among them. Today that repository is still
  resolved as the primary and the inventory written is not self-consistent — a
  task with one remaining attachment records the preferred repository, one with
  several records the attachments instead. Second, a *repository-less task whose
  session still carries a preference*: the launch is configured from that
  preference and writes one inventory row, and the guard does not object because
  there are no attachments to compare against.

  Neither is the reported defect, which requires an EMPTY preference, so every
  session in the measured population is outside both. Both keep today's behavior:
  `AC-...-001.3`, `AC-...-001.4` and `AC-...-001.5` are each scoped to a
  preference that is empty or present in the set, so none reaches or changes
  these cases. A follow-up would decide first whether a session
  should keep preferring a repository the task no longer has.
- **The model-switch relaunch path.** It reads the session preference with no
  attachment fallback, so it has the same shape, but performs no environment
  write and cannot produce this failure. Whether it misbehaves is unverified.
- **Sessions that bind to another task's environment**, which Office
  `inherit_parent` and `shared_group` membership produces. The owning task owns
  that environment's inventory; a guest launch neither rewrites it nor
  re-evaluates its readiness, and that is unchanged here.
- **Attachment-set changes mid-launch that do not cross empty to non-empty.**
  `AC-...-001.11` is scoped to the empty-to-non-empty crossing because that is
  the only transition the guards can detect: they compare a written inventory
  against *whether the task has any attachments at all*, never how many or which.
  A launch that reads a one-attachment set while a second is being added writes a
  one-row inventory no guard rejects, and the environment becomes ready holding a
  stale, partial inventory. That window is real and accepted as a residual.

  It is accepted on two grounds: it is not the reported defect, which needs no
  concurrency to reproduce, and closing it means re-reading the attachment set
  inside the transaction that writes the inventory and failing on mismatch — new
  work at a persistence boundary this requirement leaves untouched. Nothing serializes
  the write against a launch on that path: the idle check belongs to
  [Attach Workspace Sources](attach-workspace-sources.md) alone, and the generic
  task update takes none. A follow-up
  would need to decide which deltas must fail a launch: membership changes only,
  or also a base-branch edit or a `position` reorder that leaves membership
  identical.
- **A built-in terminal base-branch default for executor types other than
  worktree.** `AC-TASKS-LAUNCH-REPOSITORY-RESOLUTION-002.2` gives the worktree
  executor's single-repository launch fields a built-in default because that path
  already has one. No other executor type has an equivalent and none is added:
  when an attachment's base branch, the repository's default branch and the
  session's stored branch are all empty, a launch on those executors proceeds with
  the primary's base branch unset, as today. The same holds for the per-attachment
  specifications a multi-attachment launch carries, on any executor type including
  worktree.

  This is a real gap for the population this requirement repairs, whose evidence
  trail records a remote executor rather than a worktree one. It is left as it is
  because it is pre-existing and unchanged by resolving the primary from the
  attachment set, it needs the repository's own default branch to be missing too,
  and adding a default for every executor type would change behavior on
  materialization paths this requirement leaves untouched. A follow-up would
  decide whether that default is per-executor or global, and whether it applies
  to each attachment's specification or only the primary.
- **The environment repository inventory guards themselves.** They exist because
  a ready environment with an empty inventory for a task that has attachments
  breaks reuse. They are not relaxed, weakened or bypassed.
- **Detaching repositories, and any promote/demote flow** beyond what
  [Attach Workspace Sources](attach-workspace-sources.md) specifies; and **the
  attachment user interface and the attach operation's own validation, rollback
  and materialization**, which that requirement owns.
- **Executor-specific materialization**: how a given executor clones, mounts or
  worktrees a repository once the launch request names it.
