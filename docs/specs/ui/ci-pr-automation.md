---
status: building
created: 2026-06-18
owner: tbd
---

# Task PR Automation Controls

## Why

Users can already see pull request CI/review status above the task chat input,
but acting on a red PR still requires repeatedly noticing the failure, prompting
the agent, and deciding when it is safe to merge. A review task can also go
idle after submitting a review and miss a later re-review request, merge, or
close. Users and task agents need one task-level control plane that keeps a
linked PR moving throughout its lifecycle.

Decision: [ADR-0051](../../decisions/0051-pr-agent-notifications-extend-task-pr-automation.md).

## What

- The PR CI popover above the chat input shows five task-level automation controls:
  - `Auto-fix CI & address comments`
  - `Auto-merge when ready`
  - `Your review is requested`
  - `PR merged`
  - `PR closed without merging`
- The automation section includes an info icon or equivalent help affordance that explains what each control watches, how often Kandev checks watched PRs, how feedback snapshots prevent duplicate prompts, and how auto-merge decides readiness.
- The same controls are available anywhere the task PR CI popover is rendered, including the normal chat input status bar and passthrough toolbar surfaces.
- The shared desktop popover and mobile drawer keep auto-fix and auto-merge in
  the primary automation list. The three agent lifecycle prompt switches live
  together in a collapsed `Review follow-up` section.
- `Review follow-up` is presentation only. Its switches retain the same
  task-wide behavior and remain reachable on desktop and mobile. When any of
  its three options is enabled, the section opens so active automation is not
  concealed.
- Lifecycle switches stay compact single-line rows. Their explanations live in
  on-demand help affordances (hover tooltip on fine pointers, tap popover on
  touch) and screen-reader descriptions, not inline copy: the review-request
  switch explains `Wake the agent for any new request, including re-review
  after changes.`, and the two terminal switches share an explanation that they
  wake the agent when review work ends while remaining independently
  configurable.
- `Auto-fix CI & address comments` causes Kandev to send or queue an agent prompt when a linked PR gets actionable CI or review feedback.
- `Auto-merge when ready` causes Kandev to merge a linked PR only when the PR is open and not a draft, checks are passing, review requirements are satisfied, unresolved review threads are cleared, and the PR is cleanly mergeable.
- `Your review is requested` follows the GitHub account connected to the task's
  workspace. It silently baselines that account's current request state, then
  sends or queues a task notification on each later false-to-true request
  transition. This includes an initial request observed after baselining and a
  later re-review request after the prior request clears.
- If the connected GitHub account changes, Kandev atomically binds the task to
  the new login and silently re-establishes every linked PR's review-request
  baseline. The identity change itself never produces a review-request prompt.
- `PR merged` and `PR closed without merging` send or queue one notification
  when the linked PR enters that terminal state. The first
  complete observation also prompts when the option was enabled after the PR
  had already entered the subscribed terminal state. An observed open state
  rearms a later close.
- The three lifecycle prompts are immutable, versioned, server-owned templates.
  Their only dynamic value is the linked PR's validated canonical GitHub URL;
  they never include GitHub titles, branches, comments, review text, or
  caller-supplied content. Each template only reports the observed event. The
  agent uses its task context and workflow instructions to decide what, if
  anything, to do next.
- Lifecycle prompt text is not configurable through the UI, HTTP, MCP, or
  storage. HTTP and current-task MCP expose only the three lifecycle booleans;
  the PR automation UI exposes the same switches.
- Lifecycle prompts are visible automation-generated chat messages with
  task/repository/PR/event metadata. Repeated observations of the same event
  coalesce, while different events and different linked PRs keep distinct
  queue entries.
- Agent-, workflow-, and server-owned queue entries are reserved for backend
  dispatch. Browser and MCP clients can create, edit, append to, cancel, or
  remove only user-owned queue entries, so they cannot rewrite or discard a
  pending lifecycle prompt.
- The lifecycle switches use the task's active primary promptable session,
  falling back to another active promptable session. A busy session is not
  interrupted. A current primary session in `IDLE` or `WAITING_FOR_INPUT`
  receives the lifecycle prompt immediately; Kandev queues only when it is
  busy or delivery must retry.
