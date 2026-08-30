---
status: draft
system: ui
requirements:
  - REQ-UI-CI-PR-AUTOMATION-001
created: 2026-06-18
owners:
  - tbd
---
# Task PR Automation Controls System Design Part 1

## Purpose and boundaries

This design preserves the technical source detail for `REQ-UI-CI-PR-AUTOMATION-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-CI-PR-AUTOMATION-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Why

Users can already see pull request CI/review status above the task chat input,
but acting on a red PR still requires repeatedly noticing the failure, prompting
the agent, and deciding when it is safe to merge. A review task can also go
idle after submitting a review and miss a later re-review request, merge, or
close. Users and task agents need automation controls that keep a linked PR
moving throughout its lifecycle, configured independently per linked PR so a
task with several open PRs does not force the same setting onto all of them.

Task-to-PR associations survive restarts and archive/unarchive. Hard deletion
removes task-owned associations and refresh watches; it is not contribution
history. Decision: ADR-2026-08-13-hard-delete-task-contribution-links.

Decision: [ADR-0051](../../../decisions/0051-pr-agent-notifications-extend-task-pr-automation.md)
(the task-level control plane for the five switches was superseded by per-PR
scoping; see that ADR's Consequences section).

## What

- The PR CI popover above the chat input shows five automation controls,
  scoped to the selected linked PR:
  - `Auto-fix CI & address comments`
  - `Auto-merge when ready`
  - `Your review is requested`
  - `PR merged`
  - `PR closed without merging`
- The automation section states which PR the controls apply to (the PR number,
  interpolated into localized copy), since a task can have several linked PRs
  each with independent settings.
- The automation section includes an info icon or equivalent help affordance that explains what each control watches, how often Kandev checks watched PRs, how feedback snapshots prevent duplicate prompts, and how auto-merge decides readiness.
- The same controls are available anywhere the task PR CI popover is rendered, including the normal chat input status bar and passthrough toolbar surfaces.
- The shared desktop popover and mobile drawer keep auto-fix and auto-merge in
  the primary automation list. The three agent lifecycle prompt switches live
  together in a collapsed `Review follow-up` section.
- `Review follow-up` is presentation only. Its switches are per-PR like the
  rest of the automation section and remain reachable on desktop and mobile.
  When any of its three options is enabled for the selected PR, the section
  opens so active automation is not concealed.
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
- For tasks with multiple linked PRs, each of the five automation switches is
  scoped per linked PR: `(task_id, repository_id, pr_number)`. Enabling a
  switch in one PR's tab has no effect on any other linked PR's switches. A
  `PATCH` that omits PR identity fans the requested change out to every
  currently linked PR (unchanged behavior for MCP callers that predate
  per-PR scoping); a newly linked PR always starts with all five switches
  off. `auto_fix_prompt_override` and `review_reviewer_login` remain
  task-level — one prompt override and one resolved reviewer identity apply
  to every linked PR. Dedupe, last-attempt, review-request, and terminal
  state are tracked per linked PR, as before.
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
- While the PR status chip is present above the chat input, active lifecycle
  prompting appears as one compact `PR events N/3` badge, where `N` is the
  number of enabled options among review requested, merged, and closed. The
  badge is absent when all three options are disabled. It remains one
  task-wide badge for tasks with multiple linked PRs and does not replace the
  independent auto-fix progress or auto-merge badges.
- The grouped PR events badge is presentation only. Activating the surrounding
  PR status chip opens the existing desktop popover or mobile drawer where the
  three `PR events` switches remain independently visible and
  configurable. The chip's accessible description identifies each enabled
  lifecycle event rather than exposing only the count.
- When status-bar items compete for space on phones or narrow windows, the
  composer tray wraps complete controls onto another line instead of clipping
  a badge, shrinking it into unreadable text, or creating document-level
  horizontal overflow. The PR chip and its badges remain one tap target; the
  responsive layout does not add a second automation control or overlay.
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

- `task_id` string, primary key. References the Kandev task that owns the
  task-level controls.
- `auto_fix_prompt_override` string nullable. `NULL` or empty means use the default `ci-auto-fix` prompt.
- `review_reviewer_login` string, default `""`. Bound to the current connected
  GitHub login when review-requested prompting is enabled for any linked PR,
  and rebound when that authenticated identity changes.
- `auto_fix_enabled`, `auto_merge_enabled`, `prompt_on_review_requested`,
  `prompt_on_merged`, `prompt_on_closed` columns remain on this table for
  migration purposes only: they are read once by the additive
  `pr_scope_migrated_at`-guarded fan-out into `github_task_pr_automation_options`
  and are never read or written after that. Their current source of truth is
  the per-PR table below.
- `pr_scope_migrated_at` timestamp nullable. Guards the one-time fan-out so it
  never re-runs and re-enables a switch a user has since turned off for one PR.
- Legacy `review_prompt_override`, `merged_prompt_override`, and
  `closed_prompt_override` nullable columns remain only for additive migration
  compatibility. Startup clears persisted values and runtime ignores them.
- `created_at` timestamp.
- `updated_at` timestamp.

`github_task_pr_automation_options`

- Primary key: `task_id`, `repository_id`, `pr_number`. Source of truth for
  the five automation switches, one row per linked PR.
- `auto_fix_enabled` boolean, default `false`.
- `auto_merge_enabled` boolean, default `false`.
- `prompt_on_review_requested` boolean, default `false`.
- `prompt_on_merged` boolean, default `false`.
- `prompt_on_closed` boolean, default `false`.
- `created_at` timestamp.
- `updated_at` timestamp.
- A PR with no row here (never configured) behaves as all-off; the response
  and UI synthesize an all-off placeholder rather than requiring one row per
  linked PR to exist up front.

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
  ],
  "pr_options": [
    {
      "task_id": "task-123",
      "repository_id": "repo-123",
      "pr_number": 42,
      "auto_fix_enabled": false,
      "auto_merge_enabled": false,
      "prompt_on_review_requested": false,
      "prompt_on_merged": false,
      "prompt_on_closed": false,
      "created_at": "2026-06-18T00:00:00Z",
      "updated_at": "2026-06-18T00:00:00Z"
    }
  ]
}
```

