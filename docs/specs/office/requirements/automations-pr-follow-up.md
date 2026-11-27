---
status: active
system: office
created: 2026-11-23
owners:
  - Kandev
---

# GitHub PR Follow-up Automation Requirements

## Overview

A `github_pr` automation could start a review when a pull request opened, but
not follow up on pushes or review requests. This capability adds two events:

- **New commits**, with an optional quiet period.
- **Review requested.**

It also adds a continuation policy that keeps one task per pull request. A
follow-up continues the task that handled the pull request earlier, can use its
own prompt, and can move the task to a chosen workflow step. When the pull
request is merged or closed, Kandev can archive or delete that task.

The office system owns triggers, admission, continuation, and task lifecycle.
GitHub only supplies pull-request reads.

Source: [issue #5464](https://github.com/kdlbs/kandev/issues/5464).

## Terminology

- **New-commits event:** The `new_commits` value in a `github_pr` trigger's
  `events` list. It fires when the head commit of an open pull request changes.
- **Review-requested event:** The `review_requested` value. It fires when a
  matching reviewer is newly requested on an open pull request.
- **PR observation:** What Kandev last processed for one trigger and one open
  pull request: the head SHA and the set of requested reviewers.
- **Baseline:** The PR observation that Kandev records the first time a
  trigger sees an open pull request. Recording a baseline never fires.
- **Quiet period:** An optional number of minutes during which a new head must
  stay unchanged before the new-commits event fires.
- **PR subject:** The pull request a `github_pr` firing is about, identified as
  `<owner>/<repo>#<number>`.
- **Per-PR policy:** The `reuse_per_pr` continuation policy, which keeps one
  task per PR subject for an automation.
- **PR task:** The task currently mapped to a PR subject under the per-PR
  policy.
- **Busy PR task:** A PR task with an open automation run or a running or
  starting session.
- **Follow-up prompt:** The optional prompt used when a firing continues an
  existing PR task.
- **Continuation step:** The optional workflow step that a visible PR task
  moves to before a follow-up turn starts.
- **End-of-PR action:** What Kandev does to a PR task when its pull request is
  merged, or closed without merging: `keep`, `archive`, or `delete`.

## Requirements

### REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001: Fire once for each new PR head

**Intent:** React once to each settled head of an open pull request, without a
burst of firings for pull requests that were already open.

**User story:** As a maintainer, I want an automation to fire when a
contributor pushes to an open pull request, so that an automated reviewer can
review the new changes without manual restarts.

#### Acceptance criteria

- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.1:** When a `github_pr` trigger's
  saved `events` list contains the exact value `opened`, `new_commits`, or
  `review_requested`, the system shall poll the trigger's explicit
  repositories for open pull requests. Otherwise it shall make no provider
  read, run, or dedup reservation.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.2:** When a trigger first sees an
  open pull request, the system shall record the baseline and shall not fire
  `new_commits` or `review_requested` for it. This also applies when either
  event is added to an existing trigger. Stored observations survive a
  restart.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.3:** When a matching open pull
  request's head differs from its observed head and the quiet period is zero,
  the system shall fire `new_commits` once, with dedup key
  `pr:<owner>/<repo>#<number>@<head_sha>`. When the head is unchanged, the
  system shall neither fire nor record a skipped run.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.4:** When the head changes several
  times before a firing, the system shall fire once, for the latest head.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.5:** When a trigger has a quiet
  period of N minutes, the system shall fire `new_commits` only after the same
  new head has been observed unchanged for at least N minutes. A newer head
  restarts the wait. The quiet period shall default to 0 and accept whole
  minutes from 0 to 1440.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.6:** When any firing for a pull
  request is admitted, deduplicated, or skipped by the concurrency cap, the
  system shall store the pull request's current head and requested reviewers
  as its observation and clear any quiet-period wait. A deferred or failed
  admission shall leave the observation unchanged so a later poll retries.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.7:** The system shall fire at most
  one `github_pr` firing per pull request per poll. The precedence is
  `opened`, then `new_commits`, then `review_requested`. Transitions that the
  firing covers shall be stored as processed under AC-001.6.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.8:** When a pull request does not
  match the trigger's filters, the system shall not fire for it but shall
  still store its observation, so a later match fires only for later changes.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.9:** When a pull request is no
  longer open, the system shall delete its observation after a complete
  listing of the repository's open pull requests. A reopened pull request
  starts from a new baseline.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.10:** When `opened` is selected,
  the system shall fire it once per matching pull request, with dedup key
  `pr:<owner>/<repo>#<number>`. By default it shall fire only for a pull
  request created at or after the trigger's creation, and not for one whose
  stored observation shows it was open before `opened` was selected, even
  as the only event. An
  unknown creation time counts as new. When `include_existing_prs` is true,
  every matching open pull request shall fire once. A poll shall not resubmit
  an `opened` firing whose dedup key is held. Any combination of the three
  events can be selected on one trigger.

### REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-002: Apply PR filters and expose PR context

**Intent:** Make every saved `github_pr` filter effective, and give the prompt
enough context to review only what changed.

#### Acceptance criteria

- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-002.1:** When a `github_pr` trigger is
  evaluated for any event, the system shall apply its base-branch, author,
  label, and draft-exclusion filters to each open pull request. Base-branch
  patterns shall use the existing glob rules. A non-empty label list shall
  match a pull request that has at least one listed label.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-002.2:** When a `github_pr` firing is
  rendered, `{{pr.head_sha}}` shall resolve to the pull request's head SHA.
  `{{pr.event}}` shall resolve to `opened`, `new_commits`, or
  `review_requested`.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-002.3:** `{{pr.previous_head_sha}}`
  shall resolve to the observed head before the firing. For an `opened`
  firing, it shall resolve to an empty string.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-002.4:** For a `review_requested`
  firing, `{{pr.requested_reviewers}}` shall resolve to a comma-separated
  list of the newly requested matching reviewers. For other events, it shall
  resolve to an empty string.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-002.5:** When a person edits a
  `github_pr` prompt, the placeholder list shall include the four keys above
  with descriptions from the trigger registry, like the existing `pr.*` keys.

### REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-003: Configure PR events in the editor

**Intent:** Let a workspace owner enable and tune the PR events from the
automation editor on every viewport.

#### Acceptance criteria

- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-003.1:** When a person edits a
  `github_pr` trigger, the events section shall show **Opened**, **New
  commits**, and **Review requested** checkboxes. Each shall have a localized
  tooltip that states when it fires.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-003.2:** Selecting or clearing a
  checkbox shall add or remove its exact value (`opened`, `new_commits`,
  `review_requested`) in the saved `events` list and keep other values
  unchanged.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-003.3:** When **New commits** is
  selected, the section shall show a localized **Wait until pushes stop for**
  minutes input bound to `quiet_period_minutes`. It shall reject values
  outside 0–1440.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-003.4:** When **Review requested** is
  selected, the section shall show a localized **Reviewers** input bound to
  `reviewers`, which takes comma-separated user logins and `org/team-slug`
  values. Empty means any reviewer.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-003.5:** On desktop and phone layouts,
  every control in the section shall be reachable, readable, and operable
  without horizontal scrolling. Each checkbox shall have a touch target of at
  least 44 pixels on phone.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-003.6:** When **Opened** is selected,
  the section shall show a localized **Also run on pull requests already
  open** checkbox bound to `include_existing_prs`, cleared by default.

### REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004: Continue one task per pull request

**Intent:** Let follow-up firings for a pull request continue the task that
handled that pull request earlier, while separate pull requests keep separate
tasks.

**User story:** As a maintainer, I want a follow-up review to continue the
original review task for that pull request, so that the agent keeps its
earlier findings and conversation.

#### Acceptance criteria

- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.1:** The API, MCP configuration
  tools, and editor shall accept the `reuse_per_pr` continuation policy for
  the hidden and normal-task targets, and reject it for the managed
  conversation target with a validation error.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.2:** When a `github_pr` firing of a
  per-PR automation has no PR task, the system shall create a task for the
  firing and map it to the PR subject.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.3:** When a `github_pr` firing of a
  per-PR automation has a compatible PR task whose primary session is ready
  for input, the system shall send the rendered prompt as a new turn in that
  session and bind the run to that turn. The run shall record the thread
  action `resumed`.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.4:** When the PR task is deleted,
  archived, or incompatible with the automation's current workspace, workflow,
  target mode, runtime profiles, or repository, or its primary session has
  ended, the system shall create a replacement task, map it to the PR subject,
  and record the thread action `replaced` with the reason.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.5:** When the PR task is busy, the
  system shall defer the firing without creating a run, recording a skipped
  run, or consuming its dedup key. Each later poll shall try again, and the
  first poll after the task stops being busy shall fire for the pull
  request's latest state.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.6:** When the PR task becomes busy
  between admission and dispatch, the system shall mark the run failed with a
  reason that states that the task is busy. The system shall not create a
  second task for the PR subject.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.7:** A per-PR firing without a PR
  subject, such as a manual run, shall create an isolated task as `new_task`
  does.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.8:** The `max_concurrent_runs`
  limit shall continue to apply to the whole automation under the per-PR
  policy. The policy shall not force a value.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.9:** When a per-PR run is retried,
  the retry shall use the same PR-subject lookup and prompt selection as the
  original firing.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.10:** YAML export shall emit
  `continuation_policy: reuse_per_pr`.

### REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-005: Move continued PR tasks to a chosen step

**Intent:** Let a visible PR task return to a chosen workflow step, such as a
review column, when a follow-up firing continues it.

**User story:** As a maintainer, I want a continued review task to move back
to its review step, so that the board shows that the pull request is under
review again.

#### Acceptance criteria

- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-005.1:** When a per-PR automation uses
  the normal-task target, the system shall accept an optional continuation
  step from the automation's workflow. An empty value shall keep the task in
  its current step.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-005.2:** The system shall reject a
  continuation step that is not in the automation's workflow, or that is set
  for another policy or target mode, with a validation error.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-005.3:** When a firing continues a PR
  task, a continuation step is set, and the task is in another step, the
  system shall move the task to the continuation step before it sends the
  prompt. The step's own auto-start prompt shall not run, so the automation's
  prompt is the only turn sent. The step's other on-enter actions still apply.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-005.4:** When the move fails or the
  continuation step no longer exists, the system shall mark the run failed
  with a reason that names the step problem and shall not send the prompt.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-005.5:** When a firing creates a new or
  replacement PR task, the system shall start it in the automation's
  configured start step, not the continuation step.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-005.6:** YAML export shall emit the
  continuation step as `workflow.continuation_step: <step name>`.

### REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006: Configure per-PR continuation in the editor

**Intent:** Make the per-PR choice and its options understandable and
operable on every viewport.

#### Acceptance criteria

- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006.1:** When the edited automation has
  a `github_pr` trigger or already uses `reuse_per_pr`, the **Task for each
  run** choice shall show a third option, **One task per pull request**. Its
  description shall state that runs for one pull request continue its task and
  that different pull requests never share a task. With a `github_pr` trigger,
  the **One shared task** description shall state that different pull
  requests share one task.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006.2:** When the per-PR option is
  selected and the target is a normal task, the section shall show a localized
  **Move to step when continuing** selector. It lists the selected workflow's
  steps plus a **Stay in current step** choice, which is the default.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006.3:** When the workflow, target mode,
  or policy changes so that a per-PR option no longer applies, the editor
  shall clear that option before saving.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006.4:** When the per-PR option is
  selected, the editor shall show a **Follow-up instructions** field directly
  under the main instructions. It shall also show **When merged** and **When
  closed without merging** selectors with **Keep task**, **Archive task**, and
  **Delete task** choices. The delete choice shall carry a localized warning
  that the conversation and files are removed.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006.5:** On desktop and phone layouts,
  every per-PR control shall remain visible and operable, and each choice
  shall provide a touch target of at least 44 pixels.

### REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-007: Fire when a review is requested

**Intent:** Let a contributor ask the automated reviewer to look again, using
GitHub's normal review request or re-request.

**User story:** As a contributor, I want to re-request a review from the
automated reviewer, so that it reviews my changes when I say they are ready.

#### Acceptance criteria

- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-007.1:** When a matching open pull
  request's requested reviewers include a matching reviewer that its stored
  observation does not include, the system shall fire `review_requested`
  once. This includes a re-request after an earlier review.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-007.2:** A requested reviewer shall
  match when the trigger's `reviewers` list is empty, or when the list
  contains the user's login or the team's `org/team-slug`. Comparison shall
  ignore case.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-007.3:** Several reviewers that are
  newly requested on a pull request in one poll shall produce one firing.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-007.4:** The dedup key shall be
  `pr:<owner>/<repo>#<number>/review-requested/<generation>`, where the
  generation increases by one for each processed review-request transition of
  that pull request. A retry of the same transition shall reuse its key.

### REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-008: Use a separate follow-up prompt

**Intent:** Let a follow-up give the agent different instructions, such as
"review only the new commits", without conditional templates.

#### Acceptance criteria

- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-008.1:** When a per-PR automation has
  a non-empty follow-up prompt and a firing continues an existing PR task, the
  system shall render and send the follow-up prompt instead of the main
  prompt.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-008.2:** When a firing creates a new or
  replacement PR task, or the follow-up prompt is empty, the system shall use
  the main prompt.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-008.3:** The follow-up prompt shall
  support the same placeholders and interpolation rules as the main prompt.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-008.4:** The system shall reject a
  non-empty follow-up prompt for a policy other than `reuse_per_pr`. YAML
  export shall emit it as `follow_up_prompt` under the main prompt's fidelity
  rules.

### REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-009: Archive or delete PR tasks when the PR ends

**Intent:** Remove finished review tasks from the board without a manual step
and without an agent run.

**User story:** As a maintainer, I want review tasks to be archived or deleted
when their pull request is merged or closed, so that the board shows only live
reviews.

#### Acceptance criteria

- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-009.1:** A per-PR automation shall
  store two end-of-PR actions, `pr_merged_action` and `pr_closed_action`,
  each `keep`, `archive`, or `delete`. Both shall default to `keep`. The
  system shall reject a non-`keep` value for another policy.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-009.2:** When a mapped pull request is
  missing from a complete listing of its repository's open pull requests, the
  system shall read that pull request. If it is merged, the system shall apply
  `pr_merged_action` to its PR task. If it is closed without merging, the
  system shall apply `pr_closed_action`.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-009.3:** When the PR task is busy, the
  system shall postpone every action, including `keep`, and retry it on later
  polls until the task is no longer busy.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-009.4:** After the action is applied,
  including `keep`, the system shall remove the PR-subject mapping. A reopened
  pull request then gets a new task.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-009.5:** The action shall run directly
  in Kandev, without an agent run. An already archived or deleted task shall
  count as done.
- **AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-009.6:** YAML export shall emit both
  actions as `pr_merged_action` and `pr_closed_action`.

## Out of scope

- The `commented` event.
- GitHub `pull_request` webhook ingestion. Detection stays on the existing
  one-minute poller, so firings and end-of-PR actions land within about one
  minute plus any quiet period.
- **All repos** evaluation for `github_pr` triggers.
- Per-PR continuation and end-of-PR actions for automations that do not use
  `reuse_per_pr`, and for other trigger types.
- End-of-PR actions for mapped pull requests whose repository is no longer in
  any of the automation's `github_pr` triggers.
- Running the continuation step's own auto-start prompt.
