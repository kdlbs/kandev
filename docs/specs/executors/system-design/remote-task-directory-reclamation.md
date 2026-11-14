---
status: current
system: executors
updated: 2026-11-14
requirements:
  - REQ-SSH-TASKDIR-RECLAMATION-001
  - REQ-SSH-TASKDIR-RECLAMATION-002
  - REQ-SSH-TASKDIR-RECLAMATION-003
  - REQ-SSH-TASKDIR-RECLAMATION-004
  - REQ-SSH-TASKDIR-RECLAMATION-005
  - REQ-SSH-TASKDIR-RECLAMATION-006
  - REQ-SSH-TASKDIR-RECLAMATION-007
---

# Remote Task-Directory Reclamation System Design

## Purpose and boundaries

Reclamation is a task-scoped step inside the existing durable task-resource
cleanup job, not a new subsystem and not a new background sweeper. The job
already owns the terminal-outcome trigger, the ordering against runtime stops,
the retry-with-backoff schedule, the cancellation path when an archive is undone,
and the durable `task_resource_cleanup_jobs` row that surfaces failure. This
design adds one phase to that job and one connection-level helper to the SSH
executor package.

The boundary that matters most is *where the removal does not go*.
`SSHExecutor.StopInstance` is the obvious place and is the wrong one, for three
independent reasons:

1. **Scope mismatch.** `StopInstance` is per session; a task directory is per
   task. A three-session task would evaluate removal three times, and the first
   session's removal would delete the directory out from under the other two
   while they are still stopping.
2. **Error laundering.** `stopTaskRuntimeTargets` deliberately swallows stop
   errors for targets already marked terminal
   (`if target.terminal { ...; continue }`) and for `runtimeStopAlreadyComplete`.
   A removal failure raised from `StopInstance` would be discarded on exactly the
   paths this feature runs on, producing the false-success shape that
   REQ-SSH-TASKDIR-RECLAMATION-005 exists to prevent.
3. **Missing knowledge.** Ownership (REQ-SSH-TASKDIR-RECLAMATION-003) is a
   question about tasks, environments, and sibling rows. The executor has an
   instance; the task service has the graph.

