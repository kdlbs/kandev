---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002
created: 2026-08-24
owners:
  - kandev
---

# GitHub PR Merge Queue System Design

## Purpose and boundaries

The integration system owns the GitHub merge operation and the normalized
merge-queue state. The task system consumes a bounded pull-request summary. The
UI renders the integration state but does not own a second queue contract.

This design covers the complete vertical path. It includes the GitHub clients,
storage, task projection, HTTP response, frontend types, React surfaces, and
desktop and mobile behavior.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001` | [Components](#components-and-responsibilities), [Data](#data-and-contracts), [Control flow](#control-flow), [Failure](#failure-and-recovery), [Presentation](#presentation-and-responsive-behavior) |
| `REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002` | [Data](#data-and-contracts), [Automatic merge](#automatic-merge), [Explicit retry](#explicit-retry), [Task-row projection](#task-row-automation-projection), [Failure](#failure-and-recovery), [Persistence](#persistence), [Presentation](#presentation-and-responsive-behavior) |

## Components and responsibilities

### GitHub provider boundary

- `apps/backend/internal/github/client.go` defines the typed merge outcome.
- `gh_client.go` and `pat_client.go` call GitHub's asynchronous merge endpoint.
- The clients bind an automatic request to an expected head and pace pending
  status reads within the request context.
- `graphql.go` reads and normalizes `mergeQueueEntry` state and metadata.

### Integration service and storage

- `service_pr.go` routes the merge request through workspace-scoped write
  credentials and invalidates stale pull-request data after success.
- `controller.go` exposes the existing merge endpoint and returns the typed
  `merged` or `queued` result.
- `service_pr_watch.go` applies authoritative queue observations and publishes
  the normal pull-request update.
- `models.go` owns the normalized `PRStatus` and persisted `TaskPR` fields.
- `store.go` owns the `github_task_prs` schema and all queue-field write paths.
- The CI automation store owns a durable per-pull-request merge-attempt journal.
- The automation service exposes an explicit retry command for one linked pull
  request. The controller does not call the merge provider directly.

### Task projection

- `apps/backend/internal/task/statussummary` reduces full pull-request state to
  a bounded task status, including aggregate active auto-fix and auto-merge
  flags.
- `apps/backend/internal/backendapp/status_summary_adapter.go` supplies the
  stored queue and per-pull-request automation state during live updates and
  restart hydration.

### Web application

- `apps/web/lib/api/domains/github-pr-api.ts` types the merge result.
- `apps/web/lib/types/github.ts` defines the queue-state and `TaskPR` fields.
- `pr-merge-button.tsx` controls eligibility, submission, feedback, and refresh.
- `pr-merge-queue-status.tsx` normalizes queue presentation and duration copy.
- `pr-ci-automation-controls.tsx` maps a typed automatic-merge error to the
  explicit retry command. Loading errors keep their separate refresh action.
- `pr-task-icon.tsx` overlays bounded automation indicators on the task-row
  pull-request icon and lazily hydrates per-pull-request details for its
  pointer, keyboard, or touch disclosure.
- The task icon, status chip, summary, and detail components reuse the same
  automation labels and per-pull-request option matching.

## Data and contracts

`github_task_prs` stores the last authoritative queue observation:

- `merge_queue_state TEXT NOT NULL DEFAULT ''` stores the normalized GitHub
  state. Supported values are `queued`, `awaiting_checks`, `mergeable`,
  `unmergeable`, and `locked`.
- `merge_queue_position INTEGER NULL` stores GitHub's one-based position.
- `merge_queue_estimated_time_to_merge_seconds INTEGER NULL` stores an optional
  non-negative duration.

An empty state with null metadata means that no active queue entry is stored.
The same three fields appear in the `TaskPR` HTTP, boot, and WebSocket payloads.
The bounded task projection reduces active membership to `queued`.

`TaskStatusSummary.pull_request` also carries two optional aggregate booleans:

- `auto_fix_enabled` is true when at least one active linked pull request has
  auto-fix enabled.
- `auto_merge_enabled` is true when at least one active linked pull request has
  auto-merge enabled.

These fields are task-row decorations. They do not replace the per-pull-request
automation options, authorize automation, or carry an unbounded option list.

The existing merge endpoint remains:

```http
PUT /api/v1/github/prs/:owner/:repo/:number/merge?workspace_id=:workspaceId
Content-Type: application/json