- Kandev does not create a new session when a task has no promptable session.
  It records the per-PR automation error, keeps the event eligible, and retries
  after a session becomes promptable.
- If task-level session selection changes after lifecycle acceptance, Kandev
  durably requeues the accepted event to the newly selected session. An active
  task with no currently promptable session retains the original event; only a
  missing, archived, or deleted task discards it.
- Archived and deleted tasks are not evaluated or reactivated by task PR
  automation.
- Archive and deletion are durable queue invalidation boundaries. Privileged
  backend cleanup purges lifecycle rows even when they are reserved, and a
  task-queue generation prevents accepted, reserved, or in-flight lifecycle
  work from reinserting a stale retry after the task is later unarchived.
- Lifecycle queue acceptance and prompt claim require an active task. If archive
  or deletion wins the race, Kandev creates no lifecycle checkpoint, queued
  message, or prompt. The same guard applies to every lifecycle retry: busy and
  transient failures retain one durable coalesced retry, while an inactive-task
  retry is discarded even if the task is later unarchived. If acceptance wins,
  normal archive cancellation semantics cancel the accepted work.
- For queued lifecycle delivery, Kandev runs turn-start/runtime/model
  preparation before a final active-task claim under the session cancel guard
  and current queue token. It records the visible automation message only after
  that claim succeeds. A reset or superseded token after a claim restores the
  pre-claim session state and requeues; an archive/inactive loser discards the
  entry without a visible message.
- Ordinary messages keep the queue's existing take-and-delete behavior.
  Lifecycle delivery instead reserves the FIFO head and leaves its row durable
  until the PTY/executor accepts the prompt. A per-session in-process guard
  permits only one dispatch of that reservation at a time. A failed executor
  handoff restores the captured lifecycle dispatch state and retains or
  coalesces the row; inactive-task outcomes acknowledge and discard it.
- A passthrough session defers a reserved lifecycle row until its ready-handler
  guard is released. It claims that row's in-flight token before releasing the
  guard, and the deferred lifecycle dispatcher consumes the preclaimed token.
  This keeps the reservation serialized across the deferral boundary and then
  uses this same lifecycle dispatcher rather than a direct stdin write. Its
  final claim, visible-message ordering, retry, and acknowledgement behavior
  therefore matches non-passthrough delivery.
- If visible-message persistence fails after the claim, Kandev restores the
  pre-claim session state, completes the turn created for that dispatch, and
  requeues the event without calling the executor. Task-state rollback succeeds
  only while the task is still `IN_PROGRESS`, so it cannot overwrite a
  concurrent terminal transition or archive.
- After submitting its initial review, Kandev's built-in `PR Review` workflow
  uses the current-task MCP tool to enable review-requested, merged, and closed
  prompting. Other workflows and tasks remain opt-in through UI or MCP.
- The auto-fix prompt is customizable per task from the PR CI popover.
- The per-task prompt editor is opened from an edit button in the automation section.
- The per-task prompt editor links to Settings > Prompts so the user can edit the default `ci-auto-fix` prompt.
- The per-task prompt editor explains that `{{pr.feedback}}` is the placeholder that inserts Kandev's PR feedback snapshot. The explanation lists the included data: PR identifier, new or changed failing checks with job links, and new or changed review comments with file, line, and body text.
- Omitting `{{pr.feedback}}` from the prompt means Kandev still evaluates PR feedback for dedupe and trigger decisions, but it does not include the PR snapshot in the agent message. This supports prompts that tell the agent to pull/fetch the branch and inspect GitHub itself.
- If a task has no custom auto-fix prompt, Kandev uses a built-in default prompt named `ci-auto-fix`.
- The default `ci-auto-fix` prompt is editable from Settings > Prompts like other built-in prompts.
- Emptying or resetting the task prompt override returns the task to the default `ci-auto-fix` prompt.
- For tasks with multiple linked PRs, the controls are task-level and apply to
  every linked PR. Dedupe, last-attempt, review-request, and terminal state are
  tracked per linked PR.
