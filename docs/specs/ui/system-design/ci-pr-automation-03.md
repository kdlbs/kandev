---
status: draft
system: ui
requirements:
  - REQ-UI-CI-PR-AUTOMATION-001
created: 2026-06-18
owners:
  - tbd
---
# Task PR Automation Controls System Design Part 3

## Purpose and boundaries

This design preserves the technical source detail for `REQ-UI-CI-PR-AUTOMATION-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-CI-PR-AUTOMATION-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

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
- **GIVEN** one, two, or all three lifecycle prompt options are enabled for a
  task with a rendered PR status chip, **WHEN** the user views the tray above
  the chat input, **THEN** one badge reads `PR events 1/3`, `PR events 2/3`, or
  `PR events 3/3`, respectively, and the chip's accessible description names
  the enabled lifecycle events.
- **GIVEN** all three lifecycle prompt options are disabled, **WHEN** the user
  views the PR status chip above the chat input, **THEN** no PR events badge is
  rendered.
- **GIVEN** auto-fix, auto-merge, and all three lifecycle prompt options are
  enabled alongside other composer-tray controls, **WHEN** the task is viewed
  on a phone or narrow window, **THEN** every complete control remains visible
  or wraps within the tray and the document has no horizontal overflow.
- **GIVEN** auto-fix has already used 10 rounds for a PR and no pending auto-fix queue entry exists, **WHEN** new actionable feedback appears, **THEN** Kandev does not send or queue another prompt and records the PR as paused at `Auto-fix 10/10`.
- **GIVEN** auto-fix has already used 10 rounds for a PR and the 10th round is still queued, **WHEN** new actionable feedback appears, **THEN** Kandev replaces that pending queued prompt without incrementing the round count.
- **GIVEN** auto-fix sends a prompt for feedback that the backend considered prompt-worthy but the agent determines is already addressed or otherwise non-actionable, **WHEN** the agent reviews that prompt, **THEN** the agent does not modify files, commit, or push and only reports that there is nothing actionable to address.
- **GIVEN** auto-fix is enabled and the task session is running, **WHEN** new actionable PR feedback appears, **THEN** the prompt is queued and delivered after the current turn rather than interrupting the running session, and the chat history shows the `@ci-auto-fix` user message with visible PR snapshot details before the agent output for the queued turn.
- **GIVEN** a linked draft PR has passing checks and GitHub reports clean mergeability, **WHEN** Kandev refreshes its status, **THEN** PR status surfaces identify it as a draft and do not present it as ready to merge.
- **GIVEN** auto-merge is enabled and the PR has passing checks, required reviews, no unresolved threads, and clean mergeability, **WHEN** the PR watch poll observes the ready state, **THEN** Kandev merges the PR with the existing backend merge-method selection.
- **GIVEN** auto-merge is enabled but the PR is a draft or has requested changes, pending required review, failing checks, unresolved threads, or dirty mergeability, **WHEN** the PR watch poll observes the state, **THEN** Kandev does not merge.
- **GIVEN** auto-merge attempted a ready-state merge and GitHub rejected it, **WHEN** the same ready state is observed again, **THEN** Kandev does not retry until the readiness signature changes.
- **GIVEN** a task has two open linked PRs, **WHEN** the user enables an
  automation control from the first PR's tab, **THEN** only that PR becomes
  eligible for automation; the second PR's tab shows the control unchanged,
  and each PR records its own last-fix and last-merge state.
- **GIVEN** a task has two linked PRs in different repositories with the same
  PR number, **WHEN** the user enables a switch for one, **THEN** the other's
  switch is unaffected, because switch identity is
  `(task_id, repository_id, pr_number)`.
- **GIVEN** a task has two linked PRs, **WHEN** an agent calls
  `update_task_pr_automation_kandev` naming one PR's `repository_id` and
  `pr_number`, **THEN** only that PR's switches change.
- **GIVEN** a task has two linked PRs, **WHEN** an agent calls
  `update_task_pr_automation_kandev` without `repository_id`/`pr_number`,
  **THEN** the requested switch changes on every currently linked PR.
- **GIVEN** a task has two linked PRs, **WHEN** the user disables auto-fix on
  one PR while it stays enabled on the other, **THEN** re-enabling it on the
  first PR resets only that PR's `auto_fix_round_count`, exhaustion, and fix
  checkpoint; the second PR's checkpoint is unchanged.
- **GIVEN** a task's automation was configured before per-PR scoping shipped,
  **WHEN** the backend boots for the first time after upgrade, **THEN** every
  PR linked to that task at boot time inherits the legacy task-level values,
  and a PR linked to the task afterward starts with all five switches off.
- **GIVEN** a task's automation was already migrated to per-PR scoping and a
  user has since turned a switch off for one PR, **WHEN** the backend boots
  again, **THEN** that PR's switch remains off — the migration never replays.
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
- Per-PR auto-fix **prompt overrides**. `auto_fix_prompt_override` remains
  task-level; only the five boolean switches are per-PR.
- Per-user automation preferences.
- Merge-method selection UI. Auto-merge uses the existing backend default merge-method selection.
- Team-level review-request matching. The first version tracks the
  authenticated user-level request.
- Creating a replacement task session when the existing task has no promptable
  session.
- Configuring lifecycle prompt text. The lifecycle templates are intentionally
  immutable and server-owned; only the three per-PR booleans are exposed.
- Streaming CI logs into the chat or popover.
- Editing GitHub branch protection, review rules, or workflow files directly from the automation controls.
- GitLab merge request automation.
- Changing when terminal-only PR associations hide the PR status chip. Existing
  merged/closed banners continue to own that tray state.

## Open questions

- None.
