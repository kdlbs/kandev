---
id: "03-evaluator"
title: "Pure classification evaluator"
status: done
wave: 3
depends_on: ["02-ledger-store"]
plan: "plan.md"
spec: "../../specs/task-delivery-ledger/spec.md"
---

# Task 03: Pure classification evaluator

One function, no database and no git:

```go
func Classify(obs Observation) Classification
```

`Observation` is the input struct defined in the plan's **`observe.go`** table
(task 04 populates it; define it here if task 04 has not landed, and task 04
consumes it as given). `Classification` carries `Outcome`, `DeliveryBasis`,
`DeliveryRef`, `ReachedDefaultAt`, `ReachedDefaultBasis`, `ReachedDefaultRef` and
`ObservedBranchCommits`.

Keeping this pure is the point: it makes the spec's entire Classification and
Squash-merge scenario set a table-driven unit test with no fixtures, no temp
database and no git.

## Rules

The spec's five rules in exactly this order, first match wins:

1. `pr_merge` — a merged, non-detached provider row exists. Basis
   `provider_pr_merged`.
2. `direct_commit` — two or more distinct non-empty head commits on snapshots
   whose `branch` equals the repository's `default_branch`. Basis
   `default_branch_commit`.
3. `unknown` — commits observed on a non-default branch
   (`MaxAhead > 0` or two or more distinct heads) and neither rule 1 nor 2
   matched. Basis `reached_default_unattributed` when `reached_default_at` is
   set, `branch_commits_unmerged` otherwise.
4. `no_delivery_observed` — at least one snapshot exists and every one shows
   `ahead = 0` with a single distinct head commit. Basis `no_commits_observed`.
5. `unknown` — no snapshot and no provider row. Basis `no_observations`.

Rule 3 is deliberate, not a gap. The outcome enum is fixed at four values, so
"reached the default branch by a route nobody observed" is recorded in
`reached_default_at` / `reached_default_basis` rather than being forced into
`pr_merge` or `direct_commit`, either of which would assert a route that was not
observed.

Rules 4 and 5 must not collapse into each other: "we looked and there was
nothing" and "we never looked" are different facts, and `SnapshotCount == 0` is
what separates them.

## Default-branch observation

Computed here from three of the four bases: `provider_pr_merged` (a merged
provider row whose base branch equals the default branch),
`default_branch_commit`, and `ancestor_of_default` (task 05 supplies the boolean;
treat an absent probe as no evidence). `push_webhook_default` is in the
vocabulary but is never produced by this card — the webhook subscriber is
explicitly out of scope.

When `obs.DefaultBranch == ""`, rule 2 and the default-branch observation are
both skipped and the basis records `default_branch_unknown`. Classification still
falls through to rules 3-5; it does not error.

- **Acceptance:**
  1. Every Classification scenario in the spec has a corresponding table case
     and passes, including the rule-4 / rule-5 split and the
     `default_branch_unknown` fallthrough.
  2. A negative or absent ancestry result produces no `reached_default_at` and
     does not change the outcome.
  3. `Classify` performs no I/O: it takes no `context.Context`, no `*sqlx.DB` and
     no `exec.Cmd`.

- **Verification:**
  `cd apps/backend && go test -run TestClassify ./internal/delivery/... && make lint`

- **Files likely touched:**
  - `apps/backend/internal/delivery/evaluator.go`
  - `apps/backend/internal/delivery/evaluator_test.go`
  - `apps/backend/internal/delivery/models.go` (add `Observation` /
    `Classification` if task 04 has not already)

- **Dependencies:** Task 02 (vocabularies and `rank`).

- **Parallelism:** parallel-safe with task 04 — different files in the same
  package, no shared schema or migration. Coordinate only on the `Observation`
  struct: whichever task lands first defines it.

- **Inputs:** Spec **Classification**, **Default-branch observation**,
  **Failure modes**, and the **Classification** + **Squash-merge and negative
  evidence** scenarios.

- **Output contract:** summary, files changed, tests run with counts, blockers,
  risks, and task/plan status update in the same conversation.

## Results

**Files changed:** `internal/delivery/evaluator.go`, `internal/delivery/evaluator_test.go`
(table-driven `TestClassify*` cases), `internal/delivery/models.go`
(`Observation` / `Classification` types, shared with task 04).

**Commands run:**
- `cd apps/backend && go test -run TestClassify ./internal/delivery/...` →
  `ok`, 25 subtests pass, 0 fail — covers the rule-4/rule-5 split, the
  `default_branch_unknown` fallthrough (rules 3-5 still evaluated), and the
  squash-merge / negative-evidence scenarios.
- `make lint` — clean.

**Acceptance verification:** #1 (every spec Classification scenario has a
table case) and #2 (negative/absent ancestry produces no
`reached_default_at` and does not change the outcome) are covered by the
`TestClassify*` table cases above. #3 (`Classify` performs no I/O) verified
by reading `evaluator.go`'s `Classify` signature directly — it takes an
`Observation` value and returns a `Classification` value; no
`context.Context`, `*sqlx.DB`, or `exec.Cmd` appears anywhere in the
function or its call graph within this file.

**Security/trust and external side-effects:** None — `Classify` is pure
computation over already-fetched data.
applicable, or explicitly state `None`.
