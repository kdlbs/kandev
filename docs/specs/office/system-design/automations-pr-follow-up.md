---
status: current
system: office
requirements:
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-002
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-003
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-005
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-007
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-008
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-009
created: 2026-11-23
owners:
  - Kandev
---

# GitHub PR Follow-up Automation System Design

## Purpose and boundaries

This design covers four additions:

- The `new_commits` and `review_requested` events on the `github_pr` poller.
- A per-PR continuation policy, with a follow-up prompt and a continuation
  step.
- End-of-PR actions on the task.

The office automation subsystem owns trigger evaluation, admission, run
binding, continuation, and automation task lifecycle, so it owns this design.
That subsystem is `internal/automation`, the automation handlers in
`internal/orchestrator`, and `apps/web/components/automations`.

Adjacent contracts used but not owned:

- `internal/github` supplies the workspace-scoped automation client, the `PR`
  model, and the PR-to-task association. This design adds one read method and
  PR fields.
- The workflow engine owns step moves and step auto-start. This design moves a
  task through the existing task service. It suppresses only the agent start,
  through the existing automation start-ownership marker.
- The task service owns archive and delete. This design calls the existing
  lifecycle methods.
- The [settings event-selection design](automations-settings-02.md#github-pr-event-selection)
  owns which event selections poll. This design owns which open PRs `opened`
  fires for (AC-001.10).
- The [continuity requirements](../requirements/automation-continuity.md) own
  `new_task`, `reuse_thread`, exact run binding, and coordinator authority.
  The per-PR policy reuses their binding and dispatch path.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001` | [PR listing](#pr-listing), [PR observation](#pr-observation), [Evaluator control flow](#evaluator-control-flow), [Persistence](#persistence) |
| `REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-002` | [PR listing](#pr-listing), [Trigger config, data, and placeholders](#trigger-config-data-and-placeholders) |
| `REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-003` | [Editor](#editor) |
| `REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004` | [Per-PR continuation policy](#per-pr-continuation-policy), [Busy check and admission deferral](#busy-check-and-admission-deferral), [Dispatch](#dispatch), [Export](#export) |
| `REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-005` | [Continuation step](#continuation-step), [Export](#export) |
| `REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006` | [Editor](#editor) |
| `REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-007` | [PR observation](#pr-observation), [Trigger config, data, and placeholders](#trigger-config-data-and-placeholders) |
| `REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-008` | [Follow-up prompt](#follow-up-prompt), [Export](#export) |
| `REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-009` | [End-of-PR actions](#end-of-pr-actions), [Export](#export) |

## Components and responsibilities

| Component | Responsibility |
| --- | --- |
| `github.Client.ListOpenPRs` | Lists every open PR of one repository with head SHA, base ref, labels, draft state, and requested reviewers and teams. |
| `github.Service.ListAutomationOpenPRs` and `GetAutomationPR` | Enforce workspace repository scope and resolve the automation client, as `ListAutomationPRs` does today. |
| `automation.GitHubEvaluator` | Lists PRs once per trigger and repository. Applies filters, compares observations, chooses at most one firing per PR, maintains observations, and runs end-of-PR actions. |
| `automation.Store` | Persists PR observations, PR-subject continuations, and the new automation fields. |
| `automation.Service` admission | Defers a per-PR firing whose PR task is busy. |
| `orchestrator` automation handlers | Resolve, create, or replace the PR task. Choose the prompt, move the task to the continuation step, dispatch the turn, and expose the busy check and end-of-PR task actions. |
| `GitHubPRConfig` and the automation editor sections | Edit events, the quiet period, reviewers, the policy, the follow-up prompt, the continuation step, and the end-of-PR actions. |

## PR listing

Add `ListOpenPRs(ctx, owner, repo string) (prs []*PR, complete bool, err error)`
to `github.Client`.

- `PATClient`, and the app and token clients built on it, call
  `GET /repos/{owner}/{repo}/pulls?state=open&per_page=100`. They follow
  `Link: rel="next"` until the last page. `complete` is false if a later page
  fails after earlier pages succeeded.
- `GHClient` calls `gh api repos/<owner>/<repo>/pulls?state=open&per_page=100
  --paginate --slurp` and converts the REST pages like `PATClient`. A failed
  page fails the call, so a successful result is always complete.
- `MockClient` returns its seeded open PRs ordered by number. `NoopClient`
  returns `ErrNoClient`, like its other reads.

`github.PR` gains `Labels []string`, copied by the PAT and `gh` conversions.
The existing `RequestedReviewers` field already carries users and teams. The
evaluator normalizes them to lowercased logins and `<owner>/<team-slug>` keys,
where the owner is the repository owner because requested teams belong to the
repository's organization.

The evaluator replaces `ListAutomationPRs` (issue search) with
`ListAutomationOpenPRs` for all events. Search results carry no head SHA, base
ref, labels, or reviewers. That is why base-branch filters never matched and
label filters were never applied. The request count stays at one listing per
trigger and repository per poll, plus one request for each extra page of 100
PRs.

Filters, in order:

1. `ExcludeDraft`.
2. `matchesBranches(pr.BaseBranch, cfg.Branches)`.
3. `matchesAuthors`.
4. A new `matchesLabels(pr.Labels, cfg.Labels)`. It returns true for an empty
   filter, or when any label in the filter equals a PR label exactly.

## Trigger config, data, and placeholders

`GitHubPRTriggerConfig` gains three fields:

- `QuietPeriodMinutes int` (`quiet_period_minutes`). Validated to 0–1440 in
  `AddTrigger` and `UpdateTrigger`.
- `Reviewers []string` (`reviewers`). Logins or `org/team-slug`, compared
  without case.
- `IncludeExistingPRs bool` (`include_existing_prs`). Off by default; when
  set, `opened` also fires for PRs that were open before the trigger existed.

The `Events` comment is updated to list `opened`, `new_commits`, and
`review_requested`. The registry default stays `{"events":["opened"]}`.

`firePRTrigger` adds these keys to the trigger data:

| Key | Value |
| --- | --- |
| `head_sha` | The PR's head SHA. |
| `previous_head_sha` | The observed head before the firing. Empty for `opened`. |
| `event` | `opened`, `new_commits`, or `review_requested`. |
| `requested_reviewers` | The newly requested matching reviewers. Empty for other events. |
| `pr_subject` | `<owner>/<repo>#<number>`. |

`prPlaceholders` in `interpolator.go` maps `pr.head_sha`,
`pr.previous_head_sha`, `pr.event`, and `pr.requested_reviewers` (joined with
`, `). `trigger_registry.go` adds these four to the `github_pr` placeholder
list. The `github_pr_merged` mapping is unchanged.

## PR observation

The table `automation_pr_observations` stores one row per trigger and open PR.
See [Persistence](#persistence). The evaluator uses it when the trigger selects
`new_commits` or `review_requested`.

Each row holds:

- `head_sha`.
- `requested` (sorted JSON list of lowercased logins and team keys).
- `review_generation`.
- `pending_review_generation`, the generation reserved for a review request
  before its firing is dispatched, and `pending_review_reviewers`, the
  reviewer keys that reservation covers.
- `pending_head_sha` and `pending_since`, which track the quiet period.

The evaluator computes candidates for each listed PR:

- **No row:** insert a baseline (current head and requested reviewers). No
  candidate.
- **`new_commits` candidate:** the event is selected and
  `pr.HeadSHA != row.head_sha`. With `QuietPeriodMinutes > 0`, the evaluator
  sets `pending_head_sha`/`pending_since` when the pending head differs from
  the current head. The candidate becomes ready only when
  `now - pending_since >= quiet period`.
- **`review_requested` candidate:** the event is selected and the set of
  matching reviewers in the current requested set, minus `row.requested`, is
  not empty.
- **`opened` candidate:** the event is selected, the PR matches the filters,
  no run holds `pr:o/r#n`, and the PR is eligible. Without
  `include_existing_prs`, a PR already observed under another event is not
  treated as new. The evaluator reads the rows for every selection, even when
  `opened` is the only selected event. With
  `include_existing_prs`, every PR is eligible. Otherwise a PR is eligible
  when its row, if any, tracks `opened`, and its `created_at` is zero or not
  before the trigger's `created_at`. A row that does not track `opened` means
  the PR was open before `opened` was selected. `trigger.update` keeps the
  trigger row, so editing a trigger does not move the cutoff.

The evaluator chooses at most one ready candidate per PR, in the order
`opened`, `new_commits`, `review_requested` (AC-001.7). It does not fire
candidates for a PR that misses the filters.

Row updates after the choice:

| Outcome | Row update |
| --- | --- |
| Filters missed, or no ready candidate | Store the current `requested` set, so removals are tracked and a later re-request counts as new. Set `head_sha` to the current head only when the filters missed. Otherwise keep any pending quiet-period state. |
| Firing admitted, `Duplicate`, or capacity `Skipped` | Set `head_sha` and `requested` to current, clear the pending state, and increase `review_generation` when the event is `review_requested`. |
| `Deferred`, or an error | No change. |

After a complete listing for a repository, delete that trigger's rows for PRs
that are missing from the listing. Deleting the trigger cascades to its rows.

Dedup keys:

| Event | Key |
| --- | --- |
| `opened` | `pr:o/r#n` (unchanged) |
| `new_commits` | `pr:o/r#n@<head_sha>` |
| `review_requested` | `pr:o/r#n/review-requested/<generation>` |

A review firing's generation is reserved before it fires: one more than the
larger of the row's `review_generation` and the highest generation already in
this automation's run dedup keys for the PR. A reopened PR, a recreated
trigger, or a second trigger with another reviewer filter therefore never
reuses a key that an earlier request holds. The evaluator stores the
reservation in `pending_review_generation` and dispatches only after that
write succeeds. A retry of the same transition reuses the reserved key, so a
run committed before its observation advanced deduplicates instead of firing
again. The key is reused only when every reviewer of the retry is in
`pending_review_reviewers`; a reviewer requested later gets a new generation
and dispatches. Processing any firing, or the transition disappearing, clears the
reservation.

Each row also stores `tracked_events`, the events the trigger selected when
the row was written, including `opened`. A row that tracks no event stores
`none`, because an empty value marks a legacy row that tracks every event.
When no follow-up event remains selected, the evaluator still records the
removal, so selecting the event again rebaselines it. When an event is added to the trigger, the
next poll rebaselines that event's state (head or requested set) instead of
firing for changes made before it was selected.

## Evaluator control flow

```text
for each enabled github_pr trigger
  decode config; ev = selected events among opened|new_commits|review_requested
  if ev empty or repos empty -> return
  for each repo
    prs, complete = ListAutomationOpenPRs(workspace, owner, repo)
    for each pr
      candidate = chooseCandidate(pr, row, ev, filters, now)   // may insert baseline
      if candidate -> result = fire(candidate)
      updateObservation(pr, row, candidate, result)
    if complete
      pruneObservations(trigger, repo, prs)
      runEndOfPRActions(automation, repo, prs)   // once per automation+repo per cycle
```

Split `chooseCandidate`, `updateObservation`, and `runEndOfPRActions` into
separate functions, in a new `evaluator_pr_observation.go` if needed, so the
Go complexity limits hold. Before `opened` calls `FireTrigger`, the evaluator
checks `HasRunWithDedupKey` for `pr:o/r#n`. A held key counts as a duplicate
without calling admission, so steady-state polling writes no duplicate-skip
audit row and logs no unique-key warning.

## Per-PR continuation policy

Add `ContinuationPolicyReusePerPR ContinuationPolicy = "reuse_per_pr"`.

Validation in `automation.Service`:

- Accept all three values. `reuse_per_pr` does not constrain
  `max_concurrent_runs`.
- Reject `reuse_per_pr` with `task_mode = managed_conversation`.
- Reject a non-empty `follow_up_prompt` or `pr_continuation_step_id`, and any
  `pr_merged_action` or `pr_closed_action` other than `keep`, unless the
  policy is `reuse_per_pr`.
- Do not require a `github_pr` trigger. Firings without `pr_subject` use
  `new_task` behavior.

The MCP automation schema in `config_automation_handlers.go` adds the enum
value and the four new fields. `RetryLaunchConfigSnapshot` copies the four new
fields. The continuation for a PR subject lives in
`automation_pr_continuations`, keyed by `(automation_id, pr_subject)`. The
automation's `continuation_task_id` column is not used by this policy.

## Busy check and admission deferral

`automation.Service` gains an injected `PRTaskController` interface
(`SetPRTaskController`), implemented by the orchestrator and wired in
`backendapp`. Its `AutomationPRTaskBusy` returns true when any session of the
task is `CREATED`, `STARTING`, or `RUNNING`. The same interface exposes
`ArchiveAutomationPRTask` and `DeleteAutomationPRTask` for end-of-PR actions.

Inside `admitTriggerLocked`, after the dedup check and before the capacity
check:

```text
if policy == reuse_per_pr && triggerType == github_pr && subject != ""
  taskID = store.GetPRContinuation(automationID, subject)
  if taskID != "" && (store.HasActiveRunForTask(automationID, taskID) || busy(taskID))
    return deferred
```

`HasActiveRunForTask` uses the `CountActiveRuns` predicate: status
`task_created` and the task still open. An agent waiting for a user answer is
still inside its turn, so its run is active. `FireResult` gains
`Deferred bool`. A deferred firing creates no run row, records no skipped run,
and does not consume the dedup key. A checker error counts as busy, so the
firing is retried. Log it at warning level.

## Dispatch

`prepareAutomationTask` in `orchestrator/event_handlers_automation.go` gains a
per-PR branch. It is implemented in a new file,
`orchestrator/automation_pr_continuation.go`:

```text
subject = triggerData.pr_subject
if policy != reuse_per_pr || triggerType != github_pr || subject == ""
  -> existing new_task path
taskID = continuations.Get(automationID, subject)
if taskID == ""                                   -> create, map, action=created
task = repo.GetTask(taskID)
if missing / archived / incompatible              -> create, map, action=replaced
session = GetPrimarySessionByTaskID (never a newer secondary tab)
if session ready (WAITING_FOR_INPUT, IDLE)        -> continue, action=resumed
if session running or starting                    -> fail run "PR task is busy"
otherwise (completed, failed, cancelled, none)    -> create, map, action=replaced
```

A task or session read failure other than "not found" fails the run instead of
creating a replacement. The compatibility check reuses
`continuationTaskIncompatibility`. For this
policy only, it matches the workflow ID and not the start step. Repository
compatibility uses the PR's repository from
`resolveGitHubPRTriggerRepository`.

Creating a task follows the existing `CreateReviewTask` path:
`AutomationStartOwned`, `associateAutomationPR`, and auto-start in the
configured start step (AC-005.5). After creation, the dispatcher upserts the
mapping. If the upsert fails, it deletes the task and fails the run, as the
`reuse_thread` path does for `SetContinuationTaskID`. Continuing a task calls
`dispatchAutomationContinuation`, which binds the run to the exact turn.

## Follow-up prompt

Add `FollowUpPrompt string` (`follow_up_prompt`, column
`follow_up_prompt TEXT NOT NULL DEFAULT ''`).

`createAutomationTaskLocked` renders the main prompt as it does today, and
renders the follow-up prompt with `InterpolateAgentPrompt` when it is
non-empty. The per-PR branch returns the chosen action. When the action is
`resumed` and the follow-up prompt is non-empty, the dispatcher sends the
follow-up prompt. Otherwise it sends the main prompt.

The retry snapshot records both resolved prompts. A retry selects between them
by the same rule (AC-004.9).

## Continuation step

Add `PRContinuationStepID string` (`pr_continuation_step_id`, column
`pr_continuation_step_id TEXT NOT NULL DEFAULT ''`).

A non-empty value requires `reuse_per_pr`, `task_mode = normal_task`, and a
step in the automation's workflow, checked with
`authorizeWorkflowStepOwnership`. An update that leaves a step that no longer
applies is rejected. The editor clears the field first.

Before `dispatchAutomationContinuation`, when the field is set and the task is
in another step:

1. Move the task with the task service `MoveTaskWithOptions`, keeping its
   workflow, with one-shot `EntryOptions{SkipStepPrompt: true}`. A continued
   task always has a session, so the move enters the step through the
   existing-session lifecycle; the entry option suppresses the step's prompt
   and starts no turn, as the coordinator's undo move does.
2. On an error or a missing step, mark the run failed with
   `continuation step unavailable: <reason>` and stop.
3. Dispatch the turn.

The step's auto-start prompt does not run (AC-005.3). The step's other
on-enter actions, such as resetting the agent context or switching the agent
profile, still apply because they are part of the step the owner chose.

## End-of-PR actions

Add `PRMergedAction` and `PRClosedAction` (`pr_merged_action`,
`pr_closed_action`, columns `TEXT NOT NULL DEFAULT 'keep'`), with values
`keep`, `archive`, and `delete`.

`runEndOfPRActions` runs after a complete listing, once per automation and
repository per evaluation cycle, for `reuse_per_pr` automations:

```text
for each mapping (automationID, subject in this repo) whose PR is not in the listing
  pr = GetAutomationPR(owner, repo, number)        // one read per ended PR
  if read fails or pr.State == open -> skip (retry next poll)
  reload the automation; skip unless it is still enabled and reuse_per_pr
  action = merged ? PRMergedAction : PRClosedAction
  if HasActiveRunForTask || busy(task) -> skip (retry next poll), for every action including keep
  if action != keep
    archive: task service ArchiveTask; delete: DeleteTaskWithLifecycleAndReason(reason="automation: PR merged|closed")
    an already archived or missing task counts as done, so delete never removes a task a person archived
  delete mapping
```

The orchestrator exposes `ArchiveAutomationPRTask` and
`DeleteAutomationPRTask` through `PRTaskController`. This keeps
`internal/automation` free of task-service imports. Archive and delete reuse the task service's
existing worktree and session teardown.

## Editor

Desktop and phone share one composition. Controls stack vertically, and every
control uses an existing primitive.

- `GitHubPRConfig.PR_EVENTS` lists `opened`, `new_commits`, and
  `review_requested`. When `new_commits` is checked, an `Input type=number`
  (min 0, max 1440) appears for `quiet_period_minutes`. When
  `review_requested` is checked, an `Input` appears for `reviewers` and is
  parsed on blur like the authors field. When `opened` is checked, an
  **Also run on pull requests already open** checkbox binds
  `include_existing_prs`.
- The **Task for each run** `SegmentedChoice` shows the third option
  (`reuse_per_pr`) when the form has a `github_pr` trigger or the saved policy
  is `reuse_per_pr`. Selecting it does not change `maxConcurrentRuns`. The
  choice sits in the Repeat runs card, below the workflow whose steps it lists
  and above the prompts it affects.
- With `reuse_per_pr`, the following appear:
  - Under the choice, two `Select`s for the end-of-PR actions, side by side
    from the `sm` breakpoint. The delete warning is shown under a select when
    that select's value is `delete`.
  - With the normal-task target only, the **Move to step when continuing**
    `Select`. It lists `listWorkflowSteps(form.workflowId)` plus the
    `Stay in current step` sentinel, which maps to `""`.
  - Directly under the main instructions, a second `PromptSection` labeled
    **Follow-up instructions**. Both prompts share one `PlaceholderList`.
- The policy, workflow, and target-mode change handlers clear values that no
  longer apply. `FormState`, `automation-payload.ts`, and
  `lib/types/automation.ts` carry the new fields.
- New copy goes in every `src/locales/*/automations.json` catalog.

## Export

- `exportAutomation` gains `FollowUpPrompt *yaml.Node yaml:"follow_up_prompt,omitempty"`
  after `prompt`, encoded with the prompt node rules.
- It gains `PRMergedAction` and `PRClosedAction`, emitted only when the
  policy is `reuse_per_pr`, after `continuation_policy`.
- `exportWorkflow` gains `ContinuationStep yaml:"continuation_step,omitempty"`
  after `step`.
- `continuation_policy` exports `reuse_per_pr` as stored.
- Trigger config keys export through the existing config node table.

## Failure and recovery

| Condition | Behavior |
| --- | --- |
| Listing error for a repository | Log at debug level. No fire, no row change, no prune, no end-of-PR action. Other repositories continue. |
| Partial listing | Fire for the PRs that were read and update their rows. Skip pruning and end-of-PR actions. |
| `FireTrigger` error | No row change. The next poll retries. |
| PR task busy at admission | Defer. The observation is unchanged, so it is retried each poll. |
| PR task becomes busy before dispatch | The run fails visibly. The observation has already advanced, so the next change fires again. |
| Continuation step missing or move failure | The run fails without a prompt. The mapping is kept. |
| End-of-PR read, archive, or delete failure | Keep the mapping and retry on the next complete poll. |
| Mapping upsert fails after task creation | Delete the new task and fail the run. |
| Restart | Observations, pending quiet periods, and mappings persist. |
| Restart after a continuation's launch was reserved | Retry recovery resends the snapshot's resolved follow-up prompt for a per-PR continuation, as the original firing chose. |

## Persistence

`automation.Store` schema initialization creates the new tables for SQLite and
Postgres through `schemaSQLForDriver`:

```sql
CREATE TABLE IF NOT EXISTS automation_pr_observations (
  trigger_id        TEXT NOT NULL REFERENCES automation_triggers(id) ON DELETE CASCADE,
  repo_owner        TEXT NOT NULL,
  repo_name         TEXT NOT NULL,
  pr_number         INTEGER NOT NULL,
  head_sha          TEXT NOT NULL,
  requested         TEXT NOT NULL DEFAULT '[]',
  review_generation BIGINT NOT NULL DEFAULT 0,
  pending_review_generation BIGINT NOT NULL DEFAULT 0,
  pending_review_reviewers  TEXT NOT NULL DEFAULT '[]',
  pending_head_sha  TEXT NOT NULL DEFAULT '',
  pending_since     TIMESTAMP NULL,
  tracked_events    TEXT NOT NULL DEFAULT '',
  updated_at        TIMESTAMP NOT NULL,
  PRIMARY KEY (trigger_id, repo_owner, repo_name, pr_number)
);

CREATE TABLE IF NOT EXISTS automation_pr_continuations (
  automation_id TEXT NOT NULL REFERENCES automations(id) ON DELETE CASCADE,
  pr_subject    TEXT NOT NULL,
  repo_owner    TEXT NOT NULL,
  repo_name     TEXT NOT NULL,
  pr_number     INTEGER NOT NULL,
  task_id       TEXT NOT NULL,
  created_at    TIMESTAMP NOT NULL,
  updated_at    TIMESTAMP NOT NULL,
  PRIMARY KEY (automation_id, pr_subject)
);
```

The `automations` columns `follow_up_prompt`, `pr_continuation_step_id`,
`pr_merged_action`, and `pr_closed_action`, and
`automation_pr_observations.tracked_events`, are added through the existing
additive `ALTER TABLE` list. Both tables are registered as required automation
tables in `internal/persistence/requiredstores`. Run worktree retention
(`PrunableRunTaskIDs`, `RunWorkspaceInUse`) never reclaims a task that is
mapped to an open PR. No data migration is needed. Deleting a task does
not cascade to mappings. A stale mapping is replaced at dispatch, or removed
by the end-of-PR pass.

## Security

The listing and the single-PR read use the workspace automation identity and
the existing repository scope check. Head SHAs, labels, and reviewer logins
are public PR metadata. The continuation step is validated against the
automation's own workflow. End-of-PR actions only touch tasks in the
automation's own mapping table, never arbitrary tasks.

## Observability

- Debug log per trigger and repository: PR count, listing completeness,
  baselines recorded, pending quiet periods, and firings per event.
- Info log for each deferral and each end-of-PR action, with automation ID, PR
  subject, task ID, and the action.
- The run's `thread_action` and `thread_reason` record `created`, `resumed`,
  or `replaced`. A busy task or a step failure appears as the run's error
  message.
