---
status: current
system: agents
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003
---

# Agent resume and runtime recovery system design

## Purpose and boundaries

This design preserves a provider conversation when session launch fails. It
also defines visible recovery errors and explicit continuation after confirmed
Git branch loss.

The orchestrator owns recovery authorization, session identity, and warning
persistence. The agent lifecycle manager owns balanced preparation progress and
completion publication. The worktree manager owns branch verification and fresh
branch creation. The web client owns error presentation and recovery choices.

The task environment remains the owner of worktrees. A task session refers to
that environment and does not acquire its own worktree lifecycle.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001` | [Session identity](#session-identity), [Resume preparation lifecycle](#resume-preparation-lifecycle) |
| `REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002` | [Visible recovery errors](#visible-recovery-errors) |
| `REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003` | [Explicit branch replacement](#explicit-branch-replacement), [Warning persistence](#warning-persistence) |

## Current failure path

`Service.ResumeTaskSession` calls `executor.Executor.ResumeSession`. The
executor builds the existing resume request and asks the lifecycle manager to
prepare the task workspace. Worktree preparation calls
`worktree.Manager.Create`, which can recreate an invalid worktree.

`Manager.recreate` returns a wrapped `worktree.ErrBranchUnrecoverable` only
after it cannot find the branch locally and receives the distinct
`worktree.ErrRemoteRefMissing` sentinel from confirmed missing-ref evidence
at the configured remote. Authentication, network, timeout, and other fetch
failures remain their original failure class and cannot authorize replacement.
The wrapped error already reaches `Service.RecoverSession` and the
`session.recover` WebSocket handler.

Attach-only reuse has one additional evidence boundary. If the local branch and
`refs/remotes/origin/<branch>` are both absent, the manager runs a bounded,
noninteractive `git ls-remote` probe against the configured remote. Only a
confirmed missing remote ref produces `ErrBranchUnrecoverable`; an
authentication, network, timeout, or other probe failure remains an ordinary
reuse failure. A pruned tracking ref therefore cannot be mistaken for remote
branch deletion.

The web client currently converts WebSocket failures to a plain `Error` and
several resume call sites then discard that error. Automatic resume also hides
the resume error when read-only workspace restore succeeds.

## Resume preparation lifecycle

Native ACP resume normally reuses an already prepared workspace. A worktree
resume is different: `shouldPrepareEnvironment` can re-enter environment and
runtime preparation so Git recovery can validate, recreate, or replace the
task-owned worktree.

Preparation event publication follows one balanced lifecycle. If a launch path
can publish `executor.prepare.progress`, it publishes exactly one terminal
`executor.prepare.completed` event for that attempt after environment and
runtime preparation succeeds or fails. The presence of an ACP session ID can
skip both event kinds when no preparation runs; it cannot suppress only the
terminal event after a worktree resume has emitted progress.

The web preparation handler projects progress as `preparing` and the terminal
event as `completed` or `failed`. `useSessionState` can treat live preparation
as working while the durable session is temporarily `STARTING` or
`WAITING_FOR_INPUT`, but that derived state ends with the terminal event. A
resumed session that settles at `WAITING_FOR_INPUT` without foreground or
detached activity therefore renders as idle and keeps the composer available.

Session snapshots remain recovery evidence, not a substitute for balancing the
live stream. They can hydrate a missing preparation projection, while a live
in-flight projection remains authoritative until its matching terminal event.

## Recovery protocol

`session.recover` accepts these actions:

- `resume` retries the current worktree and provider conversation.
- `fresh_start` keeps its current behavior and clears the provider resume
  identity before a new conversation starts.
- `runtime_retry` keeps its current managed-runtime behavior.
- `resume_new_branch` resumes the current provider conversation and permits
  replacement of only a confirmed unrecoverable worktree branch.

`resume_new_branch` is explicit. A failed `resume` never changes the branch.
The handler returns a typed WebSocket conflict when the error chain matches
`worktree.ErrBranchUnrecoverable`. The error details contain:

```text
kind: branch_unrecoverable
recovery_action: resume_new_branch
original_branch: <branch>
base_branch: <configured task base>
repository_id: <workspace repository>
session_id: <task session>
```

The public error message remains descriptive for clients that do not inspect
details. Other failures retain their existing error codes and do not advertise
branch replacement.

The web WebSocket client retains `code`, `message`, and `details` in a typed
request error. Existing consumers can still treat it as an `Error`.

## Session identity

`resume_new_branch` uses the existing `TaskSession`. It does not clear
`task_sessions.metadata.acp.session_id` or `executors_running.resume_token`.
The executor passes the same provider resume identity to agent launch after
workspace preparation succeeds.

The existing guarded transition to `STARTING`, resume lock, failure rollback,
and successful token replacement rules remain unchanged. Only **Start fresh**
can intentionally remove the stored provider identity before launch.

## Explicit branch replacement

The resume request carries an internal permission to replace a confirmed
unrecoverable branch. The permission flows through executor workspace
preparation into `worktree.CreateRequest`. Normal resume leaves it disabled.

`Manager.recreate` first runs the normal local and remote recovery checks. If
those checks return `ErrBranchUnrecoverable` and replacement permission is
disabled, it returns the error without changing branch identity. If permission
is enabled, the manager performs these steps:

1. Resolve the task repository's configured base branch through the existing
   base-ref selection and required refresh rules.
2. Keep the existing task directory and repository association.
3. Generate a new worktree directory and Git branch with the existing branch
   template, `TaskBranchNameWithSuffix`, and random suffix helpers.
4. Create the worktree from the resolved base branch.
5. Update the existing task environment repository record with the new path,
   branch, and ready state.
6. Continue normal agent launch with the original session resume identity.

The update is compensating. If the existing task-environment repository record
cannot be persisted after the new checkout is created, the manager removes the
new checkout and deletes its newly created branch with an exact-tip check. The
previous record remains authoritative, so a retry does not accumulate orphan
checkouts or branches.

This path does not copy commits, uncommitted files, or Git state from the lost
branch. It does not reuse the lost branch name. Pull-request and contribution
refs from the lost branch do not become the starting ref for the replacement.

For a task with multiple repositories, each `Create` call evaluates its own
worktree. A valid worktree is reused. A confirmed unrecoverable worktree is
replaced only when the explicit permission is present. Any unrelated failure
stops launch through the existing atomic preparation boundary.

## Warning persistence

The orchestrator captures repository branch state before explicit recovery and
loads it again after workspace preparation completes. Each confirmed branch
replacement produces one `status` message through `CreateSessionMessage`, even
when a later repository preparation failure aborts the overall resume. The
message metadata is:

```text
variant: warning
kind: branch_recreated
original_branch: <lost branch>
new_branch: <created branch>
base_branch: <configured task base>
session_id: <task session>
repository_id: <workspace repository>
decision_id: <stable replacement identity>
```

The content is a neutral internal status value. The frontend selects localized
copy from `metadata.kind` and the structured branch fields.

Before message creation, the orchestrator uses a state-guarded
`SetSessionMetadataKeyIfAbsentIfState` claim with a timestamp and a key derived
from the session, repository, original branch, new branch, and base branch.
This follows the model-selection warning claim pattern. A duplicate active
claim skips message creation. A stale timestamped claim is reclaimed only
when the session state still matches and no matching warning message exists,
so a process crash between claiming and message creation can be retried. A
failed message write releases the claim so a later retry can persist the
warning.

The comparison and warning attempt happen on every terminal path after resume
workspace preparation can materialize a replacement. Provider startup and
readiness failures therefore cannot bypass the warning, and a later retry does
not lose the old branch identity by taking the already-replaced branch as its
new baseline.

Direct orchestrator persistence is preferred over a runtime stream event. The
orchestrator makes the branch decision and already owns the old and new branch
state. A runtime event would add a second source without adding evidence.

## Visible recovery errors

The manual recovery controls use one shared request helper that rejects with
the typed WebSocket error. `SessionStoppedBanner` and `RunErrorEntry` retain the
last error in component state and render the existing destructive alert
pattern. The busy state ends after the request, but the error stays visible.

The alert shows the backend message and these applicable actions:

- **Retry resume** for a normal recoverable failure.
- **Restore read-only workspace** when workspace restoration remains useful.
- **Continue on a new branch** only for `branch_unrecoverable` details.
- **Start fresh** with the existing confirmation because it loses provider
  conversation history.

The shared Retry control is disabled while its corresponding recovery request
is pending. All three recovery surfaces use the same busy state, so repeated
clicks cannot overlap resume requests.

Automatic page-load resume can keep its current read-only fallback. If restore
succeeds, the hook returns a nonblocking notice with the resume cause and the
read-only state. If restore also fails, it returns both causes. Task, preview,
and Quick Chat consumers render this state instead of ignoring it.

The recovery result uses separate error and notice fields. Manual resume and
read-only restore retain separate causes, so a second failure does not hide the
first cause or its branch details. A later successful resume clears both. A
read-only restore clears the blocking error and keeps the informational notice
until the session state changes or the user dismisses it. The automatic
resumption hook also clears stale feedback when the shared live session state
transitions to `STARTING`, `RUNNING`, or `WAITING_FOR_INPUT` after an external
manual recovery.

An automatic status or recovery attempt is owned by the complete task-session
identity, not by the session ID alone. Task navigation can briefly present the
new route session while the task store still contains the previously active
task. Each callback therefore applies feedback and status only when both its
captured task ID and session ID still match the current request identity. The
identity also carries a monotonic generation that changes on every committed
navigation cycle, so returning to the same task-session pair still invalidates
callbacks from the earlier cycle. The frontend publishes the identity during
the commit phase, before passive effects can start or finish a request. A late
result from the prior task or navigation cycle cannot set an error, notice, or
status on the newly selected task.

## Frontend status rendering

`StatusMessage` maps `metadata.kind === "branch_recreated"` to localized
warning copy. The visible text names the original branch, new branch, and base
branch. It states both outcomes:

- The provider conversation history continues in the same task session.
- Code changes and commits that existed only on the lost branch were not
  recovered.

The status entry uses the same warning layout as model-selection warnings. It
does not depend on a live WebSocket event, so reload and replay show the same
message.

## Responsive and accessible behavior

Recovery remains inside the existing chat and stopped-session surfaces. This
change adds no navigation, modal, drawer, or mobile-only workflow.

On wide screens, related actions can share a row and wrap. On narrow screens,
actions use the existing stacked layout. Each action keeps a minimum 44-pixel
touch target. The alert stays in the chat scroll owner and does not create
horizontal page overflow. Error text and button labels remain available to
assistive technology and keyboard navigation.

## Internationalization

All new labels, error titles, read-only notices, confirmations, and branch
warning text use the `task` or `chat` namespace. Keys exist in `en`, `pseudo`,
`pt-pt`, `zh-cn`, `zh-hk`, and `zh-tw`. Branch names are interpolation values,
not translated strings. User-facing copy does not use a Unicode em dash.

## Testing strategy

Backend tests start with the current failure:

- An ACP worktree resume that publishes preparation progress also publishes one
  terminal completion event. An ACP resume that skips preparation publishes
  neither event.
- Normal resume returns an error that matches `ErrBranchUnrecoverable` and does
  not mutate the branch.
- Explicit replacement keeps the session and resume token, creates a unique
  branch from the configured base, and leaves valid repositories unchanged.
- `RecoverSession` validates `resume_new_branch` and passes replacement
  permission only for that action.
- Successful replacement persists complete warning metadata once. Replay and
  retry do not duplicate it, a failed write releases the claim, and a stale
  claim left by a crash can be reclaimed when no warning exists.
- Attach-only reuse probes the authoritative remote before classifying missing
  local and tracking refs as branch loss; transient probe failures do not
  authorize replacement.
- Replacement persistence failure removes the newly created checkout and branch
  while retaining the old task-environment repository record.
- Warning persistence is attempted after every terminal resume path that can
  follow materialization, including provider startup and readiness failure.
- A warning from an earlier repository replacement survives a later
  multi-repository preparation failure.
- The WebSocket handler maps the sentinel to a conflict with recovery details.

Frontend unit tests cover the typed WebSocket error, both manual recovery
surfaces, visible automatic fallback, dual failure detail, task navigation
with the same route session while the task identity changes, and
`branch_recreated` status rendering.

Desktop and mobile Playwright tests cover the user sequence from a failed
Resume request through explicit branch continuation and the persisted warning.
They also cover read-only fallback feedback, repeated attempts, touch target
size, keyboard reachability, and horizontal overflow.

## Related decisions

- [Keep Worktrees Owned by Task Environments](../../../decisions/2026-08-08-task-owned-worktree-lifetime.md)
- [Configurable worktree branch names](../../../decisions/0032-configurable-worktree-branch-names.md)
- [Require Explicit User Action Before Continuing a Session on a Replacement Branch](../../../decisions/2026-08-31-explicit-new-branch-session-recovery.md)
