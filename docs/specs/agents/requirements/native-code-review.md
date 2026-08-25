---
status: draft
system: agents
created: 2026-07-24
owners:
  - jcfs
---
# Native Code Review Requirements

## Overview

Review feedback on agent-written code only reaches a Kandev user after they push and open a pull request, because every reviewer Kandev can reach today (CodeRabbit, Greptile, cubic) lives in GitHub CI. That leaves the working diff — the thing the user is actually looking at in the Changes/Review panel — unreviewed, gives nothing at all to GitLab, Azure DevOps, and local-only work, and never lands in Kandev's own review surface where anchored comments, reviewed hashes, and stale detection already live.

## Requirements

### REQ-AGENTS-NATIVE-CODE-REVIEW-001: Native Code Review

**Intent:** Review feedback on agent-written code only reaches a Kandev user after they push and
open a pull request, because every reviewer Kandev can reach today (CodeRabbit, Greptile, cubic)
lives in GitHub CI. That leaves the working diff — the thing the user is actually looking at in the
Changes/Review panel — unreviewed, gives nothing at all to GitLab, Azure DevOps, and local-only
work, and never lands in Kandev's own review surface where anchored comments, reviewed hashes, and
stale detection already live.

#### Acceptance criteria

- **AC-AGENTS-NATIVE-CODE-REVIEW-001.1:** A user SHALL be able to ask Kandev to review a task's current changes from the Review surface, without a pull request, a remote, or a Git host.
- **AC-AGENTS-NATIVE-CODE-REVIEW-001.2:** A review pass produces **findings**: each finding is anchored to a repository, file path, and line range, and carries a severity, a category, a one-line title, and a Markdown body.
- **AC-AGENTS-NATIVE-CODE-REVIEW-001.3:** Findings render as inline annotations in the existing Changes/Review diff, at their anchored line, using the same annotation surface as anchored review comments.
- **AC-AGENTS-NATIVE-CODE-REVIEW-001.4:** Findings are **advisory**. For each finding the human can **Resolve** it, **Dismiss** it, or **Send to agent** — which turns the finding into ordinary agent context and sends it to the active session. Kandev SHALL NOT apply, stage, or commit a fix on its own.
- **AC-AGENTS-NATIVE-CODE-REVIEW-001.5:** A review pass SHALL be triggerable two ways: on demand from the Review/Changes surface, and as a workflow-step entry action so a review sits between an implement step and a human review step.
- **AC-AGENTS-NATIVE-CODE-REVIEW-001.6:** The reviewing runtime is configurable independently of the agent that wrote the code. The on-demand path uses the effective profile selected by the built-in `code-review` utility agent; the workflow-step path can additionally name an agent profile directly. Both paths use the profile's complete launch and permission configuration as defined by [Profile-backed Utility Agents](utility-agent-profiles.md).
- **AC-AGENTS-NATIVE-CODE-REVIEW-001.7:** For a multi-repository task, findings carry their repository and are grouped per repository exactly like the rest of the Changes panel.
- **AC-AGENTS-NATIVE-CODE-REVIEW-001.8:** When the diff moves under a finding, the finding SHALL become **stale** rather than being dropped or rendered against unrelated code. Staleness reuses the per-file diff-hash mechanism that already drives review-mark staleness.

## Migrated source detail

## Why

Review feedback on agent-written code only reaches a Kandev user after they push and open a pull request, because every reviewer Kandev can reach today (CodeRabbit, Greptile, cubic) lives in GitHub CI. That leaves the working diff — the thing the user is actually looking at in the Changes/Review panel — unreviewed, gives nothing at all to GitLab, Azure DevOps, and local-only work, and never lands in Kandev's own review surface where anchored comments, reviewed hashes, and stale detection already live.

## What

