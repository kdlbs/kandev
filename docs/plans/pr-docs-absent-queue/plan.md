---
created: 2026-09-27
status: done
requirements:
  - REQ-CI-PR-DOCS-002
  - REQ-CI-PR-DOCS-003
system_design:
  - ../../specs/ci/system-design/pull-request-documentation-coverage.md
legacy_specs: []
---

# Implementation plan: Absent merge queue label reevaluation

## Overview

Fix documentation coverage evaluation when the `no-docs-allow` label changes
on a pull request whose target branch has no merge queue. Preserve strict
membership checks for actual merge-group events. Keep the PR status pending
until the label event's required reads finish, and make any job failure visible
in the runner log.

The CI system owns this fix because it owns the label event, coverage status,
and merge-group evaluation contract. This is a follow-up to the completed
[PR documentation coverage](../pr-documentation-coverage/plan.md) and
[GitHub API resilience](../github-api-resilience/plan.md) packages; their
completed work orders and recorded results remain historical evidence.

## Root cause and reproduction

On PR #3989, adding `no-docs-allow` triggered run
[`36278438749`](https://github.com/kdlbs/kandev/actions/runs/36278438749).
The run published override success on head
`8bacdcafefca287fa7aca7e19e3fa8ee879a407a`, then published error and
exited 1. The label-event branch always calls `listMergeQueueEntries` after
publishing the PR result. GitHub currently returns `mergeQueue: null` for
`main`, and `listMergeQueueEntries` classifies that valid no-queue response as
an incomplete connection. A local read-only response fixture reproduces the
`GitHub merge queue response is incomplete` error. The job log exposes only
exit code 1 because the evaluator writes the result reason to the step summary
without printing it to the runner log.

## Scope

### In scope

- Treat an explicit absent queue as no affected groups on a label event.
- Keep malformed responses, GraphQL errors, and missing actual merge-group
  membership as infrastructure errors.
- Publish the PR-head terminal status after label-related group work finishes;
  keep the PR's own policy result separate from affected group policy results.
- Print a bounded failure category and reason in the runner log.
- Add regression tests at the GitHub adapter and evaluator boundaries.

### Out of scope

- Changing coverage exemptions, label permissions, merge-queue membership,
  repository rulesets, or unrelated workflows.
- Replacing the existing request retry policy or creating a new status context.
- Application UI or browser tests.

## Technical approach

In `.github/scripts/pr-docs.cjs`, distinguish `repository.mergeQueue === null`
from a missing repository, an absent field, a malformed `entries` connection,
or a GraphQL error. Return an empty entry list only for the explicit null queue.
The label path then finds no affected groups. The `merge_group` path still
rejects an empty list because it cannot validate the event's member chain.

Keep `pending` as the only PR-head status during a label transition. Evaluate
the PR, read the queue, and update affected group statuses before publishing
the PR-head terminal result. An affected group's policy failure makes the job
fail and sets that group's status, while the PR-head status reflects the PR's
own coverage decision. A queue read failure sets the PR-head status to error.
Log the final failed result as one bounded, sanitized line, without raw API
responses or contributor document content. The step summary retains detailed
paths and remediation.

No workflow trigger or permission change is expected. The trusted-base checkout
and `persist-credentials: false` remain the security boundary.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| AC-CI-PR-DOCS-002.1, .2, .5 | `pr-docs.test.cjs`: label addition with `mergeQueue: null` finishes with override success; removal evaluates the PR normally with no groups. |
| AC-CI-PR-DOCS-003.2, .4 | Adapter fixtures distinguish explicit null from missing repository, malformed connection, and GraphQL errors; an actual merge-group event with no members remains an error. |
| AC-CI-PR-DOCS-003.8, .9 | Event fixtures assert status ordering, separate PR/group policy outcomes, error on queue-read failure, and a bounded safe runner-log reason. |

The first red test, `label addition with an absent merge queue preserves
override success`, should reproduce the observed labeled-event failure with a
fake GitHub response containing `{ repository: { mergeQueue: null } }`.
Run the existing Node suite after the change. Run the workflow contract test
to confirm the event gate remains intact.

## Work orders

- [x] [Task 01: Finish label reevaluation without a queue](task-01-finish-label-reevaluation.md)

## Verification results

Implementation completed on 2026-09-27:

- The 104 validator tests passed, including absent, malformed, and disappearing
  queue responses; label status ordering; group result separation; and bounded
  runner-log reporting.
- The workflow contract suite passed all 7 tests.
- The specification catalog validated 314 decisions and 1194 specifications.
- All specification files passed the full linter.
- `git diff --check` passed.

## Risks

- Treating every `null` as an empty queue could hide authorization or malformed
  responses. Accept only an explicit null queue under a valid repository with
  no GraphQL errors.
- Reordering status publication could make a group policy failure incorrectly
  fail the PR's own override. Keep PR and group results separate in tests.
- GitHub status writes remain eventually consistent. The existing bounded
  retry policy can create duplicate records after an uncertain response.

## References

- [Requirements](../../specs/ci/requirements/pull-request-documentation-coverage.md)
- [System design](../../specs/ci/system-design/pull-request-documentation-coverage.md)
- [Coverage policy decision](../../decisions/2026-09-10-pr-documentation-coverage.md)