`StopInstance` therefore keeps its current contract unchanged: it never removes
the task directory. That is what makes AC-SSH-TASKDIR-RECLAMATION-004.1 and
AC-SSH-TASKDIR-RECLAMATION-004.2 structural rather than conditional — an ordinary
stop and a backend shutdown never create a cleanup job, so there is no code path
from them to a removal.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-SSH-TASKDIR-RECLAMATION-001` | [Control flow](#control-flow) |
| `REQ-SSH-TASKDIR-RECLAMATION-002` | [Safety probe](#safety-probe) |
| `REQ-SSH-TASKDIR-RECLAMATION-003` | [Ownership resolution](#ownership-resolution) |
| `REQ-SSH-TASKDIR-RECLAMATION-004` | [Purpose and boundaries](#purpose-and-boundaries) |
| `REQ-SSH-TASKDIR-RECLAMATION-005` | [Failure and recovery](#failure-and-recovery) |
| `REQ-SSH-TASKDIR-RECLAMATION-006` | [Data and contracts](#data-and-contracts) |
| `REQ-SSH-TASKDIR-RECLAMATION-007` | [Control flow](#control-flow) |

## Components and responsibilities

- **`lifecycle` package (`apps/backend/internal/agent/runtime/lifecycle`).** Owns
  a new `SSHTaskDirReclaimer` that, given a connection descriptor and a target
  directory, opens its own SSH connection, runs the read-only safety probes, and
  performs the guarded removal. It reuses `ResolveSSHTarget`, the existing
  fingerprint pinning, `runSSHCommand`, and `shellQuote`. It does not touch
  `SSHExecutor` session state, because by the time it runs every session has been
  stopped and its connection closed.
- **`task/service` package.** Owns the new cleanup-job phase: capturing the
  reclamation targets into the job snapshot at prepare time, resolving ownership,
  invoking the reclaimer, and folding the outcome into the job's error set.
- **`orchestrator/executor` package.** Owns projecting the new per-profile
  setting into launch metadata as an authoritative key.
- **Web settings.** Owns the per-profile toggle and its blast-radius copy.

## Data and contracts

### Per-profile setting

A new executor-profile config key `ssh_reclaim_task_dir` (string `"true"` to
enable; anything else, including absent and empty, disables). It is added to
`profileConfigAuthoritativeKeys` in
`apps/backend/internal/orchestrator/executor/executor_state.go`, not to
`profileConfigPassthroughKeys`. That distinction is the mechanism behind
AC-SSH-TASKDIR-RECLAMATION-006.3: authoritative keys are written unconditionally
from the profile, so a task that supplies `ssh_reclaim_task_dir` in its own
metadata is overwritten rather than honoured. This matches the existing treatment
of `ssh_workdir_root` and `ssh_shell`, which are authoritative for the same
reason — they are redirect vectors.

A matching `MetadataKeySSHReclaimTaskDir` is added to `persistentMetadataKeys` so
the value survives into `ExecutorRunning.Metadata` and is readable by the cleanup
job after a backend restart, alongside the connection tuple that
`ResumeRemoteInstance` already relies on.

Default disabled satisfies AC-SSH-TASKDIR-RECLAMATION-006.1 with no migration:
existing profile rows have no such key, and an absent key is disabled.

### Reclamation target

Captured into `taskResourceCleanupSnapshot` as a new field so the job is
self-contained and survives a restart:

| Field | Meaning |
| --- | --- |
| `host`, `port`, `user` | Connection identity |
| `host_fingerprint` | Pinned fingerprint; a mismatch fails the attempt |
| `identity_source`, `identity_file`, `proxy_jump` | Auth and routing |
| `workdir_root` | Profile workspace root |
| `remote_task_dir` | Absolute remote path recorded at launch |
| `shell` | Login shell used for remote commands |

Targets are gathered from `ListExecutorsRunningByTaskID` at prepare time and
de-duplicated by `(host, port, user, remote_task_dir)`, so a multi-session task
produces one target, and a task spanning two hosts produces two.

## Control flow

Inside `executeTaskResourceCleanupJob`, after the existing stop phase and the
existing `performTaskCleanup`, and only when `len(failedStops) == 0` and no
context cancellation is pending:

1. **Gate on the setting.** Skip targets whose snapshot does not carry
   `ssh_reclaim_task_dir == "true"`.
2. **Resolve ownership** (see below). A non-owned or unresolvable target is
   skipped and recorded, not failed.
3. **Validate the path shape.** The target must equal
   `<workdir_root>/tasks/<segment>` after normalization, with `<segment>`
   non-empty and containing no `/`, no `.`, and no `..`. Anything else is
   refused. This is a belt-and-braces guard for
   AC-SSH-TASKDIR-RECLAMATION-003.4; it makes an empty or corrupted task-dir name
   incapable of resolving to `<workdir_root>/tasks/` itself.

   The comparison is made against the *expanded* root. `ensureRemoteTaskDir`
   builds the recorded directory from `expandRemoteHome(workdir_root)`, while
   the profile stores the unexpanded form and stores nothing when the field is
   left blank. A literal `~/.kandev` is therefore resolved against the remote
   `$HOME` first, and a blank root falls back to the executor's own default, so
   the guard bounds the path instead of refusing every default-configured host.
   A root that needs no expansion issues no remote command, which is what keeps
   a refused path from touching the host at all.
4. **Connect** using the snapshot tuple and pinned fingerprint. An empty pinned
   fingerprint is an error: a launch cannot happen without a trusted host key,
   so its absence means the recorded connection is not one Kandev established
   and dialling would accept whatever key answered.
5. **Probe** (see below). Any at-risk or inconclusive verdict skips and records.
6. **Remove** using the checkout classification from the safety probe. Remove
   linked children first. Git removes a linked root; the existing recursive
   command removes a standalone root. Keep the path-absence check. Git owns
   registration removal, which regression tests verify. A non-zero removal exit
   or failed path-absence check is an error.

A root with non-ignored untracked child directories fails the dirty check in
step 5. It is preserved before any child removal. The probe does not remove
children first to make the parent look clean.

The `cleanup_script` ordering required by REQ-SSH-TASKDIR-RECLAMATION-007 is
satisfied by this position rather than by new sequencing code: the script runs
inside `SSHExecutor.StopInstance` during step 0's stop phase, which the job
completes and checks before reaching step 1. AC-SSH-TASKDIR-RECLAMATION-007.2
follows because the probe in step 5 reads the tree the script left behind.

### Ownership resolution

A target is owned by the task when both hold:

- The task's `TaskEnvironment.TaskID` is the job's task, and
  `hasActiveOtherTaskSessionsForEnvironment` reports no other live task on that
  environment. This is the existing signal that distinguishes a task from an
  `inherit_parent` subtask borrowing the parent's workspace, and it is already
  consulted for the local worktree path.
- No `executor_running` row belonging to another task carries the same
  `(host, port, user, remote_task_dir)`.

The second check is the one that catches the cross-host case the environment
check cannot see, and it is a direct analogue of
`CountActiveWorktreeReferences`. Either check erroring resolves to *not owned*
per AC-SSH-TASKDIR-RECLAMATION-003.3.

### Safety probe

Run against the task-directory root and every direct child directory that
contains a `.git` entry, matching the existing remote layout. Every discovered
checkout must pass before any removal begins. One unsafe or unknown checkout
vetoes removal of the entire task directory.

Keep the correction in the existing reclaimer, using small command and parsing
helpers. Obtain the checkout root, Git directory, and common Git directory with:
`git -C <checkout> rev-parse --path-format=absolute --show-toplevel --git-dir --git-common-dir`.
Require exactly three absolute path results. The option requires Git 2.31 or
later. Older Git, unsupported options, malformed output, or failed commands
produce `probe_failed`; there is no legacy-option fallback.

Resolve the profile root, task directory, and returned directory paths on the
remote host with `cd <quoted-path> && pwd -P`. Compare physical paths at segment
boundaries. The physical task path must remain exactly one segment beneath
the physical profile root's `tasks/` directory. Its checkout root must match
that task path. A symlinked profile root is valid when these checks pass.
For child checkouts, require the physical checkout root to match the physical
child path inside the task. Common storage is external only when it lies
outside the physical task directory. Unresolvable or escaping paths are
`probe_failed`. These probes follow the existing lexical path guard.

There are two supported cases:

| Checkout shape | Commit and stash scope | Removal |
| --- | --- | --- |
| Linked worktree with distinct Git/common directories and common storage outside the task directory | This worktree's `HEAD` and recorded stash branch | `git worktree remove` |
| Standalone clone with its own `.git` directory and no additional registered worktrees | Every local branch, `HEAD`, and every stash | Existing recursive removal |

A `.git` file alone does not establish a linked worktree. Unknown or unsupported
layouts produce `probe_failed`. Linked worktrees with common storage inside the
task directory remain unsupported. Read `worktree list --porcelain -z` for both
checkout kinds. Match the current checkout's physical path to its registration.
A missing or ambiguous match produces `probe_failed`. A `locked` attribute,
with or without a reason, produces `shared` with a lock-specific `Detail`.
This is a safety skip before mutation. A standalone clone with any additional
registered worktree also produces `shared`, even for a worktree inside the task.
This guard avoids a general repository-dependency traversal.

Run the read-only `git submodule status --recursive` probe for every checkout.
Any non-empty result identifies an unsupported submodule layout and produces
`probe_failed` with a submodule-specific `Detail`. Preserve both initialized and
uninitialized submodules. A command error also preserves the directory. This
policy avoids forced removal or additional submodule-content checks.

The read-only content probes follow that classification:

| Probe | Command | Safe when |
| --- | --- | --- |
| Is a checkout | `git rev-parse --is-inside-work-tree` | prints `true` |
| Clean tree | `git status --porcelain --untracked-files=all --ignored=no` | empty output |
| Linked worktree published | `git rev-list --count HEAD --not --remotes` | prints a non-negative integer equal to `0` |
| Repository published before its storage is removed | `git rev-list --count HEAD --branches --not --remotes` | prints a non-negative integer equal to `0` |
| Stash scope | `git stash list --format=%gs` | no blocking or unknown stash |

Run probes with `GIT_OPTIONAL_LOCKS=0` and deterministic output options. A count
must contain exactly one non-negative integer. Empty, negative, or malformed
output produces `probe_failed`. Both commit commands include `HEAD`, which also
protects detached unpublished commits.

The status command does not enumerate ignored files. Remove the special
`isKandevRuntimeStatus` exemption, since ignored `.kandev/` data already stays
outside this output. Every non-empty status result blocks removal. An untracked
`.kandev/` path still blocks removal when Git does not ignore it.

`--ignored=no` does not change submodule-ignore configuration. Disposable Git
fixtures reported the same dirty submodule in the old and new status forms.
Both forms hid that state with `submodule.<name>.ignore=all`. The independent
submodule probe preserves those projects regardless of their status settings.

For a retained repository, read the full symbolic branch name and distinguish a
verified detached state from a command failure. Parse stash descriptions from
`%gs`, independent of user display formats. Recognize `WIP on <branch>: ...` and
`On <branch>: ...` with an exact branch delimiter. A substring match is unsafe
for branches such as `task/a` and `task/a-extra`.

Any matching stash produces `stash_present`, even alongside unrelated stashes.
An unknown description or a detached checkout with any stash also produces
`stash_present`, with the attribution uncertainty in `Detail`. A command failure
produces `probe_failed`. An empty stash list permits a published detached
checkout. A repository whose storage is removed retains the existing rule that
every stash blocks reclamation.

The recorded branch name is a conservative attribution aid, not a durable branch
or worktree identity. Renamed branches retain their original stash descriptions.
Reclamation never removes stashes from the retained common repository.

Remote-tracking refs are a local cache and are not refreshed: the probe answers
"was this pushed at some point", which is the correct bar, and fetching during
cleanup would need network and credentials for a remote that may no longer exist.

Default SSH preparation initializes and fetches a standalone primary checkout
at the task root (`defaultSSHPrepareScript` in `default_scripts.go`). It can
reuse an existing checkout but does not create linked worktrees. Custom scripts
or previously prepared repositories supply that layout. `verifyPrimaryCheckout`
rejects a child-repositories-only layout when a primary repository URL exists.

The root remains mandatory in the cleanup probe. A legacy or custom task root
without a checkout is preserved, even when its children contain repositories.
A false checkout result gives `not_a_checkout`; a failed command gives
`probe_failed`. Such layouts are not a supported normal SSH materialization path,
and enabling their reclamation is outside this correction. They are one possible
probe-failure cause, not an explanation established for the issue's 40 events.
Timeouts and unparseable output also resolve to safety skips.

Probes run under a bounded per-target timeout derived from the job's context.

### Registration-aware removal

`SSHTaskDirReclaimer.Reclaim` retains its public return contract. Carry only the
checkout path, checkout kind, and common-directory path from probing to removal.
The existing task lifecycle, path guard, timeout, and cancellation checks remain.

For a linked worktree, use the verified common directory as the Git context:
`git --git-dir=<quoted-common-dir> worktree remove -- <quoted-checkout>`.
Use no force option. Git 2.47.3 removes an ignored-only worktree without force
and refuses locked worktrees or initialized submodules. Known locks and all
registered submodules are skipped during probing. Git's refusal remains the
backstop if a lock or submodule appears after probing. That unexpected removal
failure enters the existing error path; a retry runs the probes again.
Git's clean-tree check also protects against new tracked or untracked changes.

The exact `git --git-dir=<common-dir> worktree remove -- <checkout>` form was
reproduced with a non-bare shared repository and an intact main worktree.
It ran through the login-shell wrapper from a sibling worktree's directory.
Only the explicit target disappeared; the main worktree, sibling, HEAD, and
shared refs remained. This command uses the explicit common directory and
absolute target, rather than relying on the caller's current working directory.

Remove linked child checkouts before their task root. For a standalone root,
use the existing recursive command after those Git removals succeed. For a
linked root, Git removes the root itself. A Git error stops the operation and
never triggers a recursive-removal fallback.

Git owns removal of its registration. Keep the existing path-absence check after
removal. Real-Git tests assert that the registration also disappears, so production
needs no second inventory verification or captured-identity revalidation pass.
No force option, broad prune, topology framework, or recovery protocol is added.
Retries use the existing cleanup job and run the safety probes again.

## Failure and recovery

The distinction that carries REQ-SSH-TASKDIR-RECLAMATION-005 is **skip versus
error**, and the two are deliberately not the same outcome:

- A **skip** — reclamation disabled, directory shared, work at risk, verdict
  inconclusive — is a correct result. It appends no error, the job completes
  `succeeded`, and the reason is logged and recorded. Retrying would not change
  the verdict, and a permanently-failing job for a directory the user is keeping
  on purpose is noise (AC-SSH-TASKDIR-RECLAMATION-002.7).
  Known worktree locks and unsupported submodule layouts also take this path.
- An **error** includes connection failures, authentication failures, fingerprint
  mismatches, failed removals, and unverifiable removal postconditions. It appends
  to the job's `errs` slice, like other cleanup failures. The existing
  `retryTaskResourceCleanupJob` applies the 1m/5m/15m/1h/3h/6h/12h backoff and
  writes `last_error`. It marks the row `failed` after
  `taskResourceCleanupMaxAttempts`.

Because the job re-decodes its snapshot and re-runs the phase on every attempt,
AC-SSH-TASKDIR-RECLAMATION-005.3 holds without extra work: no verdict is cached
across attempts.

Reclamation runs after `performTaskCleanup`, so a failure cannot prevent local
row, worktree, and quick-chat cleanup from completing
(AC-SSH-TASKDIR-RECLAMATION-005.4).

`cancelIfTaskUnarchived` already runs before and within the job body and cancels
the whole job when an archive is undone, giving
AC-SSH-TASKDIR-RECLAMATION-004.3.

Backend shutdown cancels the cleanup worker's context mid-job. The phase refuses
to begin on a cancelled context, so an irreversible removal is never started
against a process that is going away; the durable job row survives and the next
attempt runs the phase with a live context. That check lives inside the phase
rather than at its call site so a later reordering of the job body cannot
quietly remove it.

## Persistence

No new tables and no migration. The reclamation targets live inside the existing
`task_resource_cleanup_jobs.resource_snapshot` JSON blob, which is already the
mechanism by which the job survives a backend restart. The new snapshot field is
additive and absent-tolerant: a job row written by an older backend decodes with
an empty target list and performs no reclamation.

The per-profile setting persists in the executor profile's existing `Config` map;
no schema change.

Checkout classification remains in memory for one attempt. The launch metadata
and durable target JSON shape stay compatible.

## Security

The removal is the most destructive operation Kandev performs on a machine it
does not own, so the design keeps three independent barriers between a bug and a
user's disk: the profile-authoritative opt-in, the ownership resolution, and the
path-shape guard. Each is sufficient on its own to prevent removal.

Connections reuse the pinned `host_fingerprint` from the snapshot with the same
no-silent-re-pin rule as every other SSH path; a changed host key fails the
attempt rather than proceeding against an unknown machine. The remote path is
shell-quoted through the existing `shellQuote` helper and validated in Go before
it is quoted, so a task-dir name can neither escape its parent nor inject shell.

## Observability

Structured logs at the reclamation phase carry `task_id`, `host`, `remote_task_dir`,
and an `outcome` of `removed`, `skipped`, or `failed`, with a `reason` on the
latter two. `skipped` reasons are enumerated (`disabled`, `shared`,
`dirty_worktree`, `unpushed_commits`, `stash_present`, `not_a_checkout`,
`probe_failed`) so an operator can tell a deliberate preservation from a broken
probe.

The existing `task_resource_cleanup_jobs` row remains the durable record of a
failure, via its `state`, `attempts`, and `last_error` columns. The resource
snapshot in that row also records each target outcome and safety-skip reason,
so a successful preservation is discoverable after restart.

## Testing

Every test for the removal path uses a fake remote-command seam or a temporary
directory. No test executes a real deletion against a real workspace root. The
`SSHTaskDirReclaimer` takes its command execution through an interface so unit
tests never open an SSH connection, and the probe-verdict table is exercised
purely against recorded command outputs.

Add real-Git coverage through a test-only local implementation of
`sshRemoteRunner`. It runs the production `Reclaim` commands against repositories
under `t.TempDir()`. A local bare remote supplies real remote-tracking refs.
No fixture opens SSH or uses a developer repository.

The runner executes `WrapLoginShell("bash", cmd)` through an outer shell, matching
`sshClientRunner.Run`. Preserve separate stdout, stderr, and exit status.
Use isolated Git configuration and include task paths with spaces and apostrophes.
The direct home probe remains unwrapped, matching `RunDirect`.

Cover ignored files, own and unrelated branches/stashes, detached heads, and
standalone repository protection. Also cover unsafe children, shared-storage
preservation, known-lock and submodule safety skips, child removal order, and
command failures. Include a symlinked profile root, unsupported path-format output,
a pushed shallow standalone clone, and a child-repositories-only root.
Assert the target disappears, its registration disappears, and distinct sibling
sentinels and shared refs remain. Cancellation before mutation must issue no
removal command. This is filesystem end-to-end evidence, without a browser flow.

Regression coverage must include the directory being removed on terminal archive
and delete, and preserved on ordinary stop and on backend shutdown.

## Related decisions

- [Original opt-in and cleanup-job placement](../../../decisions/2026-08-24-opt-in-ssh-task-directory-reclamation.md).
- [Scope safety checks to the storage being removed](../../../decisions/2026-11-12-scope-ssh-reclamation-to-deleted-storage.md).

## Implementation plans

- [Worktree-aware correction](../../../plans/ssh-worktree-task-dir-reclamation/plan.md).
- [Original completed package](../../../plans/ssh-remote-task-dir-reclamation/plan.md).