- A user SHALL be able to ask Kandev to review a task's current changes from the Review surface, without a pull request, a remote, or a Git host.
- A review pass produces **findings**: each finding is anchored to a repository, file path, and line range, and carries a severity, a category, a one-line title, and a Markdown body.
- Findings render as inline annotations in the existing Changes/Review diff, at their anchored line, using the same annotation surface as anchored review comments.
- Findings are **advisory**. For each finding the human can **Resolve** it, **Dismiss** it, or **Send to agent** — which turns the finding into ordinary agent context and sends it to the active session. Kandev SHALL NOT apply, stage, or commit a fix on its own.
- A review pass SHALL be triggerable two ways: on demand from the Review/Changes surface, and as a workflow-step entry action so a review sits between an implement step and a human review step.
- The reviewing runtime is configurable independently of the agent that wrote the code. The
  on-demand path uses the effective profile selected by the built-in `code-review` utility agent;
  the workflow-step path can additionally name an agent profile directly. Both paths use the
  profile's complete launch and permission configuration as defined by
  [Profile-backed Utility Agents](utility-agent-profiles.md).
- For a multi-repository task, findings carry their repository and are grouped per repository exactly like the rest of the Changes panel.
- When the diff moves under a finding, the finding SHALL become **stale** rather than being dropped or rendered against unrelated code. Staleness reuses the per-file diff-hash mechanism that already drives review-mark staleness.
- An agent with task MCP SHALL be able to publish findings directly, so a full agent session (workflow step, or a user prompt) can produce the same findings as the built-in pass.
- Every review pass is visible as a **run** with a status, a finding count, and a failure reason when it fails.
- The review surface SHALL have full capability parity on phones, using native mobile presentation for the findings list and per-finding actions.

## Data model

Two new tables in the task SQLite repository (`internal/task/repository/sqlite/`).

```
task_review_runs
  id                string     PK
  task_id           string     FK -> tasks.id (cascade delete), indexed
  session_id        string     nullable; session whose workspace supplied the diff
  trigger           enum       manual | workflow_step | agent
  workflow_step_id  string     nullable; set when trigger = workflow_step
  agent_profile_id  string     effective profile used for execution; "" when trigger = agent
  agent_id          string     resolved inference agent CLI snapshot; "" when trigger = agent
  model             string     resolved model snapshot; "" when trigger = agent
  status            enum       pending | running | completed | failed | cancelled
  error_message     string     "" unless status = failed
  summary           string     optional agent-authored one-paragraph summary
  finding_count     int        findings persisted by this run
  file_count        int        changed files submitted to the reviewer
  repository_count  int        repositories covered
  prompt_tokens     int
  response_tokens   int
  duration_ms       int
  created_at        timestamp
  completed_at      timestamp  nullable
```

```
task_review_findings
  id               string   PK
  run_id           string   FK -> task_review_runs.id (cascade delete), indexed
  task_id          string   FK -> tasks.id (cascade delete), indexed
  repository_id    string   "" for a single-repository task
  repository_name  string   sanitized repo dir name; "" for a single-repository task
  file_path        string   repository-relative path
  start_line       int      > 0
  end_line         int      >= start_line
  side             enum     additions | deletions (default additions)
  severity         enum     blocker | major | minor | nit
  category         string   short kebab-case slug, <= 40 chars
  title            string   <= 120 chars, no newlines
  body             string   Markdown
  suggestion       string   optional suggested replacement text; display-only
  anchor_text      string   the anchored diff lines at publish time, <= 2000 chars
  file_diff_hash   string   djb2 hash of the file's normalized diff at publish time
  status           enum     open | resolved | dismissed
  resolved_at      timestamp nullable
  created_at       timestamp
  updated_at       timestamp
```

`(task_id, status)` and `(task_id, repository_name, file_path)` are indexed. A task keeps findings from more than one run; publishing a new run does not delete earlier findings, but `open` findings from a previous run whose `(repository_name, file_path, start_line, end_line, title)` tuple repeats are superseded — the older row is deleted so the same issue is not listed twice.

`file_diff_hash` uses the same djb2 hash as `apps/web/lib/utils/hash.ts` and `session_file_reviews.diff_hash`, over the same normalized diff text, so the frontend can compare a stored hash against a freshly computed one without a second algorithm.

Deleting a task deletes its runs and findings. Deleting a repository attachment does not delete findings; they simply stop matching a file and are reported as stale.

## API surface

### WebSocket actions (client → server)

