---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001
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

## Components and responsibilities

### GitHub provider boundary

- `apps/backend/internal/github/client.go` defines the typed merge outcome.
- `gh_client.go` and `pat_client.go` call GitHub's asynchronous merge endpoint.
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

### Task projection

- `apps/backend/internal/task/statussummary` reduces full pull-request state to
  a bounded task status.
- `apps/backend/internal/backendapp/status_summary_adapter.go` supplies the
  stored queue state during live updates and restart hydration.

### Web application

- `apps/web/lib/api/domains/github-pr-api.ts` types the merge result.
- `apps/web/lib/types/github.ts` defines the queue-state and `TaskPR` fields.
- `pr-merge-button.tsx` controls eligibility, submission, feedback, and refresh.
- `pr-merge-queue-status.tsx` normalizes queue presentation and duration copy.
- The task icon, status chip, summary, and detail components reuse that
  presentation model.

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

## State transitions

- An open pull request with no observed queue entry is not queued.
- A non-null authoritative entry creates or updates active queue membership.
- A later authoritative null removes active queue membership.
- A merged or closed state clears queue metadata and wins presentation priority.
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

## Persistence

The queue fields use additive SQLite columns in `github_task_prs`. Fresh schema,
migration, create, replace, restore, and update paths round-trip the complete
entry. Existing rows start with no queue entry and converge after a successful
GraphQL status sync.

The estimate remains a duration. Kandev does not convert it to a durable target
timestamp because GitHub can change the estimate on each observation.

## Security

The merge action uses the active workspace's personal-write GitHub route. It
requires the same pull-request permissions as GitHub's merge API. Kandev does
not bypass repository rules or elevate the user.

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

## Verification evidence

Focused backend coverage exists in the GitHub client, GraphQL conversion,
storage, synchronization, and task-status packages. Frontend unit coverage
exists for merge eligibility, merge outcomes, queue formatting, and status
priority. Playwright coverage exists in:

- `apps/web/e2e/tests/pr/pr-merge-queue.spec.ts`
- `apps/web/e2e/tests/pr/mobile-pr-merge-queue.spec.ts`

## Related delivery records

- [Queue-status visibility plan](../../../plans/github-pr-merge-queue-status/plan.md)
- [Original queue-aware merge action plan](../../../plans/github-pr-merge-queue/plan.md)
