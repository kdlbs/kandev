---
status: draft
system: ci
requirements:
  - REQ-CI-MERGE-APPROVAL-001
---

# Contributor merge approval revocation system design

## Purpose and boundaries

This design defines the base-controlled GitHub Actions workflow that invalidates
the `ready-to-merge` approval after a non-write contributor push. The CI system
owns the workflow event, trust decision, permissions, and contract tests. The
GitHub integration owns the provider's merge queue and auto-merge APIs; this
workflow only invokes those existing GitHub operations to remove state made
unsafe by the push.

The workflow does not inspect the pull-request diff, run repository code, or
decide whether the new revision is mergeable. A maintainer must apply the label
again after reviewing the new revision.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-CI-MERGE-APPROVAL-001` | [Event and trust contract](#event-and-trust-contract), [Control flow](#control-flow), [Security](#security), [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

| Component | Responsibility |
| --- | --- |
| `.github/workflows/revoke-ready-to-merge.yml` | Runs the cleanup on base-controlled `pull_request_target` synchronize events. |
| `actions/github-script` | Executes the pinned, trusted API orchestration without checking out repository content. |
| GitHub collaborator permission API | Resolves the current repository permission of the synchronize event sender. |
| GitHub label REST API | Removes the exact `ready-to-merge` label from the pull request. |
| GitHub pull-request GraphQL API | Reads active auto-merge and queue state, then disables or dequeues each active state. |
| `.github/scripts/revoke-ready-to-merge-workflow-contract_test.py` | Protects the event, pusher identity, permission, cleanup mutations, and permission boundary. |
| `.github/workflows/lint-action-pinning.yml` | Runs the workflow contract test and action-pinning checks. |

## Event and trust contract

The workflow subscribes only to:

```yaml
on:
  pull_request_target:
    types: [synchronize]
```

`github.event.sender.login` is the pusher identity for the decision. The
workflow asks GitHub for that user's current repository permission through
`GET /repos/{owner}/{repo}/collaborators/{username}/permission`.

The REST response's legacy `permission` value is the normalized decision:

- `write` and `admin` are write-capable and exempt from cleanup.
- `read`, `none`, a missing value, a missing sender, a `404`, or another
  lookup failure is untrusted and follows the cleanup path.

The workflow first checks the event payload's label snapshot for the exact
`ready-to-merge` name. This prevents a delayed workflow from removing a label
that a maintainer added after an unlabelled push. A current label removal is
still idempotent when a concurrent maintainer action removed it first.

## Data and contracts

The workflow uses only bounded event and API values:

- repository owner and name from `context.repo`;
- the pull-request number from `context.payload.pull_request.number`;
- the pusher login from `context.payload.sender.login`;
- the exact label name `ready-to-merge`;
- the pull request node ID and optional `autoMergeRequest` and
  `mergeQueueEntry` fields from a fresh GraphQL read.

The GraphQL read is equivalent to:

```graphql
query($owner: String!, $repo: String!, $number: Int!) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      id
      autoMergeRequest { id }
      mergeQueueEntry { id }
    }
  }
}
```

For each non-null state, the workflow invokes the matching mutation:

```graphql
mutation($id: ID!) {
  disablePullRequestAutoMerge(input: { pullRequestId: $id }) {
    pullRequest { id }
  }
}
```

```graphql
mutation($id: ID!) {
  dequeuePullRequest(input: { id: $id }) {
    mergeQueueEntry { id }
  }
}
```

The workflow does not change any other label, merge method, branch, review,
check, or queue entry.

## Control flow

1. GitHub starts the trusted base workflow for a `synchronize` event.
2. The job resolves the event sender's current repository permission.
3. A `write` or `admin` permission ends the job without any pull-request
   mutation.
4. An untrusted path requires `ready-to-merge` in the event label snapshot. If
   it is absent, the job ends without changing merge state.
5. The workflow removes only `ready-to-merge`. It records an absent-label
   response as an idempotent outcome and continues to merge-state cleanup.
6. The workflow reads the current pull request through GraphQL after label
   removal. It disables an active auto-merge request and dequeues an active
   queue entry as independent operations.
7. The job reports the label, auto-merge, and queue outcomes. It attempts all
   independent cleanup operations before failing the job for an unexpected API
   error.

The concurrency group is keyed by pull-request number and does not cancel an
older run. This ensures that a non-write push cannot be silently skipped when
several synchronize events arrive close together.

## Failure and recovery

- A missing or failed permission lookup fails closed into the cleanup path and
  marks the workflow result as failed or warning-visible according to the
  lookup error, so a maintainer can investigate a false revocation.
- A missing label is a successful no-op when the event snapshot did not contain
  it. A concurrent deletion is also idempotent.
- A missing auto-merge request or queue entry is a successful no-op.
- Label, auto-merge, and queue operations are attempted independently. One
  failure does not prevent the workflow from attempting the other cleanup
  operations.
- An unexpected API failure makes the workflow fail after the attempted
  cleanup. The label removal is the first safety action, so a failed later
  mutation remains visible to maintainers for manual cleanup.
- Repeated synchronize events converge on the same clean state and never
  reapply the merge approval.

## Persistence

The workflow has no repository or Kandev persistence. GitHub remains the source
of truth for labels, auto-merge requests, and queue entries. The event payload
and fresh API read provide the state needed for one run.

## Security

- `pull_request_target` runs the workflow definition from the trusted base
  branch and is required so a fork event can receive the trusted write-capable
  base `GITHUB_TOKEN`.
- The job does not use `actions/checkout`, execute a shell command from the
  pull request, read pull-request files, or expose repository secrets.
- The job declares `pull-requests: write` and does not declare `issues: write`.
  GitHub's label API permits pull-request write permission for labels on pull
  requests, and the same permission is used for the pull-request mutations.
- The external action reference is pinned to a full commit SHA. Contract tests
  reject a workflow that changes the event, permission identity, cleanup
  mutations, or trusted-content boundary.
- Event and API values are passed as API parameters, not interpolated into
  executable workflow source.

## Observability

The workflow log and step summary identify the pull request, pusher permission
classification, and each cleanup result without printing tokens. The contract
test provides local evidence for the event filter, permission gate, mutation
coverage, action pin, and least-privilege permission block.

## Related decisions

- [Revoke merge approval after an untrusted pull-request push](../../../decisions/2026-08-31-revoke-merge-approval-after-untrusted-push.md)
- [Use One Maintainer Approval Label for Contributor PR Automation](../../../decisions/2026-08-24-unified-fork-approval-label.md)
