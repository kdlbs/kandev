---
status: building
created: 2026-08-17
owner: Kandev
---

# GitHub PR Merge Queue

## Why

Users can merge an eligible GitHub pull request from Kandev, but repositories
that require GitHub's merge queue leave the same pull request looking blocked
and force the user to leave Kandev. The merge action should respect the
repository's GitHub rules and complete through the appropriate direct or queued
path.

## What

- An open, approved pull request with successful required checks exposes its
  merge action in the existing GitHub PR detail and compact status surfaces,
  including when GitHub reports it blocked only because the base branch uses a
  merge queue.
- Activating the action asks GitHub to choose the appropriate merge behavior:
  merge immediately where permitted or add the pull request to the configured
  merge queue.
- A direct merge reports that the pull request merged. An accepted queued merge
  reports that the pull request was added to the merge queue and prevents
  repeated submission while local PR state refreshes.
- After GitHub reports an active merge queue entry, the linked pull request uses
  GitHub's merge-queue color, `#966600`, in task and compact PR indicators. This
  color is distinct from the yellow CI-in-progress, green passing, and emerald
  ready states.
- The task PR hover summary, compact PR status popover or phone drawer, and PR
  detail panel identify the pull request as queued. They translate GitHub's
  queue state into user-facing copy instead of displaying the raw provider
  enum.
- Queue surfaces show the pull request's current position in the merge queue.
  They also show GitHub's estimated time to merge when GitHub supplies one,
  formatted as a localized duration rather than raw seconds. Estimates below
  one minute use a localized sub-minute label; larger estimates round up to the
  next whole minute.
- Queue membership takes precedence over other non-terminal icon colors while
  the pull request remains in the queue. Merged and closed states retain their
  existing terminal colors.
- The action remains unavailable for drafts, conflicts, changes requested,
  failed or incomplete required checks, and unmet required reviews.
- GitHub remains authoritative for final eligibility. A rejected request leaves
  the action retryable and shows GitHub's error without claiming that the pull
  request merged or entered the queue.
- Desktop and mobile use the existing PR detail behavior. On phones, the action
  remains available through the full-height Review surface with a touch-sized
  target and no horizontal document overflow. Queue status is also available
  through that Review surface and the existing PR status drawer, so it never
  depends on hover.

## Data Model

The linked pull request stores the last successfully observed merge queue
entry in `github_task_prs`:

- `merge_queue_state TEXT NOT NULL DEFAULT ''` stores the normalized GitHub
  `MergeQueueEntryState`. Supported values are `queued`, `awaiting_checks`,
  `mergeable`, `unmergeable`, and `locked`.
- `merge_queue_position INTEGER NULL` stores GitHub's one-based queue position.
- `merge_queue_estimated_time_to_merge_seconds INTEGER NULL` stores GitHub's
  current estimate as a non-negative duration in seconds.

An empty state with null metadata means no active queue entry is stored.
Existing rows start in that state and converge on their next successful
GraphQL status sync.

The queue fields are part of the existing `TaskPR` HTTP, boot, and WebSocket
payloads. The bounded task-status projection reduces a queued open pull request
to the aggregate state `queued` so task rows can retain the queue color before
the full task PR collection hydrates.

## API Surface

Kandev retains the existing endpoint and extends its success response:

```http
PUT /api/v1/github/prs/:owner/:repo/:number/merge?workspace_id=:workspaceId
Content-Type: application/json

{"merge_method":"squash"}
```

- `merge_method` remains optional and accepts `merge`, `squash`, or `rebase`.
- Kandev submits the request through GitHub's asynchronous merge API with
  `merge_action=default`, allowing GitHub to select direct merge or merge queue.
- Success returns `200` with one of:

```json
{"status":"merged"}
{"status":"queued"}
```

- A `pending` response includes a UUID. Kandev polls that request, including
  the UUID returned by an existing-request `409`, until GitHub reports
  `merged`, `enqueued`, or `failed`.
- Only `enqueued` maps to `queued`. An already-merged pull request maps to
  `merged`, while `failed` remains an error that the user can retry.

Queue status reuses the existing GitHub status synchronization contract. The
batched GraphQL pull-request selection reads
`mergeQueueEntry { state position estimatedTimeToMerge }`; it does not add an
endpoint or a poller. `position` is a one-based integer.
`estimatedTimeToMerge` is an optional duration in seconds, not a timestamp. A
status result records whether queue membership was actually observed so REST
and `gh pr view` reads, which do not include `mergeQueueEntry`, cannot clear a
valid stored queue entry.

## State Machine