{"merge_method":"squash"}
```

`merge_method` is optional. It accepts `merge`, `squash`, or `rebase`. A
successful response is one of these objects:

```json
{"status":"merged"}
{"status":"queued"}
```

The provider request uses `merge_action=default`. GitHub therefore selects the
allowed direct or queued path.

`Client.MergePR` accepts a request value with the merge method and an optional
expected head SHA. Automatic merge requests require a non-empty expected head.
The clients send it as GitHub's `sha` field. A different provider head fails the
request instead of merging a pull-request revision that Kandev did not review.
The manual merge action keeps its existing behavior until its separate contract
requires head binding.

GitHub returns the merge status at the response root. It returns the polling
UUID and failure message inside `details`. The provider clients use
`details.uuid` to poll a pending request and `details.message` to report a
failed request. A pending response without `details.uuid` is invalid.
The clients also decode `details.expected_head_sha` for error diagnostics.

`github_task_ci_pr_state` keeps the automatic attempt journal for each linked
pull request:

- `last_merge_signature` stores the readiness state that authorized an attempt.
- `last_merge_attempt_at` stores when Kandev reserved the attempt.
- `last_queue_attempt_head_sha` stores the head accepted by GitHub or observed
  in the active queue.
- `last_merge_result` stores `in_flight`, `failed`, or `accepted`.
- `last_error_kind` identifies `auto_merge` errors separately from loading,
  auto-fix, and other automation errors.

The readiness signature includes the task, repository, pull request, pull-request
lifecycle state, head SHA, check conclusion, mergeability, review decision,
review count, required-review presence and value, pending-check count, and
unresolved-thread count. Including lifecycle state prevents a closed pull
request from reusing an attempt reserved while it was open, including after a
later reopen.

The explicit retry endpoint is:

```http
POST /api/v1/github/tasks/:taskId/ci-automation/retry-merge
Content-Type: application/json