- Kandev checks watched PRs through the existing lightweight PR watch poller, which runs once per minute. Automation wakeups sync the latest lightweight PR state before evaluating gates. When auto-fix is enabled, Kandev fetches full PR feedback so failing checks, requested changes, unresolved threads, and human PR conversation comments can trigger deduped prompts even when the persisted lightweight row was stale. Auto-fix waits until all PR checks have finished before sending or queueing a prompt, so the agent receives the final check set and current comments in one pass. Bot-authored PR conversation comments without failed checks or unresolved review threads are treated as non-actionable status chatter and do not send an agent prompt.
- Lightweight PR status sync counts unresolved review threads across every page
  returned by GitHub. A connection's `totalCount` indicates that more threads
  exist; it never classifies omitted threads as unresolved. The CI popover,
  auto-fix eligibility, and auto-merge readiness consume only a complete
  review-thread count.
- If Kandev cannot finish review-thread pagination, it discards the partial
  count and follows the existing PR-status sync failure path. A partial page
  never replaces the last complete persisted count or becomes fresh automation
  input. If the initial batch also identified unresolvable repositories, those
  classifications still reach the existing negative cache even when another
  repository's continuation fails.
- Branch-only PR discovery associates PR metadata without fetching unused
  review-thread continuation pages. Once the watch has a PR number, the next
  numbered status sync produces the complete review-thread count.
- Saving PR automation options while any option is enabled immediately
  evaluates the task's current linked PRs instead of waiting for the next PR
  watch poll. Prompt edits do not reset unchanged checkpoints.
- Every auto-fix attempt records the latest actionable feedback snapshot it used. Later fix rounds include only new or materially changed CI/review feedback since the last recorded round, with enough summary context for the agent to understand the PR. If a previously recorded feedback snapshot becomes non-actionable after checks pass or review threads are cleared, Kandev can refresh the checkpoint without sending a prompt or counting another round.
- The first auto-fix round targets the task's active primary session when one exists. Once a PR has an accepted auto-fix round, later auto-fix prompts for that task/repository/PR continue targeting the recorded `last_fix_session_id`. A newer active agent session for the same task must not steal auto-fix messages. Disabling and re-enabling auto-fix resets this binding with the rest of the per-PR auto-fix state.
- Automation must not repeatedly prompt for the same failure/comment snapshot or repeatedly retry the same failed merge attempt on every poll.
- When auto-fix is enabled and the task session is busy, Kandev keeps at most one pending CI auto-fix queue entry per task/repository/PR. Newer feedback replaces that pending entry instead of appending another queued `@ci-auto-fix` message.
- Auto-fix is capped at 10 accepted rounds per task/repository/PR. A round is counted when Kandev sends a prompt directly or inserts a new queued auto-fix prompt. Replacing an already queued auto-fix prompt does not count as another round.
- The auto-fix enabled chip above the chat input shows round progress as `Auto-fix N/10`; PRs paused by the backend after the cap is reached show `Auto-fix 10/10` with warning/paused styling.
- Hovering the round-count help icon on desktop, or opening the same PR CI drawer on mobile and using the same help affordance, explains in plain language how many rounds have been used, what counts as a round, that queue replacement does not count again, and that Kandev pauses when 10/10 has no pending auto-fix message left to update.
- Accepted round-count changes and exhausted-state changes are broadcast to open clients through the task CI options update event so the chip stays current without a reload.
- The PR automation popover/drawer shows the selected linked PR's
  `last_error`, including lifecycle delivery failures, and clears that error
  after a later successful delivery.
- The GitHub Review Watch `Auto` cleanup description explains that user
  engagement or enabled PR lifecycle prompts retain a terminal review task;
  `Always delete` remains the explicit override.
- Automation controls persist across Kandev restarts.

## Data model

`github_task_ci_options`

- `task_id` string, primary key. References the Kandev task that owns the controls.
- `auto_fix_enabled` boolean, default `false`.
- `auto_merge_enabled` boolean, default `false`.
- `auto_fix_prompt_override` string nullable. `NULL` or empty means use the default `ci-auto-fix` prompt.
- `prompt_on_review_requested` boolean, default `false`.
- `prompt_on_merged` boolean, default `false`.
- `prompt_on_closed` boolean, default `false`.
- `review_reviewer_login` string, default `""`. Bound to the current connected
  GitHub login when review-requested prompting is enabled and rebound when that
  authenticated identity changes.
