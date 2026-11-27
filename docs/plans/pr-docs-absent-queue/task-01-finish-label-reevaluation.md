---
id: "01-finish-label-reevaluation"
title: "Finish label reevaluation without a queue"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-CI-PR-DOCS-002
  - REQ-CI-PR-DOCS-003
acceptance_criteria:
  - AC-CI-PR-DOCS-002.1
  - AC-CI-PR-DOCS-002.2
  - AC-CI-PR-DOCS-002.5
  - AC-CI-PR-DOCS-003.2
  - AC-CI-PR-DOCS-003.4
  - AC-CI-PR-DOCS-003.8
  - AC-CI-PR-DOCS-003.9
  - AC-CI-PR-DOCS-003.10
system_design:
  - ../../specs/ci/system-design/pull-request-documentation-coverage.md
---

# Task 01: Finish label reevaluation without a queue

## Summary

Make exception-label reevaluation succeed when the target branch has no merge
queue, while retaining fail-closed behavior for invalid queue data and actual
merge-group events. Delay the PR-head terminal status until required label
processing finishes and report job failures in the runner log.

## In scope

- Add the red regression `label addition with an absent merge queue preserves
  override success`, reproducing the `mergeQueue: null` labeled-event failure
  from PR #3989.
- Distinguish an explicitly absent queue from malformed or inaccessible queue
  data in the bounded GitHub adapter.
- Keep PR and affected-group policy results separate while publishing terminal
  statuses after label-related work. Publish affected-group reevaluation
  statuses with a distinct context so a group failure cannot overwrite the PR
  result when both statuses use the same SHA.
- Show the PR outcome and affected-group outcomes separately in the run
  summary.
- Emit a bounded, sanitized final failure reason to the runner log.
- Cover label addition, label removal, no queue, queue lookup failure, failing
  affected group, and actual merge-group event paths.

## Out of scope

- Changing the workflow's permissions, event types, or trusted checkout.
- Changing documentation classification, artifact validation, API retries,
  merge-queue membership, or repository rulesets.

## Acceptance

- With a valid `mergeQueue: null` response, label addition publishes pending
  then override success; label removal publishes the PR's normal policy result.
- Malformed or inaccessible queue data cannot produce PR-head success, and an
  actual merge-group event without validated members remains an error.
- The PR-head terminal status follows required group work. A group's policy
  failure leaves the PR's own coverage decision intact, including when the
  group head and PR head are the same SHA; failed jobs show one bounded reason
  in the runner log.

## Verification

Run from the repository root:

```bash
node --test .github/scripts/pr-docs.test.cjs
python3 .github/scripts/pr-docs-workflow-contract_test.py
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `.github/scripts/pr-docs.cjs`
- `.github/scripts/pr-docs.test.cjs`
- `docs/specs/ci/requirements/pull-request-documentation-coverage.md`
- `docs/specs/ci/system-design/pull-request-documentation-coverage.md`
- `docs/plans/pr-docs-absent-queue/plan.md`
- `docs/plans/pr-docs-absent-queue/task-01-finish-label-reevaluation.md`

## Dependencies

None. Use the completed coverage and API-resilience packages as context.

## Risks

- A broad null fallback could hide GitHub errors. Accept only a complete
  response with an explicitly absent queue.
- Moving terminal publication could conflate PR and group failures. Assert the
  status sequence and target SHA for each scenario.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ci/requirements/pull-request-documentation-coverage.md)
- [System design](../../specs/ci/system-design/pull-request-documentation-coverage.md)
- [Plan](plan.md)
- [Coverage policy decision](../../decisions/2026-09-10-pr-documentation-coverage.md)
- `.github/AGENTS.md` and nearby adapter and label-event tests.

## Results

Completed on 2026-09-27. The first adapter regression failed with
`GitHub merge queue response is incomplete` for a valid `mergeQueue: null`
response. After the fix, the complete validator suite passed all 104 tests, the
workflow contract suite passed all 7 tests, the specification catalog and full
specification linter passed, and `git diff --check` passed.

PR review fixup on 2026-09-27: Added coverage for a failing group that shares
the labeled PR's head SHA. Label-triggered group reevaluations now use a
separate GitHub status context, and the run summary shows the PR result before
the affected-group and member results. The regression failed before the fix and
passed afterward. Full verification results are recorded in the task plan.
