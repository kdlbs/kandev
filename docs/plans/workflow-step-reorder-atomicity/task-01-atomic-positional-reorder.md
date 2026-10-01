---
id: "01-atomic-positional-reorder"
title: "Atomic positional workflow step reorder"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-STEP-ORDERING-001
  - REQ-TASKS-COMPLETION-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-ORDERING-001.1
  - AC-TASKS-WORKFLOW-STEP-ORDERING-001.2
  - AC-TASKS-WORKFLOW-STEP-ORDERING-001.3
  - AC-TASKS-WORKFLOW-STEP-ORDERING-001.4
  - AC-TASKS-WORKFLOW-STEP-ORDERING-001.5
  - AC-TASKS-COMPLETION-001.4
system_design:
  - ../../specs/tasks/system-design/workflow-step-ordering.md
---

# Task 01: Atomic Positional Reorder

## Summary

Make ordinary workflow step reorder commit one complete order while preserving
concurrently saved content. Recreate permanent deterministic regression tests
before production edits, then implement and deliver the focused repair.

## In scope

- Add the file-backed two-handle writer-acquisition-barrier regression for prompt,
  profile, completion, and representative other content preservation.
- Add second-write failure coverage asserting all positions and timestamps roll back.
- Validate the complete permutation in the repository transaction; reject duplicates,
  omitted, missing, and foreign IDs without writes, and cover empty workflows.
- Preserve service authorization, controller sync/session-target guards, and fenced commands.
- Add env-gated real PostgreSQL persistence and multi-connection behavior tests.
- Update the public workflow how-to with the bounded reorder guarantee.
- Targeted checks, conventional commit, push, focused PR, CI/review remediation, normal merge.

## Out of scope

- Schema migrations, frontend edits, other fixes, broad suites, or new workers/tasks/sessions.
- Making a complete settings Save atomic or changing content-save/version APIs.

## Acceptance

1. Permanent regressions fail on the unmodified implementation for lost content and
   partial ordering, then pass with a position/timestamp-only repository transaction.
2. Successful orders are complete, unique, scoped permutations; rejected requests
   or SQL failures leave all prior positions/timestamps intact and retain guard behavior.
3. Exact targeted checks pass; conditional PostgreSQL execution/skip is recorded,
   and the exact PR head passes required CI and trusted semantic review before normal merge.

## Verification

Run these from the repository root; PostgreSQL tests use only a disposable DSN.

```bash
(cd apps/backend && go test ./internal/workflow/service -run '^TestReorderSteps(PreservesConcurrentEdit|RollsBackSecondWriteFailure)$' -count=1)
(cd apps/backend && go test -race ./internal/workflow/service ./internal/workflow/repository ./internal/workflow/controller ./internal/workflow/handlers ./internal/mcp/handlers)
(cd apps/backend && go test -race ./internal/workflow/repository -run '^TestPostgresReorderSteps' -count=1 -v)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -race ./internal/persistence/storeconformance -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
git status --short -- docs/plans/workflow-step-reorder-atomicity
```

Run the repository PR-documentation coverage preflight after implementation
using the current `.github/scripts/pr-docs.cjs` local evaluation interface.
Before a backend CI/review fixup push, run the scoped guidance's golangci-lint
command against the resolved PR base SHA. Respect worker budgets; retain and
join every command handle.

## Files likely touched

- `apps/backend/internal/workflow/service/service.go`
- `apps/backend/internal/workflow/service/reorder_test.go`
- `apps/backend/internal/workflow/repository/reorder.go`
- `apps/backend/internal/workflow/repository/reorder_test.go`
- `apps/backend/internal/workflow/repository/reorder_postgres_test.go`
- Affected controller, HTTP, and MCP boundary tests.
- `apps/backend/internal/workflow/models/errors.go` and HTTP/MCP validation translation.
- `docs/public/workflow-tips.md`
- This package and `docs/specs/tasks/{requirements,system-design}/workflow-step-ordering.md`.

## Dependencies

Parent coordinator reviewed the four artifacts and sent the later explicit
implementation request. No code dependency.

## Risks

- SQLite writer acquisition and PostgreSQL transaction behavior differ.
- Error translation must not leak foreign workflow membership.
- Do not weaken session-target validation or change exact Host version fencing.

## Parallelism

`sequential`

## Inputs

- [Ordering requirement](../../specs/tasks/requirements/workflow-step-ordering.md)
- [Ordering design](../../specs/tasks/system-design/workflow-step-ordering.md)
- [Completion requirement](../../specs/tasks/requirements/task-completion.md), criterion 001.4.
- Existing `ReorderStepsIfUnchanged`, `service/access_test.go`, and PostgreSQL isolated-schema tests.
- Permanent regression source records the writer-acquisition barrier and
  failure trigger; the owned temporary investigation fixture was removed after
  permanent coverage was established.