- Legacy `review_prompt_override`, `merged_prompt_override`, and
  `closed_prompt_override` nullable columns remain only for additive migration
  compatibility. Startup clears persisted values and runtime ignores them.
- `created_at` timestamp.
- `updated_at` timestamp.

`github_task_ci_pr_state`

- Primary key: `task_id`, `repository_id`, `pr_number`.
- `task_id` string. References the Kandev task.
- `repository_id` string. Identifies which linked repository/branch row produced the PR.
- `pr_number` integer.
- `last_fix_signature` string, default `""`. Deterministic hash of the latest feedback snapshot that produced an auto-fix prompt, or a later prompt-free checkpoint refresh that pruned resolved feedback.
- `last_fix_checkpoint_json` string, default `""`. JSON snapshot of feedback used in the last fix round or prompt-free checkpoint refresh.
- `last_fix_enqueued_at` timestamp nullable.
- `last_fix_session_id` string nullable. Pins later auto-fix rounds for this task/repository/PR to the same task session.
- `auto_fix_round_count` integer, default `0`. Counts accepted auto-fix rounds for this task/repository/PR.
- `auto_fix_exhausted_at` timestamp nullable. Set when Kandev pauses auto-fix after the 10-round cap.
- `last_merge_signature` string nullable. Deterministic hash of the last readiness state used for a merge attempt.
- `last_merge_attempt_at` timestamp nullable.
- `last_error` string nullable. Latest user-visible automation error for this task/PR pair.
- `review_request_initialized` boolean, default `false`.
- `last_review_requested` boolean, default `false`.
- `last_observed_pr_state` string, default `""`. Records the open/closed/merged
  observation used to detect terminal entry and rearm close.
- `last_lifecycle_event` string, default `""`. The latest accepted lifecycle
  prompt (`review_requested`, `merged`, or `closed`).
- `last_lifecycle_prompt_at` timestamp nullable.
- `last_lifecycle_session_id` string nullable.
- `created_at` timestamp.
- `updated_at` timestamp.

`custom_prompts`

- The existing prompt table includes a built-in prompt row:
  - `id = "builtin-ci-auto-fix"`
  - `name = "ci-auto-fix"`
  - `builtin = true`
  - `content` seeded from `apps/backend/config/prompts/ci-auto-fix.md`
- User edits to the built-in row are preserved. The embedded markdown is a fallback when the row is missing.

## API surface

HTTP endpoints under `/api/v1/github`:

```http
GET /tasks/:taskId/ci-options
```

Response:

```json
{
  "task_id": "task-123",
  "auto_fix_enabled": false,
  "auto_merge_enabled": false,
  "prompt_on_review_requested": false,
  "prompt_on_merged": false,
  "prompt_on_closed": false,
  "review_reviewer_login": "",
  "auto_fix_prompt_override": null,
  "auto_fix_max_rounds": 10,
  "effective_auto_fix_prompt": "Fix the PR feedback...",
  "using_default_prompt": true,
  "updated_at": "2026-06-18T00:00:00Z",
  "pr_states": [
    {
      "repository_id": "repo-123",
      "pr_number": 42,
      "last_fix_enqueued_at": null,
      "auto_fix_round_count": 0,
      "auto_fix_exhausted_at": null,
      "last_merge_attempt_at": null,
      "last_error": null
    }
  ]
}
```

```http
PATCH /tasks/:taskId/ci-options
```

Request fields are partial:

```json
{
  "auto_fix_enabled": true,
  "auto_merge_enabled": false,
  "prompt_on_review_requested": true,
  "prompt_on_merged": true,
  "prompt_on_closed": true,
  "auto_fix_prompt_override": "Use this task-specific prompt..."
}
```

`auto_fix_prompt_override: null` or an empty string clears the task override.
The response shape matches `GET`. Lifecycle override fields are rejected.

Task-mode MCP exposes current-task-only tools:

- `get_task_pr_automation_kandev`
- `update_task_pr_automation_kandev`