`pr_options` carries one entry per PR currently linked to the task — the
per-PR source of truth. The five top-level booleans are an aggregate kept for
MCP/API read compatibility: `true` only when every linked PR has that switch
on and at least one PR is linked, so they answer "did my task-wide enable
take" rather than any one PR's state.

```http
PATCH /tasks/:taskId/ci-options
```

Request fields are partial:

```json
{
  "repository_id": "repo-123",
  "pr_number": 42,
  "auto_fix_enabled": true,
  "auto_merge_enabled": false,
  "prompt_on_review_requested": true,
  "prompt_on_merged": true,
  "prompt_on_closed": true,
  "auto_fix_prompt_override": "Use this task-specific prompt..."
}
```

`repository_id` and `pr_number` are optional but must both be present or both
absent. When present, they target the five automation switches at that one
linked PR only; naming a `(repository_id, pr_number)` pair that is not
currently linked to the task returns `400` and writes nothing. When absent,
the five switches fan out to every PR currently linked to the task (unchanged
behavior for callers written before per-PR scoping). `auto_fix_prompt_override`
and the resolved reviewer login (set implicitly when
`prompt_on_review_requested` changes) always apply task-wide regardless of PR
identity. `auto_fix_prompt_override: null` or an empty string clears the task
override. The response shape matches `GET`. Lifecycle override fields are
rejected.

Task-mode MCP exposes current-task-only tools:

- `get_task_pr_automation_kandev`
- `update_task_pr_automation_kandev`

The MCP connection supplies the task ID. Update is partial and accepts the
same optional `repository_id`/`pr_number` PR-identity pair as the HTTP PATCH,
auto-fix, auto-merge, the three lifecycle booleans, and the auto-fix prompt
override; it cannot target another task. Omitting PR identity fans the
switches out to every linked PR, preserving pre-per-PR-scoping agent
behavior. Lifecycle override fields are rejected.

Optional websocket notification:

- `github.task_ci_options.updated`
- Payload: the same options response shape.
- `updated_at` is a task-wide, monotonic version of the complete payload. It advances atomically when task options or any per-PR automation state changes, so clients can discard equal or older events without losing current PR-state badges or errors.
- The event is emitted after a successful options or per-PR automation-state update so other open tabs refresh immediately and the backend can evaluate any currently linked PRs when automation is enabled.
