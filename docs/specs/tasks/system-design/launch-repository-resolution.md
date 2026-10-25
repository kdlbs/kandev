---
status: draft
system: tasks
requirements:
  - REQ-TASKS-LAUNCH-REPOSITORY-RESOLUTION-001
  - REQ-TASKS-LAUNCH-REPOSITORY-RESOLUTION-002
---

# Session Launch Repository Resolution System Design

## Purpose and boundaries

The task system owns how a launch request is built from durable task state.
This design fixes which record is authoritative for "what repositories does
this task have" during a launch, and makes the launch and the environment
persistence guard read that same record.

It uses, but does not own: repository entities and clones
([workspaces](../../workspaces)), executor materialization
([executors](../../executors)), and the attach operation's own validation and
rollback ([Attach Workspace Sources](attach-workspace-sources.md)).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-LAUNCH-REPOSITORY-RESOLUTION-001` | [Two readers, one authoritative record](#two-readers-one-authoritative-record), [Decision: stamp the primary repository identity on every executor type](#decision-stamp-the-primary-repository-identity-on-every-executor-type), [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery) |
| `REQ-TASKS-LAUNCH-REPOSITORY-RESOLUTION-002` | [Branch resolution](#branch-resolution) |

## Two readers, one authoritative record

Two components answer the same question from different places.

**The guard** reads the table. `taskHasRepositoriesTx`
(`apps/backend/internal/task/repository/sqlite/task_environment.go:628`) runs
`SELECT COUNT(1) FROM task_repositories WHERE task_id = ?` and is consulted at
four sites: environment creation (`:59`), materialization finalization (`:323`),
and two checks inside `PersistTaskEnvironmentTransition` (`:371`) — the worktree
workspace-path guard in `validateTaskEnvironmentTransitionWorkspace` (`:412`,
consulting the guard at `:422`) and the ready-transition guard in
`validateReadyTaskEnvironment` (`:249`, consulting it at `:265`), reached through
the sibling `validateTaskEnvironmentTransitionReady` (`:431`). Those last two are
siblings, not a chain, and they enforce different invariants: `:422` fails with
"worktree-mode env requires workspace_path", so only `:59`, `:265` and `:323`
police the ready-with-empty-inventory contract. That contract is what the guard's
own comment states: only a task with no repositories configured at all may
legitimately have no inventory.

**The launch** reads the session. `resolveResumeRepoIDAndBranch`
(`apps/backend/internal/orchestrator/executor/executor_resume.go:1775`) takes
`session.RepositoryID` and falls back to `task.Repositories[0]`. That fallback
cannot fire: `validateAndLockResume` loads the task through the raw
`Repository.GetTask` (`task.go:512`), which selects task columns only and never
attaches the one-to-many `task_repositories` rows. The neighbouring
`applyResumeMultiRepoConfig` already documents this in its own doc comment,
having been fixed for the same reason.

So when a repository is attached after a session exists, `repositoryID` stays
empty, `applyResumeRepoConfig` returns early at `executor_resume.go:1590`
before it can populate either the single-repo fields or `req.Repositories`,
`environmentReposForLaunch` (`executor_execute.go:2622`) falls through to its
last branch and returns `nil` (`:2650-2651`), and the guard refuses a ready
environment with no inventory for a task that has one row.

Resolving the primary correctly is necessary but **not sufficient**: even with a
resolved primary, the launch request only carries that repository's identity when
the executor is the worktree one. That is the second half of this design; see
[stamp the primary repository identity](#decision-stamp-the-primary-repository-identity-on-every-executor-type).

**The record the launch already holds.** `applyResumeRepoConfig` opens by
calling `resumeRepoSet`, which resolves the task's attachments through
`ListTaskRepositories` — the same table the guard reads, ordered `position
ASC, created_at ASC, id ASC`. The authoritative set is therefore already in hand,
fully resolved, before the session-derived primary is chosen. The primary
selection is the only step that reaches for a different source.

### Decision: resolve the primary from the resolved attachment set

Take the resolved set as the source for the primary when the session carries
no preference, and delete the `task.Repositories` fallback rather than leave a
branch that reads as coverage and cannot execute.

**Scope boundary.** This changes the empty-preference path only. A session whose
preference names a repository still present in the attachment set already
resolves to that repository and keeps doing so. A session whose preference names
a repository *absent* from the set — detached, or left behind on a task with no
attachments at all — is excluded by the requirement and must come out of this
change behaving exactly as it does today, including the inventory it writes.
That case is reached through the same `GetRepository` fallback in
`applyResumeRepoConfig`; do not "tidy" it while removing the dead branch above.
The two are adjacent in the same function and only one of them is dead.

The alternative — populating `task.Repositories` on the resume path so the
existing fallback becomes live — was audited and rejected. Measured
2026-09-09: `task.Repositories` is read in roughly thirty non-test backend
files, and the comment on `applyResumeMultiRepoConfig` records that this
field's emptiness on the resume path is load-bearing for at least one of them.
Making it non-empty changes what every one of those readers sees on a path
they were written against. Within the executor package the only reader of
`task.Repositories` is the dead fallback itself
(`executor_resume.go:1655-1660`), so removing it costs nothing and the audit
for the alternative is the expensive one.

### Decision: stamp the primary repository identity on every executor type

`environmentReposForLaunch` (`executor_execute.go:2622`) projects inventory rows
from three sources in order: the per-repository worktree results, then
`req.Repositories`, then the single-repository `req.RepositoryID`. On the resume
path that last field is written at exactly one site, `executor_resume.go:1760`,
inside `applyResumeWorktreeConfig` — reached only when
`shouldUseWorktree(req.ExecutorType)` (`executor_execute.go:660`) holds, which is
true for `ExecutorTypeWorktree` alone. Neither `applyResumeRepoBasics` (`:1691`)
nor `applyResumeCloneURL` (`:1707`) writes it; both write `RepositoryURL` only.
And `applyResumeMultiRepoConfig` (`:1736`) populates `req.Repositories` only when
the resolved set holds more than one attachment.

So a task holding exactly **one** attachment on a **non-worktree** executor
reaches `environmentReposForLaunch` with no worktree results, an empty
`req.Repositories` and an empty `req.RepositoryID`, falls through to
`:2650-2651`, returns `nil`, and is refused by the guard exactly as before — even
though the primary resolved correctly. That is not a corner case: the population
this design repairs runs on a remote executor, recorded under the intervention
class `repo-attach-breaks-ssh-launch`, and SSH is not the worktree executor.

**The asymmetry is one conjunct.** The initial-launch path
(`applyRepositoryConfig`, `executor_execute.go:1951`) stamps the identity at
`:1954` under `repoInfo.RepositoryPath != ""` alone, with no executor-type
condition. The resume path stamps it under `shouldUseWorktree(...) &&
repositoryPath != ""`. That extra conjunct is the whole of this half of the
defect. It stays invisible on a healthy task because the environment already
carries inventory rows from its initial launch, so the update branch's
`len(existingEnv.Repos) == 0` condition (`executor_execute.go:2493`) is false. The
repaired population's environment was materialized while the task was still
repository-less, so its inventory is genuinely empty and the refusal fires.

**Responsibility.** Whenever `applyResumeRepoConfig` resolves a primary
repository, the launch request carries that repository's identity —
`req.RepositoryID` — on every executor type. `environmentReposForLaunch` then
receives an identity it already knows how to project and needs no change of its
own.

**The stamp is unconditional on the resolved primary.** It is deliberately NOT
gated on a non-empty local repository path, which is where it departs from the
initial-launch path: `applyRepositoryConfig` conditions its whole block on
`repoInfo.RepositoryPath != ""` (`executor_execute.go:1952`). That path can be
empty on a *successful* resolution, by three routes reachable from
`ensureRepoLocalPathForSessionAndState` (`executor_resume.go:461`) — a
local-source repository with no recorded path or no cloner (`:465-466`), a
repository with no provider identity (`:485-486`), and, in the callee
`ensureRepoClonedForSessionAndState`, a nil cloner, which logs a warning and
returns no error (`:518-524`). A resolved primary with no local clone is
therefore reachable, and gating the resume stamp the way the initial path gates
its own would leave exactly that population producing no inventory row and
refused by the guard — the reported defect, unfixed.
`AC-TASKS-LAUNCH-REPOSITORY-RESOLUTION-001.4` requires one inventory row per
attachment with no local-path condition, so the stamp follows the attachment set,
not the clone.

**This is the repository's identity only.** It does not extend the worktree
executor's built-in terminal base-branch default to any other executor type, and
it does not give the per-attachment specifications one. `AC-...-002.2` and the
requirement's matching `## Out of scope` bullet keep that boundary exactly where
they put it; see [Branch resolution](#branch-resolution). The two travel on
different fields and only the identity moves here.

**Blast radius.** Stamping unconditionally does introduce one request shape the
initial-launch path never produces — `RepositoryID` non-empty while
`RepositoryPath` is empty — because on that path the two are set together inside
the same `RepositoryPath != ""` block. It is not enough to observe that these
readers already see a non-empty identity on a non-worktree initial launch; each
was checked against the new shape specifically, and none reads the path in a way
that changes its outcome:

- `failingLaunchRepositoryIdentity` (`executor_execute.go:1462`) returns the
  identity pair for launch-failure attribution and never reads the path.
  `handleEarlyLaunchFailure` (`:1556`) already passes an empty task-repository id
  across the same boundary, so a partially-populated identity is pre-existing
  there.
- `environmentReposForLaunch` (`:2650`, `:2667`) projects the identity and a
  branch slug. `TaskEnvironmentRepo` has no repository-path column, and the
  physical-worktree fields are filled from the launch response, not the request.
- `topLevelLaunchRepoSpec` (`executor_environment_reuse.go:379`) copies the path
  into a `RepoSpec`, but its only reachable non-worktree consumer is
  `canonicalInventoryMatches` (`:112`), which matches on `RepositoryID` and
  branch slug alone. Its other caller, `reuseExistingRepositoryWorktrees`
  (`:271`), returns early on `!req.UseWorktree`.

Whether a launch that resolves a primary it cannot clone then succeeds is
materialization's concern, not this design's. The inventory row records the
repository slot the task is attached to, which is true whether or not a clone
exists yet.

**Rejected alternative: drop the `len(allRepos) > 1` gate** so a single
attachment also populates `req.Repositories`. It satisfies the same criteria, but
`applyResumeMultiRepoConfig` also sets `req.TaskDirName` and routes the launch
down the multi-repository preparer path, so it would change materialization for
every single-repository worktree resume — tasks that are not broken. The narrower
change touches only the executor types that are.

### Decision: no session back-fill

`task_sessions.repository_id` stays a per-session preference written once at
`prepareSession` (`executor_execute.go:939`) from
`GetPrimaryTaskRepository`. Neither the attach path
(`service_workspace_sources.go`) nor `replaceTaskRepositories`
(`service_tasks.go:1843`) writes it, and neither should start.

Resolving at launch repairs the already-broken sessions on their next launch,
with no migration. A back-fill would not: it can only stamp sessions from the
moment it ships, so the existing rows would still need a repair migration.
`replaceTaskRepositories` also deletes and recreates the whole attachment set,
so a stamped value can be left naming a row that no longer exists — the same
divergence, re-created by a third writer of the same fact.

## Components and responsibilities

| Component | Responsibility |
| --- | --- |
| `Executor.applyResumeRepoConfig` (`executor_resume.go:1573`) | Owns launch-time repository resolution. Already resolves the authoritative set; gains ownership of the primary selection, and of stamping the resolved primary's identity on the request for every executor type rather than only the worktree one. |
| `resolveResumeRepoIDAndBranch` (`executor_resume.go:1775`) | Selects the primary. Must take the resolved set as input instead of the task record, and lose the unreachable branch. |
| `Repository.ListTaskRepositories` (`task_repository.go:84`) | Authoritative ordered read of the attachment set. Its `ORDER BY` is the primary-selection order. |
| `Repository.GetPrimaryTaskRepository` (`task_repository.go:255`) | Defines "primary" as the first row of that read. The launch must agree with it, not define a second primary. |
| `environmentReposForLaunch` (`executor_execute.go:2622`) | Projects the launch request into inventory rows. Unchanged in itself — but it is correct only once the request actually carries the primary's identity, which on a non-worktree executor it does not today. See [stamp the primary repository identity](#decision-stamp-the-primary-repository-identity-on-every-executor-type); without that half, this component still returns `nil` at `:2650-2651` for a single-attachment non-worktree launch and the guard still refuses. |
| `taskHasRepositoriesTx` (`task_environment.go:628`) | The guard. Unchanged. |

Backend only. No frontend component changes: the user-visible outcome is that
an existing launch action succeeds where it previously failed.

## Data and contracts

- `task_repositories` — the attachment set. `UNIQUE(task_id, repository_id,
  base_branch, checkout_branch)`, so one repository may legitimately appear
  more than once with different branches. `position INTEGER DEFAULT 0`.
- `task_sessions.repository_id`, `task_sessions.base_branch` — the session
  preference. Both `TEXT DEFAULT ''`; empty means "not stated", not
  "repository-less".
- `task_environment_repos` — the environment inventory the guard requires.
- `LaunchAgentRequest.RepositoryID` / `.Repositories` — the launch-side
  projection consumed by `environmentReposForLaunch`.

**Ordering.** `ListTaskRepositories` orders `position ASC, created_at ASC`,
which is not total: two rows with equal `position` written in the same
timestamp tick have no defined order, so "the primary" is undefined for them.
The order must be made total by appending `id ASC` as a final tiebreak, and
the launch must select through the same definition
`GetPrimaryTaskRepository` uses rather than introducing a second one.

## Control flow

1. A launch of an existing session enters through `LaunchSession` with
   `IntentResume`. Workflow auto-start, a Backlog bounce, and
   `RecoverSession` (`session_launch.go:419`) with any of `resume`,
   `resume_new_branch`, `fresh_start`, or `runtime_retry` all converge here;
   `fresh_start` differs only in clearing the resume token first.
2. `validateAndLockResume` takes the per-session lock and loads the task
   (without its attachments).
3. `buildResumeRequestAtCredentialBoundaryWithOptions` resolves the attachment
   set once and passes it into `applyResumeRepoConfig`
   (`executor_resume.go:1266`).
4. `applyResumeRepoConfig` selects the primary — from the session preference
   when set, otherwise from the resolved set — resolves the branch, populates
   the single-repository fields **including the primary's identity, which must be
   stamped on every executor type and not only the worktree one, and whether or
   not that repository has a local clone**, and populates
   `req.Repositories` when the set holds more than one attachment. The "when set"
   arm is unchanged and still fires for a preference that names a repository
   absent from the set; per the scope boundary above that arm is out of scope,
   not a target.
5. The launch runs; `persistTaskEnvironment` projects the request into
   inventory rows and writes the environment, where the guard admits it.

The single-repository and multi-repository cases differ only in step 4's last
clause: `environmentReposForLaunch` produces one row from the single-repository
fields, or one row per entry of `req.Repositories`. Both satisfy the guard —
but the single-repository branch does so only when step 4 stamped the primary's
identity, which is why that stamp is a named responsibility above rather than an
implementation detail. A launch that resolves the right primary and does not
stamp it produces no rows at all and is refused exactly as it is today.

## Branch resolution

`resolveResumeBaseBranch` (`executor_resume.go:1671`) already implements the
required precedence and is unchanged; it simply becomes reachable with a
resolved primary. It matches the primary against the resolved set: an
unambiguous match's non-empty base branch wins over the session's stored
value, so a base-branch repair on the task takes effect on the next launch;
an ambiguous match (the same repository attached twice) falls back to the
session value, because `task_sessions` predates exact row identity and cannot
name which of the two rows it meant. So does an *unambiguous* match whose base
branch is empty: `resolveResumeBaseBranch` returns the session's stored value in
all three of its non-winning cases — no match, an ambiguous match, and a single
match whose branch is empty. **The precedence is therefore four rungs, not
three:** the attachment's branch, then the repository's default branch, then the
session's stored branch, then — on the worktree executor alone — the built-in
constant. `AC-...-002.2` picks that chain up at the second rung and runs it to
the fourth, under `AC-...-002.1`'s antecedent, in which the session carries no
branch so the third is empty by construction; the requirement's `## Out of scope`
bullet on the terminal default states the same chain with the session rung
present.

**Where `AC-...-002.4` sits on that chain.** `AC-...-002.4` takes the
complementary antecedent — the session's stored branch is NON-empty — and its
phrase "that attachment's base branch" denotes the value already resolved through
the SECOND rung: `repoInfo.BaseBranch` after `resolveTaskRepoInfoForSession` has
defaulted an empty attachment branch to the repository's default branch. It is not
the raw `task_repositories.base_branch` column. That resolved value is exactly what
`resolveResumeBaseBranch` receives, so the AC's "when the attachment's is
non-empty" tests the second rung's output, and its "otherwise" arm — keep the
session's stored value — is reached only when the attachment's branch and the
repository's default branch are BOTH empty. Worked case: a session branch of
`main`, one attachment whose raw `base_branch` is empty, and a repository default
branch of `develop` resolve to `develop`, because the repository default outranks
the session's stored value exactly as the four-rung order above states.
`AC-...-002.2` uses the same phrase for the RAW column instead, because its
antecedent sits at the first rung's input rather than the second's output. The two
criteria describe one chain from two different points on it; neither introduces a
second ordering.

For the population this design repairs the session's
stored branch is itself empty, so the primary's base branch is left unset and only
the per-attachment specifications carry branches; `AC-...-002.1` states that
outcome rather than leaving a test author to discover it. `resolveTaskRepoInfoForSession` has
already defaulted an empty attachment branch to the repository's default branch
before this point, and `backfillRepoDefaultBranch` fills that default from the
local clone when the repository row itself lacks one. When all three are still
empty the single-repo worktree path falls back to the `defaultBaseBranch` constant
(`executor.go:165`, currently `"main"`), applied at `executor_resume.go:1764`
inside `applyResumeWorktreeConfig`. That is the only site applying it on this
path, and it is reached only when `shouldUseWorktree(req.ExecutorType)`
(`executor_execute.go:660`) holds — true for `ExecutorTypeWorktree` alone, not
for `ssh`, `local_docker`, `remote_docker`, `sprites`, `k8s` or `local`
(`internal/task/models/models.go:1962-1969`). The per-attachment specifications
built by `buildRepoSpecs` receive no such fallback either. The terminal default
is therefore real but narrow, and `AC-...-002.2` is scoped to exactly that
worktree path: on other executor types a launch whose attachment branch,
repository default branch and session branch are all empty proceeds with the
primary's base branch unset. Both the fallback and its narrowness are existing
behavior, unchanged by this design; they are stated here because `AC-...-002.2`
now draws a boundary through them, and the requirement's `## Out of scope`
records the missing equivalent elsewhere as a named pre-existing gap.

## Failure and recovery

- **Attachment read failure.** The read that resolves the launch's repositories
  fails the launch and surfaces the error; that read is what `AC-...-001.6` is
  scoped to. It fails closed in the same DIRECTION as `taskIsRepoBacked`
  (`executor_execute.go:2571`) — neither treats a read error as "no
  repositories", which is what would produce the ready-with-empty-inventory state
  the guards exist to prevent — but not by the same mechanism, and the AC does not
  reach it. `taskIsRepoBacked` logs the error and returns `true` (`:2578-2579`),
  so it cannot surface it to its caller: a failure of the guards' own read is
  either absorbed, when this launch wrote inventory, or reported as the
  missing-inventory message at `:2496` rather than as the read error. That is the
  guards' behaviour, unchanged here, and the requirement's `## Out of scope` keeps
  the guards themselves outside this change.
- **Attachment set changes mid-launch.** Assume nothing serializes an
  attachment-set write against a launch. `workspaceSourcesIdle`
  (`service_workspace_sources.go:694`) has exactly one caller,
  `AttachWorkspaceSources` (`:106`). It rejects an attach while any session has an
  active turn, but it does not serialize against a launch that has not yet
  produced a turn — and, decisive here, that operation refuses a task with no
  attachments (`:114`), so it cannot perform the empty-to-non-empty crossing at
  all. The only writer that can is the generic task update through
  `replaceTaskRepositories` (`service_tasks.go:1843`), which takes no idleness
  check. So on the path that produces `AC-...-001.11`'s scenario there is no
  synchronization to rely on, and a race test must create the window itself rather
  than assume the idle check narrows it. What the guards do in that window depends
  entirely on which transition occurred, and only one of the two is covered.

  **Empty to non-empty is covered.** The launch resolved an empty set and so
  writes no inventory rows, and it is refused before any environment reaches
  ready. Which check refuses it depends on the branch, and the two are not the
  same mechanism. Creating a fresh environment, the guard re-reads the table
  inside the same transaction as the INSERT and refuses ahead of it (`:59`), so
  nothing partial lands. Updating an environment that already exists, the first
  refusal comes earlier and outside any transaction, from `taskIsRepoBacked`
  (`executor_execute.go:2571`) reading the attachment set afresh before the
  transition is attempted (`executor_execute.go:2485`). It restores the previous
  status and fails with its own distinct message, "persist task environment: ready
  status requires repository inventory" (`:2496`), and it deliberately stands
  down while the environment is still `CREATING`, leaving the initial
  materializer responsible for publishing or failing the inventory; the
  transactional `:265` check is the backstop behind both. Both branches fail
  closed, and neither brings an environment to ready nor writes an inventory row,
  which is what `AC-...-001.11` asserts. A ready
  environment that already existed is left untouched — a task repository-less at
  session creation may own one carrying an empty inventory — and it does not block
  the next launch, which resolves the new set and succeeds.

  **Non-empty to a different non-empty is NOT covered, and this design does not
  close it.** An earlier draft claimed the same guard handled the general case.
  It does not: every `taskHasRepositoriesTx` call site tests only whether the
  task has *any* attachments, and compares that against an inventory that is
  *empty*. A launch that resolved a one-attachment set while a second attachment
  was being added writes one row, `len(env.Repos) == 0` is false at every site,
  no branch fires, and the environment becomes ready holding a stale partial
  inventory — the state these guards exist to prevent, reached by a path they
  cannot see. Closing it means re-reading the attachment set inside the
  transaction that writes the inventory and failing on mismatch, which is new
  work at a persistence boundary this design otherwise leaves untouched. It is
  recorded as an accepted residual in the requirement's `## Out of scope` rather
  than claimed as handled here.
- **Concurrent launches of one session.** Serialized by the per-session lock
  taken in `validateAndLockResume` (`executor_resume.go:1033`). The lock is
  blocking, so the second launch is not refused on arrival: it waits for the
  first to release, then re-reads the session row inside the lock and is rejected
  there, before it can resolve repositories or write an environment. Two distinct
  rejections, both reachable: `SessionStateSupersededError` when the re-read shows
  a terminal state different from the one this launch requested
  (`executor_resume.go:1092`), and `ErrExecutionAlreadyRunning`
  (`executor.go:170`) when the session is non-terminal and an execution is already
  registered for it (`executor_resume.go:1106`). Neither fires in a third case,
  which `AC-...-001.12` now names rather than leaving to be discovered: if the
  first launch failed and `rollbackResumeStateAfterFailure` restored the session
  to the same state the second launch requested, that state is terminal but not
  *different*, so the superseded branch does not fire; and being terminal it
  skips the already-running check entirely (`:1103-1107`). The second launch then
  proceeds and resolves repositories by the rules above, which is correct — no
  execution is running and it is a retry, not a duplicate. The in-lock re-read is
  what makes all three outcomes safe: the caller fetched the session before taking
  the lock, so its copy of the state may already be stale. The lock is held for
  the whole of `ResumeSession` (`defer unlock()`, `executor_resume.go:782`), not
  merely for the re-read, so a second launch cannot observe a half-built
  environment. This is `AC-...-001.12`; the mechanism is unchanged by this design
  and is documented here only because the AC asserts it.
- **Repository row missing.** Unchanged: `GetRepository` fails and the launch
  fails, whether the id came from the session or the attachment set.

## Persistence

No schema change beyond the `ORDER BY` tiebreak, which affects a read only. No
migration and no backfill: the sessions currently in the broken state are
repaired by the next launch resolving correctly. Restart behavior is
unchanged, and startup recovery reaches this path through the same resume
entry point as any other launch.

## Security

None affected. Repository resolution stays inside the task's own attachment
set and its workspace; no authorization boundary moves. Git credential
issuance continues to key off the resolved repositories, so a launch that
previously carried none and now carries the task's attachments requests
exactly the credentials that task's repositories already require.

## Observability

The failure is currently visible only as the guard's error text. Resolution
that falls back to the attachment set because the session carried no
preference should be logged at info with the task id, session id, and the
resolved repository id, so the repaired-session case is distinguishable in
logs from a session that always had a preference. No new metric.

## Related decisions

None. This design records a defect correction inside an existing contract
([Attach Workspace Sources](attach-workspace-sources.md)) and does not
establish a new boundary.