The MCP connection supplies the task ID. Update is partial and accepts auto-fix,
auto-merge, the three lifecycle booleans, and the auto-fix prompt override; it
cannot target another task. Lifecycle override fields are rejected.

Optional websocket notification:

- `github.task_ci_options.updated`
- Payload: the same options response shape.
- The event is emitted after a successful options update so other open tabs refresh immediately and the backend can evaluate any currently linked PRs when automation is enabled.

## State machine

Task CI automation options:

- `disabled`: all five automation switches are false. PR watch events update UI only.
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
5. Kandev filters feedback down to prompt-worthy signals: failed, timed-out, cancelled, or action-required completed checks, unresolved review-thread comments, and human PR conversation comments. Bot-authored PR conversation comments without a failed check or unresolved thread are ignored before delta computation.
6. Kandev compares the current feedback snapshot to `last_fix_checkpoint_json` and `last_fix_signature`.
7. If there is no material change, the cycle ends without prompting.
8. If there is new or materially changed prompt-worthy feedback, Kandev renders the task override or default `ci-auto-fix` prompt and sends or queues it for the task session. If `last_fix_session_id` is already set for this task/repository/PR, Kandev targets that same session instead of the newest active session for the task. Otherwise, Kandev targets the active primary session when one exists, falling back to the newest active session only when there is no primary active session. The saved/shared `ci-auto-fix` instructions are hidden system context. If the rendered prompt contains `{{pr.feedback}}`, Kandev replaces it with visible PR snapshot details after `@ci-auto-fix`, before the agent output for that automation turn. If the placeholder is absent, no PR snapshot is included in the chat message.
9. The default prompt instructs the agent to classify the new feedback before editing. If the
   new feedback is only summaries, status updates, no-finding reports, duplicated or already
   addressed comments, rate-limit notices, or other non-actionable review diagnostics, the agent
   must not modify files, commit, or push; it should only report that there is nothing actionable
   to address. When the agent addresses actionable PR review comments, the default prompt instructs
   it to reply with a fix summary and resolve the addressed PR review threads so they do not keep
   the PR blocked.
10. Once the prompt is queued or accepted by the agent runtime, Kandev records the new signature/checkpoint and attempt metadata for the latest prompt-worthy feedback snapshot, so identical snapshots are not sent repeatedly while the agent is still working.
11. If the task session is busy and a pending auto-fix entry for this task/repository/PR already exists, Kandev replaces that queued entry with the latest rendered prompt instead of appending a second queued message. The round count is unchanged.
12. If a new prompt would require an 11th accepted auto-fix round for the same task/repository/PR, Kandev does not send or queue the prompt. It records a paused error and keeps the chip visible as `Auto-fix 10/10`. Disabling and re-enabling auto-fix resets the round count and paused state for the task's PR automation rows.

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
| Same feedback snapshot repeats                                         | Auto-fix does not send another prompt.                                                                                                                                                                                                                          |
| Auto-fix reaches 10 rounds for a PR                                    | Kandev pauses auto-fix for that task/repository/PR, records a visible error, and does not create an 11th round. Already exhausted PRs skip full feedback fetching on later watcher wakes.                                                                       |
| GitHub merge fails                                                     | Auto-merge records the error and does not retry until the readiness signature changes.                                                                                                                                                                          |
| Default prompt row is missing                                          | Backend falls back to the embedded `ci-auto-fix.md` content.                                                                                                                                                                                                    |
| Kandev restarts while an automation prompt is queued                   | Queued message and automation options/checkpoints persist according to the existing message queue and new CI automation tables.                                                                                                                                 |
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
- Per-PR automation state persists across restarts so duplicate prompts and merge retries do not resume after restart.
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
- The default prompt row persists in `custom_prompts`; user edits are not overwritten by reseeding.
- The existing 1-minute PR poller cadence, 30-second lightweight PR status cache, and 8-second full PR feedback cache remain cache behavior, not user-visible persistence guarantees.
- In-memory singleflight/cache state does not survive restart and must not be required for dedupe correctness.

## Scenarios