{"repository_id":"repo-id","pr_number":3117}
```

The endpoint returns `202 Accepted` after it durably authorizes a new evaluation.
This response does not claim that GitHub accepted or completed the merge.

## Control flow

### Merge request

1. The frontend derives eligibility from the current pull-request state.
2. The user activates the existing merge action.
3. The service selects workspace-scoped personal-write credentials.
4. The provider starts or resumes GitHub's asynchronous merge request.
5. The provider polls a pending request until GitHub returns a terminal result.
6. The controller returns `merged` or `queued` to the frontend.
7. The frontend shows outcome-specific feedback and requests a state refresh.

Only GitHub's `enqueued` result maps to `queued`. An already merged pull request
maps to `merged`. A provider failure remains retryable.

### Automatic merge

1. The evaluator refreshes the exact linked pull request.
2. It requires fresh state, a non-empty head SHA, and all readiness gates.
3. It computes the complete readiness signature.
4. Before the unchanged-signature guard, it consumes a pending one-shot retry
   authorization for this exact repository and pull request, if one exists.
   That authorization bypasses only the unchanged-signature guard. The
   evaluator still requires fresh state, the current head, and every readiness
   gate. Without an authorization, an unchanged `in_flight`, `failed`, or
   `accepted` signature stops the automatic path.
5. It reserves an `in_flight` attempt in storage before any provider side
   effect.
6. It calls the provider with the exact observed head SHA.
7. It records `failed` or `accepted` from the provider result. An accepted
   result clears a prior `auto_merge` error for that pull request.

If the reservation cannot be stored, the evaluator does not call GitHub. A
changed readiness signature can authorize one later automatic attempt after all
gates pass.

### Pending polling

The provider waits at least one second between pending-status reads. The wait
observes cancellation and the existing two-minute request budget. A conflict
response resumes the request with its returned UUID. A missing, expired, or
unknown request fails with a diagnostic error.

### Explicit retry

The retry command authorizes one new evaluation for the exact repository and
pull-request number. It applies only to a failed attempt or to an expired
`in_flight` attempt. Expiration is an explicit transition:
`expired in_flight -> failed -> authorized retry`. A retry cannot be authorized
while the original attempt is still `in_flight`, and it does not bypass checks,
reviews, mergeability, thread, or head requirements.

The service persists the retry authorization and publishes the normal
automation evaluation event. The evaluator then refreshes the pull request and
runs the full gate sequence. The HTTP handler never calls GitHub directly.

### Task-row automation projection

The authoritative pull-request loader reads linked pull requests and their
per-pull-request automation options in bounded batches. It supplies the two
switches with each internal pull-request observation. The summary derives each
aggregate flag from open pull requests only.

The status-summary projector subscribes to `github.task_ci_options.updated`.
It refreshes the task's authoritative pull-request observations before it
publishes a complete replacement summary. Existing pull-request update events
use the same observations, so a merge, close, unlink, or option change removes
obsolete indicators without browser-side polling.

The task row reads only `status_summary`. It does not mount
`useTaskCIAutomationOptions` for every visible task and does not issue an HTTP
request per row. When the user opens the icon disclosure, one deduplicated lazy
request hydrates the full pull-request records and per-pull-request automation
options needed for detailed text.

### Queue observation

The batched GraphQL selection reads this field:

```graphql
mergeQueueEntry { state position estimatedTimeToMerge }
```

`graphql.go` records whether the response observed queue membership. This guard
distinguishes an authoritative null from REST and `gh pr view` responses that
cannot read queue data.

`SyncTaskPR` atomically replaces the queue state, position, and estimate after
an authoritative GraphQL observation. It preserves all three fields after a
queue-unaware read. It clears them after an authoritative null, merge, or close.
The update then publishes `github.task_pr.updated` through the existing path.

An active queue entry reconciles the attempt journal to `accepted`. A merged
state does the same. Either observation clears only an error whose kind is
`auto_merge`. It preserves unrelated automation errors.

## State transitions

- An open pull request with no observed queue entry is not queued.
- A non-null authoritative entry creates or updates active queue membership.
- A later authoritative null removes active queue membership.
- A merged or closed state clears queue metadata and wins presentation priority.
- An active queue or merged observation marks the matching attempt accepted.
- A queue-unaware or failed read preserves the last complete observation.
- An unknown future non-empty state remains active and uses generic UI copy.

## Failure and recovery

Missing credentials, permission errors, provider validation errors, rate limits,
and transport errors return a non-success response. These errors do not change
the stored pull-request state or show a successful outcome.

An unknown successful provider result fails closed. Kandev does not claim that
the pull request merged or entered the queue.

A failed GraphQL queue read preserves the last complete observation. The next
successful authoritative sync updates or clears it. This behavior also protects
queue state during restart recovery and mixed GraphQL, REST, and CLI refreshes.

An automatic provider failure records `failed`, stores an `auto_merge` error,
and blocks automatic repetition for the unchanged readiness signature. A changed
signature or an explicit retry can rearm the attempt.

After a process interruption, Kandev first reconciles a stored `in_flight`
attempt with authoritative queue and merged state. If neither state appears
before the attempt deadline, Kandev marks the attempt failed and exposes the
explicit retry. It does not resubmit the unchanged attempt automatically.

## Persistence

The queue fields use additive SQLite columns in `github_task_prs`. Fresh schema,
migration, create, replace, restore, and update paths round-trip the complete
entry. Existing rows start with no queue entry and converge after a successful
GraphQL status sync.

The attempt journal uses additive columns in `github_task_ci_pr_state`. A
startup migration classifies only recognized stable legacy merge-error prefixes
as `auto_merge`. An unknown or empty error kind is never cleared as an automatic
merge error.

Reservation, result, error-kind, and retry-authorization changes use storage
transactions. The existing per-PR singleflight remains an optimization. Durable
reservation is the correctness boundary across concurrent events and restarts.

The estimate remains a duration. Kandev does not convert it to a durable target
timestamp because GitHub can change the estimate on each observation.

## Security

The merge action uses the active workspace's personal-write GitHub route. It
requires the same pull-request permissions as GitHub's merge API. Kandev does
not bypass repository rules or elevate the user.

The retry endpoint uses the same task and workspace authorization. It accepts
only the exact pull request that is linked to the task.

## Presentation and responsive behavior

An active queue entry uses `#966600`. Terminal states take precedence. Queue
membership takes precedence over other non-terminal icon states.

