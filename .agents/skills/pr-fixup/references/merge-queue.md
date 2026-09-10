# Merge Queue Coordination

Load this reference when the user requests merge-queue coordination, or when
authoritative GitHub data shows a non-null `mergeQueueEntry`. Queue mutations
are external writes: dequeue and enqueue only with explicit user
authorization. `scripts/pr-await` validates the pull-request head; it does not
prove the synthetic merge-group commit passed. For review-only work, report
queue membership as a mutation blocker and do not dequeue just to inspect or
reply.

## Identify the queue attempt

Use the pull-request GraphQL node ID for mutations. It is different from the
active merge-queue entry ID. Record the current pull-request head and base, the
queue entry state and position, and the synthetic commit used for its checks:

```bash
gh api graphql \
  -f query='query($owner:String!,$repo:String!,$number:Int!) {
    repository(owner:$owner,name:$repo) {
      pullRequest(number:$number) {
        id
        headRefName
        headRefOid
        baseRefName
        mergeQueueEntry {
          id
          state
          position
          headCommit {
            oid
            statusCheckRollup { state }
          }
        }
      }
    }
  }' -f owner=<owner> -f repo=<repo> -F number=<PR>
```

The `pullRequest.id` is the PR node ID. Do not pass
`mergeQueueEntry.id` to `dequeuePullRequest` or `enqueuePullRequest`.

## Authorized branch updates

1. Capture the reported PR head OID and authoritative base name. Confirm the
   linked-worktree preflight in `merge-conflicts.md` before mutating a branch.
2. If the PR is queued, dequeue it through the repository-approved path before
   pushing. A push rejected with `GH006` means the queue still owns the branch;
   do not retry the push or force-push around that protection. Re-query
   `pullRequest.mergeQueueEntry` until it is null after the dequeue request;
   queue state is authoritative while the mutation propagates.
3. Fetch the fresh base, merge or resolve it using the conflict procedure, run
   the affected gates, and commit with normal hooks. Recheck the remote head
   lease immediately before pushing.
4. After the push, verify the PR head OID, exact-head checks, review state, and
   mergeability. Every push or base reconciliation invalidates earlier CI and
   review evidence.
5. Re-enqueue only after the pushed head is verified. Pass the PR node ID and
   the newly verified head as `expectedHeadOid`; do not use a stale queue-entry
   ID or stale head. GitHub may restore queue membership automatically after
   required checks pass; if it does, monitor the existing entry instead of
   enqueueing it a second time.

The GraphQL mutation shapes are:

```bash
gh api graphql \
  -f query='mutation($prNodeId:ID!) {
    dequeuePullRequest(input:{id:$prNodeId}) {
      clientMutationId
    }
  }' -f prNodeId=<pull-request-node-id>

gh api graphql \
  -f query='mutation($pullRequestId:ID!,$expectedHeadOid:GitObjectID!) {
    enqueuePullRequest(input:{pullRequestId:$pullRequestId,expectedHeadOid:$expectedHeadOid}) {
      clientMutationId
    }
  }' -f pullRequestId=<pull-request-node-id> \
  -f expectedHeadOid=<verified-head-oid>
```

Use the repository's approved UI or API if these mutations are unavailable to
the current token. Do not substitute a queue-entry ID, and do not retry a
failed mutation without rereading the PR and queue state.

## Monitor the synthetic result

After enqueueing, reread `mergeQueueEntry` and its `headCommit` rollup. Treat
`QUEUED` and `AWAITING_CHECKS` as pending. A persistently `UNMERGEABLE` entry is
an explicit signal to stop, report the state, and obtain authorization before
dequeueing or reconciling again. Stop on `MERGED` or `CLOSED`, or on a clearly
documented external blocker. Map queue-removal timeline events and their
`beforeCommit.oid` to the corresponding `merge_group` run as described in
`ci-troubleshooting.md`; ordinary `gh pr checks` does not cover that synthetic
commit.

For a request to monitor until the PR is merged, include the pull request's
`state`, `mergedAt`, `mergeCommit { oid }`, and `isInMergeQueue` in each bounded
poll. Do not report completion from a clean head check, a non-null merge entry,
or a null `autoMergeRequest`: require `state=MERGED`, both merge fields, and
`isInMergeQueue=false`. Stop on `CLOSED` or a clearly documented external
blocker, and keep the final snapshot with the synthetic commit and queue state.

If the queue entry is `UNMERGEABLE` while the PR API reports
`mergeable=UNKNOWN`, fetch the authoritative base and run
`git merge-tree --write-tree HEAD origin/<base>` (or the equivalent current
base ref) before mutating the branch. Use the conflict procedure only when the
preflight finds a real conflict; otherwise treat the GitHub state as transient
and re-query it.

Recheck the PR head after any queue transition. A new push, base race, queue
ejection, or changed expected head starts a new evidence cycle: rerun the exact
head gates, then inspect the new merge-group commit and checks.
