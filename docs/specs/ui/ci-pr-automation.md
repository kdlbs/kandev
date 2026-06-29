---
status: draft
created: 2026-06-18
owner: tbd
---

# CI PR Automation Controls

## Why

Users can already see pull request CI/review status above the task chat input, but acting on a red PR still requires repeatedly noticing the failure, prompting the agent, and deciding when it is safe to merge. Users need per-task controls that let Kandev keep a watched PR moving: ask the agent to fix new CI/review feedback, and merge only when the PR is ready.

## What

- The PR CI popover above the chat input shows two task-level automation controls:
  - `Auto-fix CI & address comments`
  - `Auto-merge when ready`
- The automation section includes an info icon or equivalent help affordance that explains what each control watches, how often Kandev checks watched PRs, how feedback snapshots prevent duplicate prompts, and how auto-merge decides readiness.
- The same controls are available anywhere the task PR CI popover is rendered, including the normal chat input status bar and passthrough toolbar surfaces.
- `Auto-fix CI & address comments` causes Kandev to send or queue an agent prompt when a linked PR gets actionable CI or review feedback.
- `Auto-merge when ready` causes Kandev to merge a linked PR only when the PR is open, checks are passing, review requirements are satisfied, unresolved review threads are cleared, and the PR is cleanly mergeable.
- The auto-fix prompt is customizable per task from the PR CI popover.
- The per-task prompt editor is opened from an edit button in the automation section.
- The per-task prompt editor links to Settings > Prompts so the user can edit the default `ci-auto-fix` prompt.
- The per-task prompt editor explains that `{{pr.feedback}}` is the placeholder that inserts Kandev's PR feedback snapshot. The explanation lists the included data: PR identifier, new or changed failing checks with job links, and new or changed review comments with file, line, and body text.
- Omitting `{{pr.feedback}}` from the prompt means Kandev still evaluates PR feedback for dedupe and trigger decisions, but it does not include the PR snapshot in the agent message. This supports prompts that tell the agent to pull/fetch the branch and inspect GitHub itself.
- If a task has no custom auto-fix prompt, Kandev uses a built-in default prompt named `ci-auto-fix`.
- The default `ci-auto-fix` prompt is editable from Settings > Prompts like other built-in prompts.
- Emptying or resetting the task prompt override returns the task to the default `ci-auto-fix` prompt.
- For tasks with multiple linked PRs, the controls are task-level and apply to every open linked PR. Dedupe, last-attempt, and error state are tracked per linked PR.
- Kandev checks watched PRs through the existing lightweight PR watch poller, which runs once per minute. Automation wakeups sync the latest lightweight PR state before evaluating gates. When auto-fix is enabled, Kandev fetches full PR feedback so failing checks, requested changes, unresolved threads, and plain PR comments can trigger deduped prompts even when the persisted lightweight row was stale.
- Saving CI automation options while `Auto-fix CI & address comments` or `Auto-merge when ready` is enabled immediately evaluates the task's current linked PRs instead of waiting for the next PR watch poll. This includes prompt edits made while automation is already enabled; unchanged feedback is still deduped by the per-PR checkpoint.
- Every auto-fix attempt records the latest feedback snapshot it used, including non-actionable snapshots that were intentionally no-ops. Later fix rounds include only new or materially changed CI/review feedback since the last recorded round, with enough summary context for the agent to understand the PR.
- Automation must not repeatedly prompt for the same failure/comment snapshot or repeatedly retry the same failed merge attempt on every poll.
- When auto-fix is enabled and the task session is busy, Kandev keeps at most one pending CI auto-fix queue entry per task/repository/PR. Newer feedback replaces that pending entry instead of appending another queued `@ci-auto-fix` message.
- Auto-fix is capped at 10 accepted rounds per task/repository/PR. A round is counted when Kandev sends a prompt directly or inserts a new queued auto-fix prompt. Replacing an already queued auto-fix prompt does not count as another round.
- The auto-fix enabled chip above the chat input shows round progress as `Auto-fix N/10`; exhausted or capped PRs show `Auto-fix 10/10` with warning/paused styling.
- Hovering the chip on desktop, or opening the same PR CI drawer on mobile, explains in plain language how many rounds have been used, what counts as a round, that queue replacement does not count again, and that Kandev pauses at 10/10 to prevent loops.
- Automation controls persist across Kandev restarts.