The task hover summary, desktop status popover, phone drawer, and pull-request
detail panel show localized state and position. Estimates below one minute use a
localized sub-minute label. Estimates of one minute or more round up to a
localized whole-minute duration. Missing estimates omit estimate text instead
of inventing a value. Unknown provider states use generic queue copy.

Desktop and mobile share state derivation and actions. Mobile uses the existing
Review surface and status drawer. The action has a touch-sized target, does not
depend on hover, and does not create document-level horizontal overflow.

The task-row pull-request icon is the nearest shipped status-summary exemplar.
It overlays a small yellow dot at the top-left for active auto-fix and a small
purple dot at the top-right for active auto-merge. A contrasting surface ring
keeps both dots distinct from every pull-request status color. The dots are
supplementary; accessible text names each enabled setting.

Fine-pointer hover and keyboard focus extend the existing pull-request tooltip
with an Automation section. The section identifies enabled settings by active
pull request when per-pull-request options differ. A coarse-pointer activation
uses the existing compact task status drawer pattern and a touch-sized icon
target. Both presentations reuse the same lazy data and view model. The task
row remains the primary navigation target, and the automation disclosure does
not change settings.

An `auto_merge` error shows Retry on desktop and mobile for the selected pull
request. Other stored automation errors show Refresh because repeating a merge
would not address them. Loading errors keep their existing retry of the state
request.

Mobile keeps the existing inset status drawer and its single scroll owner. The
shared error row has a target of at least 44 by 44 CSS pixels. This change does
not add a drawer, dialog, or navigation surface.

## Verification evidence

Focused backend coverage exists in the GitHub client, GraphQL conversion,
storage, synchronization, and task-status packages. Frontend unit coverage
exists for merge eligibility, merge outcomes, queue formatting, and status
priority. Playwright coverage exists in:

- `apps/web/e2e/tests/pr/pr-merge-queue.spec.ts`
- `apps/web/e2e/tests/pr/mobile-pr-merge-queue.spec.ts`
- `apps/web/e2e/tests/pr/pr-status-badge.spec.ts`
- `apps/web/e2e/tests/pr/mobile-pr-sidebar-automation-indicators.spec.ts`

## Related delivery records

- [Bind automatic merge attempts to the reviewed head](../../../decisions/2026-08-28-bind-github-auto-merge-attempts-to-reviewed-head.md)
- [Separate task summary and session stream traffic](../../../decisions/2026-08-01-separate-task-summary-session-stream-traffic.md)
- [Automatic merge reliability plan](../../../plans/github-auto-merge-reliability/plan.md)
- [Sidebar automation indicators plan](../../../plans/github-sidebar-automation-indicators/plan.md)
- [Queue-status visibility plan](../../../plans/github-pr-merge-queue-status/plan.md)
- [Original queue-aware merge action plan](../../../plans/github-pr-merge-queue/plan.md)