| Action | Payload | Response |
|---|---|---|
| `task.review.run` | `{task_id, session_id, repository_id?, agent_profile_id?}` | `{run: TaskReviewRun}` |
| `task.review.cancel` | `{run_id}` | `{run: TaskReviewRun}` |
| `task.review.get` | `{task_id}` | `{runs: TaskReviewRun[], findings: TaskReviewFinding[]}` |
| `task.review.finding.update` | `{finding_id, status}` | `{finding: TaskReviewFinding}` |
| `task.review.clear` | `{task_id}` | `{success: true}` |

`task.review.run` rejects with `review_agent_unavailable` when no effective agent profile can be resolved, and with `review_no_changes` when the task has no changed files. A second `task.review.run` for a task that already has a `pending` or `running` run returns that run unchanged instead of starting a second pass.

### WebSocket events (server → client)

- `task.review.run_updated` — payload is the run; fired on every status change.
- `task.review.findings_published` — `{task_id, run_id, findings: TaskReviewFinding[]}`.
- `task.review.finding_updated` — `{task_id, finding: TaskReviewFinding}`.
- `task.review.cleared` — `{task_id}`.

### Task MCP tool

`publish_review_findings_kandev`

```
task_id  string   required
summary  string   optional one-paragraph summary of the review
findings array    required, >= 1 entry; each entry:
  repo        string  optional multi-repo subpath; omit for single-repo tasks
  file        string  required, repository-relative path
  line        int     required, > 0
  line_end    int     optional, >= line
  severity    string  required, one of blocker | major | minor | nit
  category    string  required, short kebab-case slug
  title       string  required
  body        string  required, Markdown
  suggestion  string  optional
```

Returns the number of findings stored and the run id. The call creates a `task_review_runs` row with `trigger = agent` and `status = completed`. Rejecting a malformed entry rejects the whole call so an agent never persists a partially-anchored review.

### Workflow step action

A new `on_enter` action type `run_code_review`, with an optional `agent_profile_id`. Entering a step with this action starts a review pass with `trigger = workflow_step`. The action is available in the step editor next to **Auto-start agent** and is exported/imported with the workflow, referencing the agent profile by portable agent name, model, and mode like every other step profile reference.

## State machine

A review run:

| From | To | Trigger | Actor |
|---|---|---|---|
| — | `pending` | `task.review.run`, `run_code_review` step entry, or `publish_review_findings_kandev` | user, workflow engine, agent |
| `pending` | `running` | the run acquires the task's diff and dispatches the reviewer prompt | review service |
| `pending`/`running` | `cancelled` | `task.review.cancel`, or the backend shuts down mid-run | user, system |
| `running` | `completed` | reviewer returned a parseable result; findings persisted | review service |
| `running` | `failed` | diff unavailable, no capable agent, reviewer error, or unparseable result | review service |

A finding: `open` → `resolved` (user resolves) or `dismissed` (user dismisses); both are reversible back to `open`. Staleness is not a stored state — it is derived per render by comparing `file_diff_hash` against the current diff for that file, so a finding becomes stale and un-stale as the diff changes.

## Failure modes

| Condition | Behavior |
|---|---|
| No effective utility profile, or the resolved profile is missing, disabled, non-inference-capable, or CLI-passthrough-only | The run fails immediately with `review_agent_unavailable`. The Review surface shows an inline message naming **Settings → Utility Agents** and does not retry. No findings are written. |
| Resolved profile has no usable model | Same as above; the error names the profile and missing model. |
| Task workspace not materialized, or agentctl unreachable | The run fails with `review_workspace_unavailable`. Existing findings are untouched. |
| Task has no changed files | `task.review.run` returns `review_no_changes` without creating a run. |
| Reviewer returns text that contains no parseable findings block | The run fails with `review_unparseable_response`, and the raw response is retained on the run's `error_message` (truncated) for debugging. No findings are written. |
| A single finding in an otherwise valid response is malformed (missing file, non-positive line, unknown severity) | That finding is skipped and counted; the run still completes. The run's summary reports how many entries were rejected. Malformed entries submitted through `publish_review_findings_kandev` reject the whole call instead, because an agent can retry. |
| A finding anchors to a file that is not in the current changed-file set | The finding is persisted and listed in the findings overview under its repository, marked **not in current changes**. It is not rendered inside any file's diff. |
| Diff exceeds the reviewer's context | The diff is submitted in per-file batches; a file whose own diff cannot fit is skipped and named in the run summary. The run still completes with findings from the files that were reviewed. |
| Backend restarts while a run is `pending` or `running` | On boot, those runs are marked `cancelled` with `error_message = "interrupted by restart"`. They are never silently resumed. |