## Data model

`github_task_ci_options`

- `task_id` string, primary key. References the Kandev task that owns the controls.
- `auto_fix_enabled` boolean, default `false`.
- `auto_merge_enabled` boolean, default `false`.
- `auto_fix_prompt_override` string nullable. `NULL` or empty means use the default `ci-auto-fix` prompt.
- `created_at` timestamp.
- `updated_at` timestamp.

`github_task_ci_pr_state`

- Primary key: `task_id`, `repository_id`, `pr_number`.
- `task_id` string. References the Kandev task.
- `repository_id` string. Identifies which linked repository/branch row produced the PR.
- `pr_number` integer.
- `last_fix_signature` string nullable. Deterministic hash of the latest feedback snapshot, actionable or non-actionable, that produced an auto-fix prompt.
- `last_fix_checkpoint_json` string nullable. JSON snapshot of feedback used in the last fix round. This includes non-actionable no-op snapshots so identical bot summaries/status updates are not repeatedly sent.
- `last_fix_enqueued_at` timestamp nullable.
- `last_fix_session_id` string nullable.
- `auto_fix_round_count` integer, default `0`. Counts accepted auto-fix rounds for this task/repository/PR.
- `auto_fix_exhausted_at` timestamp nullable. Set when Kandev pauses auto-fix after the 10-round cap.
- `last_merge_signature` string nullable. Deterministic hash of the last readiness state used for a merge attempt.
- `last_merge_attempt_at` timestamp nullable.
- `last_error` string nullable. Latest user-visible automation error for this task/PR pair.
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
  "auto_fix_prompt_override": null,
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
  "auto_fix_prompt_override": "Use this task-specific prompt..."
}
```

`auto_fix_prompt_override: null` or an empty string clears the task override. The response shape matches `GET`.

Optional websocket notification:

- `github.task_ci_options.updated`
- Payload: the same options response shape.
- The event is emitted after a successful options update so other open tabs refresh immediately and the backend can evaluate any currently linked PRs when automation is enabled.

## State machine

Task CI automation options:

- `disabled`: both toggles are false. PR watch events update UI only.
- `auto_fix_enabled`: Kandev evaluates actionable PR feedback immediately when enabled, when CI automation options are saved while it remains enabled, and on later PR watch events.
- `auto_merge_enabled`: Kandev evaluates PR merge readiness immediately when enabled, when CI automation options are saved while it remains enabled, and on later PR watch events.
- `both_enabled`: Kandev evaluates both paths. Auto-fix does not merge; auto-merge merges only after readiness conditions are satisfied.

Auto-fix cycle for one task/PR:

1. Existing PR watch poll, PR feedback event, or CI options save wakes automation.
2. Kandev syncs the latest lightweight PR state for the task's linked PRs, including linked PR rows that do not currently have an active watch.
3. Kandev fetches full PR feedback.
4. Kandev compares the current feedback snapshot to `last_fix_checkpoint_json` and `last_fix_signature`.
5. If there is no material change, the cycle ends without prompting.
6. If there is new or materially changed feedback, Kandev renders the task override or default `ci-auto-fix` prompt and sends or queues it for the task session. The saved/shared `ci-auto-fix` instructions are hidden system context. If the rendered prompt contains `{{pr.feedback}}`, Kandev replaces it with visible PR snapshot details after `@ci-auto-fix`, before the agent output for that automation turn. If the placeholder is absent, no PR snapshot is included in the chat message.
7. The default prompt instructs the agent to classify the new feedback before editing. If the
   new feedback is only summaries, status updates, no-finding reports, duplicated or already
   addressed comments, rate-limit notices, or other non-actionable review diagnostics, the agent
   must not modify files, commit, or push; it should only report that there is nothing actionable
   to address. When the agent addresses actionable PR review comments, the default prompt instructs
   it to reply with a fix summary and resolve the addressed PR review threads so they do not keep
   the PR blocked.
8. Once the prompt is queued or accepted by the agent runtime, Kandev records the new signature/checkpoint and attempt metadata for the latest feedback snapshot, actionable or non-actionable, so identical snapshots are not sent repeatedly while the agent is still working.
9. If the task session is busy and a pending auto-fix entry for this task/repository/PR already exists, Kandev replaces that queued entry with the latest rendered prompt instead of appending a second queued message. The round count is unchanged.
10. If a new prompt would require an 11th accepted auto-fix round for the same task/repository/PR, Kandev does not send or queue the prompt. It records a paused error and keeps the chip visible as `Auto-fix 10/10`. Disabling and re-enabling auto-fix resets the round count and paused state for the task's PR automation rows.

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

| Dependency / invariant | Behavior |
|---|---|
| GitHub auth is missing or invalid | Controls remain visible but saving/enabling or automation execution surfaces an error; no auto-fix prompt or merge is attempted. |
| PR is closed or merged | Controls are disabled for that PR; no automation runs. |
| Full PR feedback fetch fails | Auto-fix does not prompt; per-PR automation state records the error and the next materially changed lightweight status may retry. |
| Task has no promptable session | Auto-fix records an error instead of creating a surprising new session. |
| Task session is busy | Auto-fix queues the rendered prompt with workflow/automation metadata for later delivery; the visible `@ci-auto-fix` chat message, including PR snapshot details, is created when the queued prompt is delivered and before the agent's response for that turn. |
| Task session is busy and a pending auto-fix already exists for that PR | Kandev replaces the pending queued prompt with the latest feedback snapshot; it does not append a second queued message or increment the round count. |
| Same feedback snapshot repeats | Auto-fix does not send another prompt. |
| Auto-fix reaches 10 rounds for a PR | Kandev pauses auto-fix for that task/repository/PR, records a visible error, and does not create an 11th round. |
| GitHub merge fails | Auto-merge records the error and does not retry until the readiness signature changes. |
| Default prompt row is missing | Backend falls back to the embedded `ci-auto-fix.md` content. |
| Kandev restarts while an automation prompt is queued | Queued message and automation options/checkpoints persist according to the existing message queue and new CI automation tables. |

## Persistence guarantees

- Task CI options persist until the task or its automation options row is deleted.
- Per-PR automation state persists across restarts so duplicate prompts and merge retries do not resume after restart.
- The default prompt row persists in `custom_prompts`; user edits are not overwritten by reseeding.
- The existing 1-minute PR poller cadence, 30-second lightweight PR status cache, and 8-second full PR feedback cache remain cache behavior, not user-visible persistence guarantees.
- In-memory singleflight/cache state does not survive restart and must not be required for dedupe correctness.

## Scenarios

- **GIVEN** a task with one open linked PR, **WHEN** the user opens the CI popover above the chat input, **THEN** the popover shows the current CI/review summary and the two automation controls.
- **GIVEN** a user is viewing the CI popover automation controls, **WHEN** they activate the info icon, **THEN** they see help text explaining that Kandev uses the existing 1-minute PR watch checks, fetches full feedback only for candidate PRs, snapshots each auto-fix round, and merges only when readiness gates pass.
- **GIVEN** a task with one open linked PR, **WHEN** the user enables `Auto-fix CI & address comments`, **THEN** the setting persists and remains enabled after page reload.
- **GIVEN** a task with one open linked PR, **WHEN** the user enables `Auto-merge when ready`, **THEN** the setting persists and remains enabled after page reload.
- **GIVEN** a task using the default auto-fix prompt, **WHEN** the user edits the prompt from the CI popover, **THEN** only that task uses the custom prompt and Settings > Prompts continues to hold the global default.
- **GIVEN** the task prompt editor is open, **WHEN** the user follows the default-prompt settings link, **THEN** Kandev opens Settings > Prompts where the `ci-auto-fix` default can be edited.
- **GIVEN** a task with a custom auto-fix prompt, **WHEN** the user resets the prompt override, **THEN** the task uses the current default `ci-auto-fix` prompt.
- **GIVEN** the default `ci-auto-fix` prompt is edited in Settings > Prompts, **WHEN** a task without an override later auto-fixes a PR, **THEN** the rendered prompt uses the edited default content.
- **GIVEN** auto-fix is enabled and a watched PR transitions from passing to failing CI, **WHEN** the 1-minute PR watch poll observes the failure, **THEN** Kandev fetches full PR feedback and sends or queues one auto-fix prompt for that failure snapshot.
- **GIVEN** auto-fix already prompted for a failure snapshot, **WHEN** the same failure is observed again on a later poll, **THEN** no duplicate prompt is sent.
- **GIVEN** auto-fix already prompted for a failure snapshot, **WHEN** a new failed check or new unresolved review comment appears, **THEN** Kandev sends or queues a new prompt containing the new or materially changed feedback.
- **GIVEN** auto-fix is enabled and the task session is running, **WHEN** changed CI feedback appears multiple times for the same PR before the queue drains, **THEN** Kandev keeps one queued `@ci-auto-fix` entry for that PR and updates it with the latest feedback.
- **GIVEN** auto-fix has used 1 of 10 rounds for a PR, **WHEN** the user views the auto-fix chip above the chat input, **THEN** the chip reads `Auto-fix 1/10` and the hover/drawer explanation states that one round out of ten has been used.
- **GIVEN** auto-fix has already used 10 rounds for a PR and no pending auto-fix queue entry exists, **WHEN** new actionable feedback appears, **THEN** Kandev does not send or queue another prompt and records the PR as paused at `Auto-fix 10/10`.
- **GIVEN** auto-fix has already used 10 rounds for a PR and the 10th round is still queued, **WHEN** new actionable feedback appears, **THEN** Kandev replaces that pending queued prompt without incrementing the round count.
- **GIVEN** auto-fix sends a prompt for a snapshot that contains only non-actionable automation summaries or status updates, **WHEN** the agent reviews that prompt, **THEN** the agent does not modify files, commit, or push and only reports that there is nothing actionable to address.
- **GIVEN** auto-fix is enabled and the task session is running, **WHEN** new actionable PR feedback appears, **THEN** the prompt is queued and delivered after the current turn rather than interrupting the running session, and the chat history shows the `@ci-auto-fix` user message with visible PR snapshot details before the agent output for the queued turn.
- **GIVEN** auto-merge is enabled and the PR has passing checks, required reviews, no unresolved threads, and clean mergeability, **WHEN** the PR watch poll observes the ready state, **THEN** Kandev merges the PR with the existing backend merge-method selection.
- **GIVEN** auto-merge is enabled but the PR has requested changes, pending required review, failing checks, unresolved threads, or dirty mergeability, **WHEN** the PR watch poll observes the state, **THEN** Kandev does not merge.
- **GIVEN** auto-merge attempted a ready-state merge and GitHub rejected it, **WHEN** the same ready state is observed again, **THEN** Kandev does not retry until the readiness signature changes.
- **GIVEN** a task has two open linked PRs, **WHEN** the user enables either automation control, **THEN** both PRs are eligible for automation and each PR records its own last-fix and last-merge state.
- **GIVEN** the user is on mobile, **WHEN** they open the PR CI drawer, **THEN** the automation controls and prompt editor are usable without text overflow or overlapping controls.
- **GIVEN** the task is shown in a passthrough toolbar surface, **WHEN** the user opens the PR CI popover/drawer, **THEN** the same automation controls are available.

## Out of scope

- Webhook-based GitHub event ingestion. This feature uses the existing PR watch poller.
- Changing the global PR watch poll interval.
- Per-PR automation toggles in the first version.
- Per-user automation preferences.
- Merge-method selection UI. Auto-merge uses the existing backend default merge-method selection.
- Creating a new task or new task session when no promptable session exists.
- Streaming CI logs into the chat or popover.
- Editing GitHub branch protection, review rules, or workflow files directly from the automation controls.
- GitLab merge request automation.

## Open questions

- None.
