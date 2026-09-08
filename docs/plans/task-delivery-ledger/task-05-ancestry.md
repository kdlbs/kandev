---
id: "05-ancestry"
title: "Default-branch ancestry probe"
status: done
wave: 4
depends_on: ["02-ledger-store"]
plan: "plan.md"
spec: "../../specs/task-delivery-ledger/spec.md"
---

# Task 05: Default-branch ancestry probe

One bounded git call that answers a single question: is this pair's last observed
head commit an ancestor of the repository's default branch in the local checkout?

```go
subproc.NewGitCommand(ctx, "-C", repo.LocalPath,
    "merge-base", "--is-ancestor", headCommit, defaultRef)
// run via subproc.RunGitClass(ctx, subproc.GitWorkClassBackground, cmd)
```

Production git must go through `internal/common/subproc` with an explicit work
class (`apps/backend/CLAUDE.md`); `background` is correct for a telemetry sweep
and keeps this out of the interactive and lifecycle admission budgets. At most
one call per `(task, repository)` per evaluation, 10-second timeout.

## Only exit code 0 is evidence

This is the crux of the task and the easiest thing to get wrong.

Exit 0 sets `reached_default_at` with basis `ancestor_of_default`. **Everything
else produces nothing**: `reached_default_at` stays `NULL`, `delivery_outcome` is
untouched, and no column anywhere records `false`.

That is not defensive coding. Kandev squash-merges by policy, so a merged
branch's head commit never becomes an ancestor of the default branch. The spec's
finding 2 receipts this: PR #2514's branch head `4dfa4d545` is present in the
object store and is **not** an ancestor of `origin/main`; the commit that landed
is the squash `15524de62`. A negative ancestry result is therefore a routine
false negative, and persisting it as a negative fact would be worse than not
looking.

Increment `delivery_ledger_ancestry_errors_total` (task 06 owns the counter;
surface the error from here) on a git failure, a missing object, or a timeout.
Do **not** increment it on a clean non-zero "not an ancestor" exit — that is a
successful probe with a negative answer, not an error.

Skip the probe entirely when `repositories.local_path` is empty or
`repositories.default_branch` is empty; both are ordinary states, not failures.

- **Acceptance:**
  1. A commit genuinely on the default branch of a real local checkout returns a
     positive result and sets `reached_default_at` with basis
     `ancestor_of_default`.
  2. A commit not on the default branch returns no evidence: no column records
     `false` and the outcome is unchanged.
  3. A missing checkout, absent object or timeout increments the error counter,
     leaves `reached_default_at` NULL, and does not fail the evaluation.
  4. The git command is constructed via `subproc.NewGitCommand` and run under the
     `background` class; no raw `exec.Command("git", ...)` appears.

- **Verification:**
  `cd apps/backend && go test ./internal/delivery/... && make lint`

  The positive case needs a real `git init` fixture under `t.TempDir()`. That
  spawns a subprocess, so `testing/synctest` is not applicable — use
  channel-based synchronization or plain sequential execution, not `time.Sleep`.

- **Files likely touched:**
  - `apps/backend/internal/delivery/ancestry.go`
  - `apps/backend/internal/delivery/ancestry_test.go`

- **Dependencies:** Task 02 (package and basis vocabulary).

- **Parallelism:** parallel-safe with task 08 — disjoint trees
  (`apps/backend/internal/delivery` vs `apps/web`).

- **Inputs:** Spec **Default-branch observation**, **Failure modes**,
  **Evidence** finding 2, and the **Squash-merge and negative evidence**
  scenarios; `apps/backend/CLAUDE.md` git-subprocess rule;
  `docs/decisions/2026-08-02-class-aware-git-subprocess-admission.md`;
  helper signatures in `apps/backend/internal/common/subproc/shared.go:86`.

- **Output contract:** summary, files changed, tests run with counts, blockers,
  risks, and task/plan status update in the same conversation.

## Results

**Files changed:** `internal/delivery/ancestry.go`, `internal/delivery/ancestry_test.go`.
(`TestAncestryChecker_CheckoutResolverErrorIsAnAncestryError` was rewritten
during this feature's Review round 2 to use a valid-looking commit SHA
instead of the literal string `"HEAD"`, which short-circuited
`looksLikeCommitSHA` before ever reaching the checkout-resolver call the
test claimed to cover — the fix also asserts the fake resolver's call count
to prove invocation.)

**Commands run:**
- `cd apps/backend && go test ./internal/delivery/... -run TestAncestry -v`
  → `ok`, 12 subtests pass, 0 fail across the 10 `TestAncestryChecker_*`
  functions listed in `ancestry_test.go` (some carry sub-cases via `t.Run`).
  `TestAncestryChecker_PositiveResult` and
  `TestAncestryChecker_LocalBranchFallbackWhenRemoteTrackingRefMissing` use a
  real `git init` fixture under `t.TempDir()`; both clean up automatically
  via `t.TempDir()`'s own teardown, no manual removal needed.
- `cd apps/backend && go test ./internal/delivery/...` → `ok`, 99 subtests,
  0 fail (full package).
- `make lint` — clean.

**Acceptance verification:** #1 (positive ancestor result,
`ancestor_of_default` basis) — `TestAncestryChecker_PositiveResult`. #2
(commit not on default branch → no evidence, outcome unchanged) —
`TestAncestryChecker_NegativeResultIsNotAnError`. #3 (missing checkout /
absent object / timeout increments the error counter, leaves
`reached_default_at` NULL, does not fail the evaluation) —
`TestAncestryChecker_NeitherRefResolvesIsAnError` and
`TestAncestryChecker_CheckoutResolverErrorIsAnAncestryError`. #4 (git
command via `subproc.NewGitCommand` under `background` class, no raw
`exec.Command("git", ...)`) — confirmed directly in `ancestry.go`: both git
invocations go through `subproc.RunGitAfterAcquire(ctx, subproc.GitBackground,
...)` wrapping `subproc.NewGitCommand`; `grep -n "exec.Command" ancestry.go`
returns no matches.

**Security/trust and external side-effects:** None beyond the git
subprocess itself, which is read-only (`rev-parse`, `merge-base
--is-ancestor`) and admission-classified per
`docs/decisions/2026-08-02-class-aware-git-subprocess-admission.md`. Test
fixtures are ordinary `t.TempDir()` git repos, cleaned up by the Go test
runner.