## Persistence guarantees

Runs and findings survive a Kandev restart; they are ordinary task-scoped SQLite rows and are removed only when the task is deleted or the user clears the task's review. In-flight review passes do not survive a restart — see Failure modes. Finding `status` changes are persisted immediately, so a resolved finding stays resolved across reloads and across browsers, unlike pending inline comments, which remain browser-local `sessionStorage` state.

Derived staleness is not persisted. A finding's `file_diff_hash` and `anchor_text` are, so staleness is recomputed identically on every client.

## Scenarios

- **GIVEN** a task with uncommitted changes and a configured `code-review` utility agent, **WHEN** the user selects **Review changes** in the Review toolbar, **THEN** a run appears with status `running`, and on completion each returned finding renders as an annotation at its anchored line in that file's diff.
- **GIVEN** a completed review with three findings, **WHEN** the user resolves one, **THEN** that annotation collapses to a resolved state, the findings count drops to two open, and the state survives a page reload.
- **GIVEN** an open finding on `apps/web/foo.ts:42`, **WHEN** the user selects **Send to agent**, **THEN** the finding's file, line range, and body are sent to the active session as agent context, and the finding remains `open`.
- **GIVEN** a multi-repository task with findings in `frontend` and `backend`, **WHEN** the Review panel renders, **THEN** each finding renders only inside its own repository's file, and the findings overview groups them under per-repository headings.
- **GIVEN** an open finding whose file diff has changed since the run, **WHEN** the Review panel renders, **THEN** the finding is shown as stale in that file's section and is not rendered against the file's current line 42.
- **GIVEN** a workflow step configured with the `run_code_review` entry action and an agent profile whose model differs from the implementing step's, **WHEN** a task moves into that step, **THEN** a run with `trigger = workflow_step` starts using that profile's agent and model, and the resulting findings are visible in the task's Review panel.
- **GIVEN** a workspace with no effective utility profile configured, **WHEN** the user selects
  **Review changes**, **THEN** no run is created and the surface shows an actionable message
  pointing at Settings → Utility Agents.
- **GIVEN** a task whose only agent profile is CLI-passthrough, **WHEN** a `run_code_review` step is entered, **THEN** the run fails with `review_agent_unavailable` and the task still enters the step.
- **GIVEN** an agent session with task MCP, **WHEN** it calls `publish_review_findings_kandev` with two valid findings, **THEN** a `completed` run with `trigger = agent` is stored and both findings appear in the Review panel without a page reload.
- **GIVEN** an agent calls `publish_review_findings_kandev` with one finding missing `file`, **WHEN** the call is handled, **THEN** it returns an error, and no run or finding is stored.
- **GIVEN** a review run in `running` state, **WHEN** the user selects **Cancel review**, **THEN** the run moves to `cancelled` and no findings are written.
- **GIVEN** a task with review findings, **WHEN** the task is deleted, **THEN** its runs and findings are removed.
- **GIVEN** a phone-sized viewport with a completed review, **WHEN** the user opens Changes, **THEN** the findings list is reachable as a bottom sheet and every desktop per-finding action is available from it.

## Out of scope

- Auto-applying, staging, or committing a fix from a finding. `suggestion` is display-only in this iteration.
- Replacing or duplicating GitHub CI reviewers. This runs earlier and is complementary; nothing here reads or writes GitHub review threads.
- Reconciling findings with a pull request's review comments, in either direction.
- Cross-task or whole-repository review. Scope is one task's changed files.
- Reviewing files that are not part of the task's changed set (no "review the whole module for context" mode).
- A dedicated review-findings history UI beyond the per-task run list.
- Scheduled or automatic-on-every-turn review. Triggers are explicit: user action, workflow-step entry, or an agent MCP call.