- **GIVEN** a task with one open linked PR, **WHEN** the user opens the CI popover above the chat input, **THEN** the popover shows the current CI/review summary and all five automation controls.
- **GIVEN** a linked PR with more than 100 review threads and every thread is
  resolved, **WHEN** Kandev completes its lightweight PR status sync, **THEN**
  the CI popover shows no unresolved-review row and automation evaluates an
  unresolved-thread count of zero.
- **GIVEN** a linked PR whose unresolved review threads span more than one
  GitHub page, **WHEN** Kandev completes its lightweight PR status sync,
  **THEN** the persisted and displayed count equals the unresolved threads
  across all pages.
- **GIVEN** Kandev has a complete persisted review-thread count and GitHub
  fails a later pagination request, **WHEN** the lightweight PR status sync
  runs, **THEN** no partial or inferred count replaces that complete value.
- **GIVEN** one repository is unresolvable and another repository's
  review-thread continuation fails in the same batch, **WHEN** lightweight PR
  status sync handles the failure, **THEN** it discards all partial statuses
  while still negative-caching the unresolvable repository.
- **GIVEN** a user is viewing the CI popover automation controls, **WHEN** they activate the info icon, **THEN** they see help text explaining that Kandev uses the existing 1-minute PR watch checks, fetches full feedback only for candidate PRs, snapshots each auto-fix round, and merges only when readiness gates pass.
- **GIVEN** a task with one open linked PR, **WHEN** the user enables `Auto-fix CI & address comments`, **THEN** the setting persists and remains enabled after page reload.
- **GIVEN** a task with one open linked PR, **WHEN** the user enables `Auto-merge when ready`, **THEN** the setting persists and remains enabled after page reload.
- **GIVEN** a task using the default auto-fix prompt, **WHEN** the user edits the prompt from the CI popover, **THEN** only that task uses the custom prompt and Settings > Prompts continues to hold the global default.
- **GIVEN** the task prompt editor is open, **WHEN** the user follows the default-prompt settings link, **THEN** Kandev opens Settings > Prompts where the `ci-auto-fix` default can be edited.
- **GIVEN** a task with a custom auto-fix prompt, **WHEN** the user resets the prompt override, **THEN** the task uses the current default `ci-auto-fix` prompt.
- **GIVEN** the default `ci-auto-fix` prompt is edited in Settings > Prompts, **WHEN** a task without an override later auto-fixes a PR, **THEN** the rendered prompt uses the edited default content.
- **GIVEN** auto-fix is enabled and a watched PR transitions from passing to failing CI, **WHEN** the 1-minute PR watch poll observes the failure, **THEN** Kandev fetches full PR feedback and sends or queues one auto-fix prompt for that failure snapshot.
- **GIVEN** auto-fix is enabled and a PR still has queued, pending, or in-progress checks, **WHEN** automation evaluates the PR, **THEN** Kandev does not send or queue an `@ci-auto-fix` prompt and does not count a round, even if some checks have already failed or comments are present.
- **GIVEN** auto-fix already prompted for a failure snapshot, **WHEN** the same failure is observed again on a later poll, **THEN** no duplicate prompt is sent.
- **GIVEN** auto-fix already prompted for a failure snapshot, **WHEN** a new failed check or new unresolved review comment appears, **THEN** Kandev sends or queues a new prompt containing the new or materially changed feedback.
- **GIVEN** auto-fix is enabled and a PR has only pending checks plus a bot-authored PR conversation/status comment, **WHEN** automation evaluates the PR, **THEN** Kandev does not send or queue an `@ci-auto-fix` prompt and does not count a round.
- **GIVEN** auto-fix is enabled and the task session is running, **WHEN** changed CI feedback appears multiple times for the same PR before the queue drains, **THEN** Kandev keeps one queued `@ci-auto-fix` entry for that PR and updates it with the latest feedback.
- **GIVEN** auto-fix is enabled on a task with a primary active session and a newer non-primary active session, **WHEN** the first actionable PR feedback appears, **THEN** Kandev sends or queues the auto-fix prompt on the primary session, not the newer session.
- **GIVEN** auto-fix has already accepted a round for one task session, **WHEN** another active session is created for the same task and new actionable PR feedback appears, **THEN** Kandev sends or queues the auto-fix prompt on the previously recorded session, not the newer session.
- **GIVEN** auto-fix has used 1 of 10 rounds for a PR, **WHEN** the user views the auto-fix chip above the chat input and opens the round-count help affordance, **THEN** the chip reads `Auto-fix 1/10` and the hover/drawer explanation states that one round out of ten has been used.
- **GIVEN** auto-fix has already used 10 rounds for a PR and no pending auto-fix queue entry exists, **WHEN** new actionable feedback appears, **THEN** Kandev does not send or queue another prompt and records the PR as paused at `Auto-fix 10/10`.
- **GIVEN** auto-fix has already used 10 rounds for a PR and the 10th round is still queued, **WHEN** new actionable feedback appears, **THEN** Kandev replaces that pending queued prompt without incrementing the round count.
- **GIVEN** auto-fix sends a prompt for feedback that the backend considered prompt-worthy but the agent determines is already addressed or otherwise non-actionable, **WHEN** the agent reviews that prompt, **THEN** the agent does not modify files, commit, or push and only reports that there is nothing actionable to address.
- **GIVEN** auto-fix is enabled and the task session is running, **WHEN** new actionable PR feedback appears, **THEN** the prompt is queued and delivered after the current turn rather than interrupting the running session, and the chat history shows the `@ci-auto-fix` user message with visible PR snapshot details before the agent output for the queued turn.
- **GIVEN** a linked draft PR has passing checks and GitHub reports clean mergeability, **WHEN** Kandev refreshes its status, **THEN** PR status surfaces identify it as a draft and do not present it as ready to merge.
- **GIVEN** auto-merge is enabled and the PR has passing checks, required reviews, no unresolved threads, and clean mergeability, **WHEN** the PR watch poll observes the ready state, **THEN** Kandev merges the PR with the existing backend merge-method selection.
- **GIVEN** auto-merge is enabled but the PR is a draft or has requested changes, pending required review, failing checks, unresolved threads, or dirty mergeability, **WHEN** the PR watch poll observes the state, **THEN** Kandev does not merge.
- **GIVEN** auto-merge attempted a ready-state merge and GitHub rejected it, **WHEN** the same ready state is observed again, **THEN** Kandev does not retry until the readiness signature changes.
- **GIVEN** a task has two open linked PRs, **WHEN** the user enables either automation control, **THEN** both PRs are eligible for automation and each PR records its own last-fix and last-merge state.
- **GIVEN** review-request prompting is enabled while the connected GitHub user
  is already requested, **WHEN** Kandev first evaluates the PR, **THEN** it
  records a quiet baseline and does not prompt.