- An open pull request with no `mergeQueueEntry` is not queued.
- A successful GraphQL observation of `mergeQueueEntry` atomically enters or
  updates its state, position, and optional estimate, then publishes the normal
  task-PR update.
- A later successful GraphQL observation with `mergeQueueEntry: null` leaves
  the queue and clears the stored state, position, estimate, and queued UI.
- A merged or closed pull request clears queue state and renders its existing
  terminal status even if a less capable read path did not observe the queue
  entry disappear.
- A failed or queue-unaware status read preserves the complete last stored
  queue entry.

## Permissions

The action uses the active workspace's personal-write GitHub routing and
requires the same GitHub content/pull-request permissions as the provider's
merge API. Kandev does not bypass repository rules or elevate the user.

## Failure Modes

- Missing GitHub credentials or required permissions return a non-success
  response, surface a useful error, and leave the action retryable.
- GitHub validation, readiness, rate-limit, or transport failures do not change
  local PR state and do not show a success notification.
- An unrecognized successful provider status fails closed rather than claiming
  that the pull request merged or entered the queue.
- After an accepted request, Kandev invalidates cached PR feedback/status and
  refreshes the linked pull request; the GitHub poller remains authoritative
  for its eventual merged state.
- A failed GraphQL queue-status read preserves the previous queue state and
  leaves the normal refresh affordance available. Kandev does not interpret a
  missing field from a queue-unaware read as removal from the queue.
- An unknown future non-empty queue state still identifies the pull request as
  queued with generic copy; Kandev does not expose the raw enum value.
- If GitHub omits the estimated time to merge, Kandev still shows queue state
  and position without an empty or invented estimate.

## Persistence Guarantees

The last observed queue state, position, and estimate survive a Kandev restart
with the linked task PR. They can be temporarily stale until the next
successful GitHub status sync. Kandev labels the time as an estimate and does
not convert the duration to a durable predicted timestamp.

## Scenarios

- **GIVEN** an approved open PR with successful required checks on a branch
  without a required merge queue, **WHEN** the user activates the merge action,
  **THEN** GitHub merges it and Kandev reports that the PR merged.
- **GIVEN** an approved open PR with successful required checks on a branch
  that requires a merge queue, **WHEN** the user activates the merge action,
  **THEN** GitHub accepts it into the queue and Kandev reports that the PR was
  added to the merge queue.
- **GIVEN** a PR already in the merge queue, **WHEN** the merge request is
  repeated, **THEN** Kandev treats GitHub's idempotent response as queued and
  does not report a failure.
- **GIVEN** a linked open PR with an active GitHub merge queue entry, **WHEN**
  Kandev completes a status sync, **THEN** its task and compact PR indicators
  use the dedicated queue color and its hover, drawer, popover, and detail
  surfaces describe the queue state, position, and available merge estimate.
- **GIVEN** GitHub reports an active queue entry without an estimated time to
  merge, **WHEN** a queue surface renders, **THEN** it shows queue state and
  position and omits the estimate without placeholder or fallback data.
- **GIVEN** a linked PR with a stored queue state, **WHEN** a REST or `gh pr
  view` feedback refresh completes without queue data, **THEN** the stored
  queue state, position, estimate, and queued UI remain unchanged.
- **GIVEN** a linked queued PR is removed from the queue while remaining open,
  **WHEN** a later GraphQL status sync observes no merge queue entry, **THEN**
  Kandev clears the queued UI and resumes the normal review, CI, and merge
  status color.
- **GIVEN** a linked queued PR reaches a merged or closed state, **WHEN** any
  authoritative lifecycle sync completes, **THEN** the terminal state replaces
  the queued presentation.
- **GIVEN** a draft, conflicted PR, failed checks, changes requested, or missing
  required approvals, **WHEN** the PR surface renders, **THEN** no merge or
  queue action is available.
- **GIVEN** GitHub rejects an otherwise eligible merge request, **WHEN** the
  action completes, **THEN** Kandev shows the provider error and leaves the
  action available for retry.
- **GIVEN** a phone-sized task view with an eligible queue-required PR, **WHEN**
  the user opens Review and activates the action, **THEN** the queued outcome is
  visible, the action is touch-usable, and the document has no horizontal
  overflow.

## Out of Scope

- Displaying or navigating the full merge queue.
- Removing a pull request from the merge queue.
- Selecting between direct merge and merge queue when GitHub policy permits
  both; GitHub's `default` behavior remains authoritative.
- Changing Kandev's independent CI auto-merge automation setting.
- GitLab merge-request behavior.

## Implementation Plans

- [Queue-status visibility plan](../../plans/github-pr-merge-queue-status/plan.md)
- [Original queue-aware merge action plan](../../plans/github-pr-merge-queue/plan.md)
