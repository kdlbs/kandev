---
id: "04-observations"
title: "Candidate pairs and evidence queries"
status: done
wave: 3
depends_on: ["02-ledger-store"]
plan: "plan.md"
spec: "../../specs/task-delivery-ledger/spec.md"
---

# Task 04: Candidate pairs and evidence queries

Populate the `Observation` the evaluator consumes: discover candidate
`(task, repository)` pairs and gather each pair's evidence from the task tables
and all three provider tables. Every table involved lives in the same database,
so this is plain SQL, not a cross-service call.

## Candidate pairs

The union of `task_repositories`, non-empty `task_sessions.repository_id`, and
the `(task_id, repository_id)` of `github_task_prs`, `gitlab_task_mrs` and
`azure_devops_task_prs` — restricted to `repository_id <> ''` and to repositories
with `deleted_at IS NULL`. A soft-deleted repository freezes evaluation for its
pairs: existing rows are retained and `last_evaluated_at` stops advancing.

A task with no repository yields **no ledger row**. Absence means "not
applicable" and is a different fact from `unknown`; do not synthesize a row with
an empty `repository_id` to make the join tidy.

Ordering is `tasks.created_at ASC, tasks.id ASC, repositories.id ASC`. It must be
total and deterministic so an interrupted sweep resumes on the same order and
re-evaluates only what it did not reach.

## Per-pair evidence

Fill the `Observation` fields from the plan's table. Snapshots reach the pair
through `task_session_git_snapshots.session_id -> task_sessions.id`, matching on
`task_sessions.task_id` **and** `task_sessions.repository_id`.

Three facts from the reference-store measurement that change the SQL:

- **`ahead` is base-branch-relative, not upstream-relative**
  (`internal/agent/runtime/lifecycle/event_types.go:288-294`). It counts commits
  the branch has that its base does not, and it does not fall to zero on push. It
  is a valid "this branch carries work" signal and an invalid "this session
  authored work" signal. Use it only for `observed_branch_commits` and rule 3.
- **An empty `head_commit` is not a distinct head.** Filter `head_commit <> ''`
  in every `COUNT(DISTINCT ...)`, or a pair with two empty snapshots reads as
  having authored a commit.
- **A session with an empty `repository_id`** contributes to no pair. Count it
  toward `delivery_ledger_sessions_unattributed_total` (task 06 owns the
  counter; expose the count on the result here). Three such sessions exist in
  the reference store.

## Provider rows

```sql
-- GitHub: detachment applies here and only here.
SELECT pr_url, merged_at, pr_number, base_branch FROM github_task_prs
 WHERE task_id = ? AND repository_id = ?
   AND merged_at IS NOT NULL AND detached_at IS NULL
-- GitLab: no detached_at column exists; the predicate is vacuously true.
SELECT mr_url, merged_at, mr_iid, base_branch FROM gitlab_task_mrs
 WHERE task_id = ? AND repository_id = ? AND merged_at IS NOT NULL
-- Azure: no merged_at column exists; status carries merge.
SELECT pull_request_url, NULL, pull_request_id, target_branch
  FROM azure_devops_task_prs
 WHERE task_id = ? AND repository_id = ? AND status = 'completed'
```

Do not add a `detached_at` column to the GitLab or Azure tables, and do not add
`merged_at` to Azure. Those are provider-table changes this card does not own.
An Azure status outside the known vocabulary leaves the pair at `unknown` with
basis `branch_commits_unmerged` rather than being guessed.

Wrap each provider query so `db.IsMissingTableError` (task 01) yields zero rows
instead of failing the whole evaluation — a database where a provider store never
initialized has no such table.

## Deterministic reference selection

- `delivery_ref` for `pr_merge`: earliest `merged_at`; NULL `merged_at` (every
  Azure row) sorts **last**; ties broken by number ascending; further ties by
  provider name ascending in the order `azure_devops`, `github`, `gitlab`.
- `delivery_ref` for `direct_commit`: among default-branch snapshots sharing the
  greatest `created_at`, the lexicographically greatest `head_commit`.
- `observed_branch_commits`: `MAX(ahead)`, no tiebreak needed.