- **GIVEN** review-request prompting was quietly baselined as false, **WHEN**
  that connected GitHub user is requested for the first time or requested
  again after a prior request cleared, **THEN** Kandev sends or queues exactly
  one visible `Your review was requested on {{pr.url}}.` message.
- **GIVEN** review-request prompting is enabled for connected account A,
  **WHEN** GitHub authentication changes to account B, **THEN** Kandev stores B,
  quietly re-baselines every linked PR, and does not mistake B's current
  request state for a new request event.
- **GIVEN** a merged or closed prompt is enabled, **WHEN** the linked PR enters
  that state, **THEN** Kandev sends or queues one terminal prompt and does not
  repeat it while the state remains stable.
- **GIVEN** a merged or closed prompt is newly enabled after the linked PR
  already entered that subscribed terminal state, **WHEN** Kandev first
  evaluates the PR, **THEN** it sends or queues that terminal prompt once.
- **GIVEN** a closed PR reopens and closes again, **WHEN** both transitions are
  observed, **THEN** the second close produces a new prompt.
- **GIVEN** a lifecycle event qualifies while the task session is running,
  **WHEN** later polls observe the same task/PR/event before the queue drains,
  **THEN** Kandev retains one queued message for that event and does not
  interrupt the running turn.
- **GIVEN** a lifecycle row is reserved for dispatch, **WHEN** Kandev restarts
  before acknowledgement, **THEN** the durable row is eligible for
  redelivery; if the executor accepted immediately before the restart, the
  prompt may be delivered twice rather than lost.
