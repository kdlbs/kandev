---
status: draft
system: ui
requirements:
  - REQ-UI-CI-PR-AUTOMATION-001
created: 2026-06-18
updated: 2026-09-06
owners:
  - tbd
---
# Task PR automation (part 2)

## Purpose and boundaries

This design preserves the technical source detail for `REQ-UI-CI-PR-AUTOMATION-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-CI-PR-AUTOMATION-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## State machine

Per-PR automation switches (state applies independently to each linked PR):

- `disabled`: all five automation switches are false for this PR. PR watch events update UI only.
- `auto_fix_enabled`: Kandev evaluates actionable PR feedback immediately when enabled, when CI automation options are saved while it remains enabled, and on later PR watch events.
- `auto_merge_enabled`: Kandev evaluates PR merge readiness immediately when enabled, when CI automation options are saved while it remains enabled, and on later PR watch events.
- `both_enabled`: Kandev evaluates both paths. Auto-fix does not merge; auto-merge merges only after readiness conditions are satisfied.
- `review_requested_prompt_enabled`: the first complete observation is a quiet
  baseline; later false-to-true transitions for `review_reviewer_login`
  prompt once, and a false observation rearms.
- `terminal_prompt_enabled`: merged or closed entry prompts once after the
  prompt is accepted or durably queued. A first complete observation already
  in the subscribed terminal state also prompts. Stable terminal state is
  quiet.
- Enabling a lifecycle option resets only the checkpoint needed to establish
  that option's documented baseline/entry semantics.

Lifecycle prompt cycle for one task/PR:

1. The existing PR watch poll synchronizes the linked PR and emits a lightweight
   lifecycle evaluation tick for tasks with lifecycle prompt options.
2. Review-request evaluation resolves the GitHub login connected to the task's
   workspace. When it differs from `review_reviewer_login`, Kandev atomically
   rebinds the login and resets the task's review-request baselines without
   notifying.
3. Kandev compares the current PR fact with the per-PR checkpoint.
4. A qualifying edge renders the immutable server-owned template using only the
   validated canonical PR URL and calls the shared task prompt dispatcher with a
   task/repository/PR/event coalesce key.
5. A current primary session in `IDLE` or `WAITING_FOR_INPUT` receives a visible
   automation-generated message immediately after its final guarded claim. A
   busy session receives a durable queued message. Identical task/PR/event
   observations coalesce without combining different events or PRs.
   The queue reserves a lifecycle head without deleting it and holds a
   per-session in-process dispatch guard. For a deferred passthrough delivery,
   it claims the row's in-flight token before that guard is released and the
   later dispatcher consumes this preclaimed token, preventing a concurrent
   drain from dispatching the same row. The row is acknowledged only after the
   PTY/executor accepts the prompt; ordinary queue entries retain destructive
   dequeue behavior. A failed handoff uses the dispatch's captured lifecycle
   rollback state before retrying.
   If another task session becomes selected before that final claim, the event
   is inserted or coalesced on the new selection before the source reservation
   is acknowledged. If the active task temporarily has no promptable session,
   the event remains queued on its original selection.
   A successful claim reconciles the task to `IN_PROGRESS` and publishes the
   session's `RUNNING` transition before visible message persistence or
   executor dispatch.
6. Kandev stamps the checkpoint and clears `last_error` only after the prompt
   is accepted or durably queued. Queue acceptance and prompt claim require an
   active task: archive/delete winning the race produces no checkpoint, queued
   message, or prompt; acceptance winning is subject to normal archive
   cancellation semantics. Archive/delete privileged cleanup also purges
   reserved lifecycle rows and advances the task queue generation, so stale
   accepted, reserved, or in-flight work cannot reinsert after a later
   unarchive. A failed attempt records `last_error` and remains eligible on a
   later poll.
7. A subscribed terminal watch remains attached to the PR until the terminal
   prompt is accepted; legacy reset-to-search behavior resumes afterward.
8. Archiving or deleting the task removes it from lifecycle evaluation.

Auto-fix cycle for one task/PR:

1. Existing PR watch poll, PR feedback event, or CI options save wakes automation.
2. Kandev syncs the latest lightweight PR state for the task's linked PRs, including linked PR rows that do not currently have an active watch.
3. Kandev fetches full PR feedback.
4. If the latest lightweight PR state or fetched check list shows any queued, pending, or in-progress check, the cycle ends without prompting or counting a round.
5. Kandev filters feedback down to prompt-worthy signals. These signals include
   failed completed checks, review comments, and provider-owned conflict state.
   Bot-authored status comments without another actionable signal are ignored.
6. Kandev compares the current actionable snapshot and provider generation to
   `last_fix_checkpoint_json`, `last_fix_signature`, and the current attempt
   state. GitHub conflict and queue-removal rules come from the integration
   system designs. Check execution timestamps and the PR head identify reruns
   and pushes even when a check name, URL, conclusion, and output are unchanged.
7. If there is no material change and the current attempt is queued, running,
   awaiting provider progress within its two-watch-interval deadline, or
   acknowledged, the cycle ends without prompting. A retryable attempt is
   eligible even when its feedback signature is unchanged.
8. If there is new or materially changed prompt-worthy feedback, Kandev renders the task override or default `ci-auto-fix` prompt and sends or queues it for the task session. If `last_fix_session_id` is already set for this task/repository/PR, Kandev targets that same session instead of the newest active session for the task. Otherwise, Kandev targets the active primary session when one exists, falling back to the newest active session only when there is no primary active session. The saved/shared `ci-auto-fix` instructions are hidden system context. If the rendered prompt contains `{{pr.feedback}}`, Kandev replaces it with visible PR snapshot details after `@ci-auto-fix`, before the agent output for that automation turn. If the placeholder is absent, no PR snapshot is included in the chat message.
9. The default prompt instructs the agent to classify the new feedback before editing. If the
   new feedback is only summaries, status updates, no-finding reports, duplicated or already
   addressed comments, rate-limit notices, or other non-actionable review diagnostics, the agent
   must not modify files, commit, or push; it should only report that there is nothing actionable
   to address. When the agent addresses actionable PR review comments, the default prompt instructs
   it to reply with a fix summary and resolve the addressed PR review threads so they do not keep
   the PR blocked. Kandev also appends an immutable, server-owned instruction
   requiring `report_pr_auto_fix_outcome_kandev`; prompt overrides cannot omit
   or replace it. Structured sessions receive it as trusted hidden context,
   while passthrough sessions receive equivalent plain terminal input.
10. Once the prompt is queued or accepted by the agent runtime, Kandev records
    the signature, checkpoint, provider generation, session, queue entry, and
    attempt state. Queue delivery binds the accepted prompt to its exact turn
    before that turn can disposition the attempt.
11. The task-bound outcome tool accepts only the unresolved attempt identified
    by the calling session's current turn metadata. It records one first-write
    outcome: `action_taken`, `non_actionable`, or `blocked`, plus a bounded
    plain-text summary. Stale, cross-task, cross-session, cross-turn, and
    already-dispositioned calls fail without changing state.
12. `action_taken` moves the attempt to `awaiting_provider_progress`. A later
    evaluation acknowledges it when the PR head, check execution identity,
    review-thread state, conflict state, or queue-removal state changed. If no
    such change appears within two complete watch intervals, it becomes
    `retryable`. `non_actionable` moves directly to `acknowledged`. `blocked`
    also acknowledges the snapshot and writes the summary to `last_error`.
13. A successful turn or recoverable agent failure that ends without a valid
    outcome moves its matching running attempt to `retryable`. The next settled
    PR evaluation may create a new round for the full current actionable
    snapshot. User cancellation, task archival/deletion, a terminal PR, or an
    explicitly disabled auto-fix option does not schedule a retry.
14. If the task session is busy and a pending auto-fix entry for this task/repository/PR already exists, Kandev replaces that queued entry with the latest rendered prompt instead of appending a second queued message. The round count is unchanged.
15. If a new prompt would require an 11th accepted auto-fix round for the same task/repository/PR, Kandev does not send or queue the prompt. It records a paused error and keeps the chip visible as `Auto-fix 10/10`. Disabling and re-enabling auto-fix resets the round count, attempt state, and paused state for the task's PR automation rows.

Auto-merge cycle for one task/PR:

1. Existing PR watch poll updates lightweight PR state.
2. Kandev checks merge readiness.
3. If the readiness state matches `last_merge_signature` for a failed prior attempt, the cycle ends without retrying.
4. If the PR is ready and the readiness signature is new, Kandev calls the existing PR merge operation using the backend default merge-method selection.
5. Kandev records the merge attempt and refreshes PR state after a successful merge when practical.

## Permissions

- Any user who can view and interact with the task chat can read and update the task CI automation options for that task.
- Any user who can edit prompts in Settings > Prompts can edit the default `ci-auto-fix` prompt.
- Automation runs with the backend's configured GitHub credentials and the existing task-session execution permissions.
- Auto-merge must fail closed when GitHub credentials are missing, invalid, or lack permission to merge the PR.

## Failure modes

| Dependency / invariant                                                 | Behavior                                                                                                                                                                                                                                                        |
| ---------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| GitHub auth is missing or invalid                                      | Controls remain visible but saving/enabling or automation execution surfaces an error; no auto-fix prompt, lifecycle prompt, or merge is attempted.                                                                                                             |
| Workspace GitHub login changes                                         | Kandev atomically rebinds `review_reviewer_login`, resets review-request baselines, and emits no notification for the identity change itself. If identity lookup fails, it preserves the prior login and checkpoints and retries later.                            |
| PR is closed or merged                                                 | Auto-fix and auto-merge stop. The matching enabled terminal prompt remains eligible exactly once per observed terminal entry.                                                                                                                                   |
| Full PR feedback fetch fails                                           | Auto-fix does not prompt; per-PR automation state records the error and the next materially changed lightweight status may retry.                                                                                                                               |
| Task has no promptable session                                         | Auto-fix and lifecycle delivery record a per-PR error instead of creating a surprising new session. Lifecycle events remain unstamped and retry when a session becomes promptable.                                                                               |
| Task session is busy                                                   | Auto-fix queues the rendered prompt with workflow/automation metadata for later delivery; the visible `@ci-auto-fix` chat message, including PR snapshot details, is created when the queued prompt is delivered and before the agent's response for that turn. |
| Task session is busy during a lifecycle event                          | Kandev queues one visible automation message per task/repository/PR/event and does not interrupt the running turn. Duplicate observations of that same event coalesce.                                                                                           |
| Task session is busy and a pending auto-fix already exists for that PR | Kandev replaces the pending queued prompt with the latest feedback snapshot; it does not append a second queued message or increment the round count.                                                                                                           |
| Same actionable snapshot repeats while its attempt is queued or running | Auto-fix coalesces the observation and does not start another round.                                                                                                                                                                                             |
| Auto-fix turn ends without an outcome                                  | The matching attempt becomes retryable; the next settled PR evaluation may start another round for the same snapshot.                                                                                                                                           |
| Agent reports action but provider state stays unchanged                | Kandev waits two complete PR watch intervals, then makes the attempt retryable.                                                                                                                                                                                  |
| Agent reports non-actionable feedback                                  | Kandev acknowledges the current snapshot and waits for materially changed feedback.                                                                                                                                                                             |
| Agent reports blocked work                                             | Kandev acknowledges the current snapshot, exposes the bounded reason through `last_error`, and waits for materially changed feedback or a user reset.                                                                                                           |
| Auto-fix reaches 10 rounds for a PR                                    | Kandev pauses auto-fix for that task/repository/PR, records a visible error, and does not create an 11th round. Already exhausted PRs skip full feedback fetching on later watcher wakes.                                                                       |
| GitHub merge fails                                                     | Auto-merge records the error and does not retry until the readiness signature changes.                                                                                                                                                                          |
| Default prompt row is missing                                          | Backend falls back to the embedded `ci-auto-fix.md` content.                                                                                                                                                                                                    |
| Default prompt row matches an untouched legacy revision                | Backend replaces it with the current embedded default by an exact-content and unchanged-timestamp conditional update. Edited rows remain unchanged.                                                                                                             |
| Kandev restarts while an automation prompt is queued                   | Queued message and automation options/checkpoints persist according to the existing message queue and new CI automation tables.                                                                                                                                 |
| Kandev restarts with a running attempt whose turn is already terminal  | Startup reconciliation makes the attempt retryable unless a valid outcome was durably recorded before shutdown.                                                                                                                                                 |
| Kandev stops before acknowledging a reserved lifecycle prompt          | The durable row remains and is delivered again after restart. If the executor accepted the prompt immediately before the stop, the retry can duplicate that prompt; lifecycle delivery is at-least-once rather than lossy.                                      |
| Review-request identity lookup fails                                   | Preserve the prior login and request checkpoint, record the error, and retry on a later PR lifecycle tick.                                                                                                                                                      |
| Lifecycle prompt delivery fails                                        | Record `last_error`, do not stamp the edge, and retain a durable coalesced retry for busy/transient failures or an active task with no promptable session. A zero-row guarded claim re-reads task/session state in the same transaction: active non-promptable is busy and retained, while missing/inactive is discarded. Later success clears the error. |
| Selected lifecycle session changes                                     | Requeue the accepted event to the newly selected task session; do not dispatch through the stale session or discard the event.                                                                                                                                  |
| Visible lifecycle message cannot be persisted                          | Restore the pre-claim session state, close the turn only if this dispatch created it, retain one durable retry, and do not prompt the executor for that attempt. A pre-existing turn remains open.                                                               |
| Task is archived or deleted                                            | Remove or ignore its PR watches and task-bound automation state; no lifecycle event can wake or recreate the task.                                                                                                                                               |
| Lifecycle evaluation and CI automation both report an error            | Lifecycle evaluation runs before auto-fix/auto-merge error persistence, so a successful lifecycle delivery cannot erase a same-pass CI error.                                                                                                                  |
| PR is merged/closed before the next cleanup cycle                       | Enabled lifecycle prompt options retain `cleanup_policy=auto` review tasks; `always` remains an explicit deletion override. |

## Persistence guarantees

- Task CI options persist until the task or its automation options row is deleted.
- Per-PR automation state persists the queue entry, bound session/turn,
  provider generation, outcome, deadline, and retryable state. Restart
  reconciliation preserves queued work and rearms a terminal undispositioned
  turn instead of suppressing it forever.
- A queued lifecycle row persists until executor prompt acceptance is
  acknowledged. Restart before acknowledgement redelivers it; a crash after
  external acceptance but before acknowledgement can duplicate it. This
  at-least-once boundary prevents silent queue loss.
- Archive/delete invalidates lifecycle delivery across the queue's accepted,
  reserved, and in-flight states. Its privileged purge includes reservations,
  and the task queue generation rejects stale retries after unarchive.
- For a production SQLite queue, archive/delete and workspace-cascade cleanup
  purge persistent rows and advance task generations in the same task
  transaction. Any registered or fallback in-memory queue is mirrored only
  after commit; a workspace cascade notifies that mirror once for each task it
  captured for deletion.
- The default prompt row persists in `custom_prompts`. Startup upgrades only
  exact, untouched shipped legacy revisions; arbitrary user edits are not
  overwritten by reseeding.
- The existing 1-minute PR poller cadence, 30-second lightweight PR status cache, and 8-second full PR feedback cache remain cache behavior, not user-visible persistence guarantees.
- In-memory singleflight/cache state does not survive restart and must not be required for dedupe correctness.