Both orderings must be total. Anything that falls back to a random identifier or
a dialect-specific row number will make the idempotency test flap.

- **Acceptance:**
  1. Candidate discovery covers all three sources, excludes empty
     `repository_id` and soft-deleted repositories, and produces no row for a
     task with no repository.
  2. A merged GitLab MR is found; a detached GitHub PR is not; an absent
     provider table yields zero rows rather than an error.
  3. Every `delivery_ref` tiebreak in the spec resolves as specified and is
     stable across repeated evaluation.

- **Verification:**
  `cd apps/backend && go test ./internal/delivery/... && make lint`

- **Files likely touched:**
  - `apps/backend/internal/delivery/observe.go`
  - `apps/backend/internal/delivery/observe_test.go`
  - `apps/backend/internal/delivery/models.go` (add `Observation` if task 03 has
    not already)

- **Dependencies:** Task 02 (package and types); task 01
  (`db.IsMissingTableError`).

- **Parallelism:** parallel-safe with task 03 — different files in the same
  package. Coordinate only on the `Observation` struct.

- **Inputs:** Spec **Candidate pairs**, **Ordering, idempotency, concurrency**,
  **Failure modes**, and **Evidence the classification already works**
  findings 1 and 3; plan decisions D3, D4 and D5; table shapes at
  `internal/task/repository/sqlite/base_schema.go:762`,
  `internal/github/store.go:107`, `internal/gitlab/store.go:70`,
  `internal/azuredevops/store.go:37`.

- **Output contract:** summary, files changed, tests run with counts, blockers,
  risks, and task/plan status update in the same conversation.

## Results

**Files changed:** `internal/delivery/observe.go`, `internal/delivery/observe_test.go`,
`internal/delivery/provider_seed_test.go` (seed helpers reused by later
tasks' tests), `internal/delivery/models.go` (`Observation`, shared with
task 03).

**Commands run:**
- `cd apps/backend && go test -run 'TestCandidates|TestSnapshotsForPair|TestProvidersForPair|TestGitHubPRs|TestGitLabMRs|TestAzureDevOpsPRs' ./internal/delivery/...`
  → `ok`, 9 subtests pass, 0 fail:
  `TestCandidates_ExcludesMissingRepositoryAndTask`,
  `TestCandidates_ExcludesMissingTask`,
  `TestCandidates_TaskWithNoRepositoryProducesNoRow`,
  `TestSnapshotsForPair_JoinsThroughSessions`,
  `TestSnapshotsForPair_NullHeadCommitNormalizesToEmpty`,
  `TestProvidersForPair_MissingTablesTolerated`,
  `TestGitHubPRs_MergedAndDetachedFromColumns`,
  `TestGitLabMRs_MergedFromColumnAndScopeIsProjectPath`,
  `TestAzureDevOpsPRs_StatusCaseInsensitiveAndUnrecognisedIsNotMerged`.
- `cd apps/backend && go test ./internal/delivery/...` → `ok`, 99 subtests
  pass, 0 fail (full-package run, confirms no interaction with tasks 02/03/05/06).
- `make lint` — clean.

**Acceptance verification:** #1 (candidate discovery, missing-repository
exclusion, no row for a task with no repository) is covered by the three
`TestCandidates_*` cases above. #2 (merged GitLab MR found, detached GitHub
PR not, absent provider table yields zero rows) is covered by
`TestGitHubPRs_MergedAndDetachedFromColumns`,
`TestGitLabMRs_MergedFromColumnAndScopeIsProjectPath`, and
`TestProvidersForPair_MissingTablesTolerated`. #3 (`delivery_ref` tiebreaks)
lives in `evaluator.go`'s `selectProviderRef`, not `observe.go`, and is
covered by the `TestClassify*` table cases (task 03's verification run).

**Security/trust and external side-effects:** None — read-only queries
against existing provider tables; `db.IsMissingTableError` (task 01)
classifies a missing table as zero rows rather than surfacing a raw driver
error.
teardown evidence. Record security/trust and external side-effect boundaries when
applicable, or explicitly state `None`.