## Results

Permanent RED on unmodified main: the exact two-regression command failed in
0.055s with partial position 0 instead of 1 and lost prompt `original` instead
of `edited concurrently`. Invalid-membership tests also proved duplicate,
omitted, and empty orders incorrectly succeeded.

GREEN/local evidence:

- Exact two-regression command: passed (0.081s).
- `go test -race ./internal/workflow/service ./internal/workflow/repository ./internal/workflow/controller`:
  passed, including the added controller session-target order guard. Service and
  repository results were cached from the successful full affected-package run.
- Additional `go test -race ./internal/workflow/handlers`: passed (6.641s),
  covering route success, foreign-step privacy, sync immutability, and rollback.
- `go test -race ./internal/workflow/handlers -run 'Test(ReorderStepsEndpoint|ReorderRequiresStepsToBelongToTheWorkflow|StepMutation)' -count=1`:
  passed (1.747s).
- Real env-gated PostgreSQL command: passed (2.917s), four test families with
  seven membership subcases. No ambient DSN was set. An owned disposable
  PostgreSQL 17 Docker container supplied the DSN, with isolated schema per case;
  no live instance or developer data was used.
- `go run ./cmd/sqlguard ./internal`: passed.
- `go test -race ./internal/persistence/storeconformance -count=1` with the
  disposable PostgreSQL DSN: passed (146.768s).
- Catalog validation and specification lint: passed; linter tests: 36 passed.
- Public-doc validator tests: 62 passed; public validator: 47 pages validated.
- PR-doc `validateCoverage` preflight on all actual changed paths: covered, no errors.
- `git diff --check`: passed. Work-order status remains `in_progress` until delivery.

One fixture correction isolated rows from constructor-seeded default steps;
PostgreSQL timestamp assertions compare persisted snapshots rather than
nanosecond inputs, because the database stores microsecond precision. The new
controller test confirms existing behavior, including a successful legal reorder.

PR, exact-head CI/trusted semantic reviews, and normal merge are pending.


### PR review remediation

PR [#4141](https://github.com/kdlbs/kandev/pull/4141) initially exposed valid
boundary gaps: invalid complete orders returned internal errors, explicit empty
MCP orders were rejected even for empty workflows, and SQLite did not check
workflow existence before accepting an empty order. Permanent tests reproduced
HTTP 500 instead of 400, missing-workflow success, and incorrect MCP responses
before the remediation edits. Typed invalid-order classification uses existing
HTTP 400/MCP validation replies; missing/foreign resources remain not-found.
The PostgreSQL edit barrier now observes `pg_blocking_pids`, and invalid-order
tests check both classification and reason. Targeted remediation verification,
CI, trusted semantic reviews, and merge remain pending.

The first Action Pinning run failed only in the unchanged walkthrough-runner
shared-deadline test. Its exact local test passed 20 repeats. The failed job was
rerun once; attempt 2 of run 36908151283 succeeded on the exact original head
without source edits, confirming this failure was transient. No walkthrough
source/test edits were made in this scope.


The repository API confirms `OPENCODE_REVIEW_ENABLED=false`; the trusted-base
OpenCode workflow gates on that setting and configured App identity. Disabled,
skipped review jobs do not count as semantic review evidence. The active Claude
review and all registered checks remain delivery gates. Initial Claude run
36908151025 completed successfully, but its App-authored comment only says it
will analyze the PR; no substantive review has been claimed from that result.

Post-review local checks: the expanded five-package race command passed
(repository 11.416s, HTTP handlers 10.983s, MCP handlers cached from a passing
116.592s initial run); the exact original two regressions passed (0.046s).
The updated real PostgreSQL command passed (8.373s), including the additional
missing-workflow case and live lock-table edit barrier. SQL guard, catalog,
specification lint, public documentation checks, and PR coverage preflight
passed. Full backend lint found one test-only if/else-chain style issue; it was
converted to a switch and the targeted repository cases passed (1.530s).
The repeated SQLite/disposable-PostgreSQL persistence conformance command passed
(168.428s). Final lint on the rebased source and exact-head remote delivery
checks are pending. Main advanced to `daab1c45647e7ac9e002f15e02f6e910a3e778a4`.

The first Claude job's raw log reports `is_error=true`, one turn, 445ms duration,
and zero cost despite a successful job conclusion; its boilerplate comment does
not count as a review. The next normal synchronize review must provide real
current-head findings/verdict before merge. Old-head E2E shard 5 also reports a
nested-submodule review assertion failure outside reorder scope; the exact
leaf was sent to the parent coordinator to avoid duplicate remediation.
