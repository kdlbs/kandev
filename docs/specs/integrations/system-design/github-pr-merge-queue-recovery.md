---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-001
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003
created: 2026-08-24
owners:
  - Kandev
---
# GitHub PR Merge Queue Recovery System Design

## Purpose and boundaries

This design extends the existing GitHub PR poller and CI automation evaluator.
It retains queue-removal evidence, sends repair prompts, and re-arms auto-merge
after a new pull-request head appears.

GitHub remains the source of truth for queue policy and merge eligibility.
Kandev keeps the existing one-minute poll interval and queue-aware merge API.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002` | [Requeue flow](#requeue-flow), [Failure and recovery](#failure-and-recovery) |
| `REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-001` | [Provider observation](#provider-observation), [Persistence](#persistence) |
| `REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002` | [Auto-fix flow](#auto-fix-flow) |
| `REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003` | [Requeue flow](#requeue-flow) |

## Provider observation

`apps/backend/internal/github/graphql.go` extends the batched pull-request
selection with these fields:

```graphql
headRefOid
mergeQueueEntry {
  id
  state
  position
  estimatedTimeToMerge
  headCommit { oid }
}
timelineItems(last: 1, itemTypes: REMOVED_FROM_MERGE_QUEUE_EVENT) {
  nodes {
    ... on RemovedFromMergeQueueEvent {
      id
      createdAt
      reason
      beforeCommit { oid }
    }
  }
}
```

The timeline event remains available after `mergeQueueEntry` becomes `null`.
This property closes the gap between queue attempts and the one-minute poll.

The GraphQL result records separate populated flags for queue membership and
queue-removal history. A REST or `gh` read cannot clear either observation.

## Persistence

`github_task_prs` adds these snapshot fields:

- `head_sha` stores the current pull-request head.
- `merge_queue_entry_id` stores the active queue entry ID.
- `merge_queue_entry_head_sha` stores the head for the active queue attempt.
- `merge_queue_last_removal_id` stores the latest removal event ID.
- `merge_queue_last_removed_at` stores the provider event time.
- `merge_queue_last_removal_reason` stores the provider reason.
- `merge_queue_last_removal_before_sha` stores the optional event commit.

An authoritative `mergeQueueEntry: null` clears only the active entry fields.
The last removal fields remain until a newer removal replaces them.

Every `TaskPR` write path must preserve these fields. The normal task-PR event
publishes them to current automation and UI consumers.

`github_task_ci_pr_state` adds these fields:

- `last_queue_attempt_head_sha` stores the head from the last accepted or
  observed queue attempt.
- `last_queue_fix_event_id` stores the last removal that consumed an auto-fix
  round.
- `last_queue_removal_cause` stores the normalized cause for the latest
  removal. Values are `checks_failed`, `checks_timed_out`, `conflict`,
  `manual`, `branch_protection`, and `unknown`.

The main merge-queue design owns the automatic attempt journal, typed error
kind, and explicit retry authorization. Recovery reads that journal but does not
define a second attempt state machine.

## Auto-fix flow

`handleTaskPRCIAutomationWithRefresh` evaluates queue evidence before normal PR
feedback. A new removal is new when its event ID differs from
`last_queue_fix_event_id`.

The evaluator classifies a removal as actionable when one of these facts
exists:

- The provider reason matches a reviewed failure or timeout form.
- The pull request has a merge conflict.
- The active queue entry reached GitHub's `UNMERGEABLE` state.

The reason mapper accepts only reviewed provider-generated forms. It maps
manual removal, branch-protection failure, and all unknown text to a
non-actionable cause. Structured logs record unknown forms for later review.

GitHub documents `beforeCommit` as the commit before the removal event. It does
not document this field as the temporary merge-group commit. The evaluator
does not fetch check runs for this commit or use its checks as merge-group
evidence.

When Kandev has an exact merge-group commit from a supported provider source,
the existing `ListCheckRuns` boundary can add failed check names and links.
This polling-only version does not infer that identity from the removal event.

The auto-fix checkpoint adds a queue-removal item with its event ID, normalized
cause, raw reason, time, conflict state, and available check snapshots.
Existing sanitization and `sysprompt.Wrap` boundaries treat all provider text
as untrusted data.

The existing auto-fix dispatch path owns session selection, durable queuing,
coalescing, and the 10-round limit. One accepted queue-recovery prompt records
`last_queue_fix_event_id` with the normal fix checkpoint.

Enabling auto-fix runs the normal evaluator. If the latest retained removal is
actionable, belongs to the current head, and has not been checkpointed, the
evaluator accepts one repair round. Enabling the option while the pull request
is still queued only arms this behavior for a later removal.

## Requeue flow

The merge signature adds `TaskPR.HeadSHA`. This change makes a new commit a
clear re-arm signal, even when file counts and gate values stay unchanged.

Kandev records the current merge signature and
`last_queue_attempt_head_sha` when either condition occurs:

- Kandev receives a successful `queued` merge outcome.
- The poller observes an active queue entry that another client created.

This observation also handles a user who enables auto-merge while the pull
request is already queued. The evaluator adopts the active entry and returns
without calling the merge API.

Adoption records the attempt as `accepted`. It clears a stale error only when
the stored error kind is `auto_merge`. A merged observation applies the same
reconciliation rule.

When Kandev first observes a removal without an earlier active observation, it
records a conservative baseline for the current head. This fallback prevents
an immediate same-head requeue.

The normal readiness gates remain unchanged. Auto-merge submits a new request
only when the head SHA changes and all gates pass. The existing
`MergePRForAutomation` call uses GitHub's `merge_action=default` behavior.
Enabling auto-merge after removal does not weaken this condition.

This state machine has these transitions:

```text
ready head A -> queued head A -> removed head A -> blocked head A
blocked head A -> auto-fix or manual repair -> ready head B -> queued head B
```

The event ID deduplicates repair work. The head SHA deduplicates queue work.

The option combinations produce these outcomes:

- Auto-fix only sends one actionable removal to the agent. It does not requeue.
- Auto-merge only adopts an active attempt or queues a later eligible new head.
  It does not repair the failure or requeue the removed head.
- Both options run the full repair-and-requeue flow.

## Failure and recovery

- If the timeline query fails, Kandev preserves the prior removal snapshot.
- If the reason mapper does not recognize a value, Kandev records the value as
  `unknown`. It does not start auto-fix from that value.
- If removal evidence is not actionable, Kandev records the status only.
- If the current head SHA is absent, Kandev does not requeue automatically.
- If GitHub rejects a requeue request, the existing error row shows the error.
- If an `in_flight` attempt survives a restart, Kandev reconciles queue and
  merged state before it fails the attempt and offers explicit retry.
- If a repair produces no new head, Kandev waits. A user can repair or requeue
  the pull request on GitHub.

## Permissions and security

The timeline and check reads use the workspace automation identity. The merge
request uses the existing workspace-scoped automation write identity.

Kandev does not trust the removal reason, check name, check output, or URL as
instructions. Prompt rendering keeps the existing untrusted-data boundary.

## Observability

Structured logs identify the task, repository, pull request, removal event,
head SHA, normalized cause, action classification, and decision. Logs do not
include check output.

The existing per-PR `last_error` and automation-state event expose failed
reads, dispatches, and queue requests to the automation surface.

## Related decisions and designs

- [Bind automatic merge attempts to the reviewed head](../../../decisions/2026-08-28-bind-github-auto-merge-attempts-to-reviewed-head.md)
- [GitHub PR Merge Queue requirements](../requirements/github-pr-merge-queue.md)
- [Task PR Automation Controls design](../../ui/system-design/ci-pr-automation-01.md)
- [PR agent notification decision](../../../decisions/0051-pr-agent-notifications-extend-task-pr-automation.md)