- **GIVEN** that restarted row still contains its prior in-flight marker,
  **WHEN** redelivery fails, **THEN** the returned and requeued copies omit the
  transient marker so the retry is visible and eligible to drain again.
- **GIVEN** a lifecycle row is workflow-owned, **WHEN** a browser or MCP client
  attempts to impersonate a reserved identity or edit, cancel, append to, or
  remove that row, **THEN** Kandev rejects the mutation and preserves the row.
- **GIVEN** lifecycle delivery reselects another active session, **WHEN** it
  transfers the event, **THEN** Kandev inserts or coalesces the target row
  before acknowledging the source reservation.
- **GIVEN** a lifecycle dispatch successfully claims an `IDLE` or
  `WAITING_FOR_INPUT` session, **WHEN** it proceeds toward visible delivery,
  **THEN** the task is `IN_PROGRESS` and the session's `RUNNING` transition is
  published before the visible message or executor prompt.
- **GIVEN** a guarded lifecycle claim updates no row because another writer
  made the active session non-promptable, **WHEN** Kandev classifies the
  result, **THEN** it treats the session as busy and retains the event for
  retry rather than discarding it as inactive.
- **GIVEN** two linked PRs or two distinct lifecycle events qualify while the
  task session is busy, **WHEN** Kandev queues them, **THEN** each PR/event has
  a distinct ordered queue entry.
- **GIVEN** a lifecycle event qualifies while the task has no promptable
  session, **WHEN** delivery fails, **THEN** Kandev shows the per-PR automation
  error, leaves the event unstamped, and retries after a session becomes
  promptable.
- **GIVEN** a lifecycle delivery previously recorded `last_error`, **WHEN** a
  later attempt is accepted or durably queued, **THEN** Kandev clears the error
  in the desktop popover and mobile drawer.
- **GIVEN** visible lifecycle message persistence fails, **WHEN** the dispatch
  rolls back, **THEN** it closes only a turn created by that dispatch, leaves
  any pre-existing turn open, and restores task state only if it is still
  `IN_PROGRESS` rather than clobbering a concurrent terminal/archive state.
- **GIVEN** a task is archived or deleted, **WHEN** a linked PR later requests
  review, merges, closes, reopens, or closes again, **THEN** Kandev does not
  wake or recreate that task.
- **GIVEN** a review-watch-created task has lifecycle prompting enabled and
  cleanup policy `auto`, **WHEN** its PR becomes terminal, **THEN** cleanup
  retains the task for lifecycle automation.
- **GIVEN** a review-watch-created task has cleanup policy `always`, **WHEN**
  its PR becomes terminal, **THEN** the explicit deletion policy takes
  precedence over lifecycle prompting.
- **GIVEN** a task agent calls `update_task_pr_automation_kandev`, **WHEN** it
  enables the three lifecycle options, **THEN** the same options appear enabled
  in the related-PR Automation menu.
- **GIVEN** the user is on mobile, **WHEN** they open the PR CI drawer, **THEN** the automation controls and prompt editor are usable without text overflow or overlapping controls.
- **GIVEN** the task is shown in a passthrough toolbar surface, **WHEN** the user opens the PR CI popover/drawer, **THEN** the same automation controls are available.

## Out of scope

- Webhook-based GitHub event ingestion. This feature uses the existing PR watch poller.
- Changing the global PR watch poll interval.
- Selecting a destination workflow step for a lifecycle event. Lifecycle
  notifications prompt the task in its current workflow step; event-to-step
  routing is a follow-up feature.
- Per-PR automation toggles in the first version.
- Per-user automation preferences.
- Merge-method selection UI. Auto-merge uses the existing backend default merge-method selection.
- Team-level review-request matching. The first version tracks the
  authenticated user-level request.
- Creating a replacement task session when the existing task has no promptable
  session.
- Configuring lifecycle prompt text. The lifecycle templates are intentionally
  immutable and server-owned; only the three task-level booleans are exposed.
- Streaming CI logs into the chat or popover.
- Editing GitHub branch protection, review rules, or workflow files directly from the automation controls.
- GitLab merge request automation.

## Open questions

- None.
