---
created: 2026-09-15
status: done
requirements:
  - REQ-CI-PR-DOCS-002
  - REQ-CI-PR-DOCS-003
  - REQ-CI-PR-DOCS-004
  - REQ-CI-PR-SIZE-001
system_design:
  - ../../specs/ci/system-design/pull-request-documentation-coverage.md
  - ../../specs/ci/system-design/pull-request-size-labels.md
legacy_specs: []
---

# Implementation plan: GitHub API resilience

## Overview

Reduce avoidable GitHub API traffic and recover correctly when GitHub applies a
transient or secondary rate limit. Two documentation-coverage runs failed with
HTTP 429 while evaluating otherwise valid pull requests. Read-only replay showed
about 14 requests for the smaller example and 33 for the larger example,
including status publication. Per-requirement code searches and repeated pull
request reads account for much of the avoidable traffic.

The CI system owns this package because it owns the trusted workflow event,
request, mutation, and failure contracts.

## Scope

In scope: rate-aware retries and diagnostics in the custom documentation
evaluator. The package also covers request reuse, event gates, and lazy
size-label definition creation.

Out of scope: replacing `GITHUB_TOKEN`, changing documentation policy, or
changing size rules. The package does not change merge-queue membership or
repository rulesets. It does not include low-frequency workflow cleanup.

## Technical approach

Extend the documentation evaluator's existing bounded client instead of adding
a second request library. Classify errors before each retry. Follow GitHub wait
headers. Use the documented one-minute minimum for a secondary limit without a
usable header. Stop within a fixed total wait budget. Emit sanitized diagnostics
to the runner log.

Pass the wrapper's initial pull request snapshot into the evaluator. Build a
per-snapshot requirement cache from changed exact-head documents and their base
versions before using code search. Keep code search as the bounded fallback for
new, moved, unresolved, and ambiguous identities.

Narrow documentation coverage to events that can change its inputs. Keep
`labeled` and `unlabeled` triggers for GitHub routing, use a job-level gate for
the exact `no-docs-allow` label, and admit `edited` only for base-branch
retargets. Exclude title and description edits and ready-for-review changes.

For size labels, list current pull request labels once and optimistically apply
the target. Create only the target definition after a narrowly recognized
missing-label response. Existing target labels require no repository
label-definition requests.

Open PR #3693 is implementation input for Task 01. Reuse its bounded client and
diagnostic tests after rebasing. Replace its 250/500 ms no-header fallback with
the documented 60/120 second waits and total budget.

## Tests

| Criteria | Evidence |
| --- | --- |
| AC-CI-PR-DOCS-003.6 through .7 | Client fixtures for transport, 408, 429, rate-limit 403, 5xx, headers, no-header 60/120 second waits, exhaustion, permanent errors, wait-budget overflow, and sanitized logs. |
| AC-CI-PR-DOCS-004.1 through .3 | Exact-head/base fixtures count calls, reuse the initial snapshot, skip search for verified changed requirements, and retain new-ID, missing-ID, and duplicate-ID failures. |
| AC-CI-PR-DOCS-004.4 | Workflow contract verifies the minimal event list and exact override-label job gate. |
| AC-CI-PR-SIZE-001.5, .6, .9, .10 | Workflow contract verifies one current-label read, direct target apply, lazy target creation, concurrent creation recovery, and no definition preflight on the steady path. |

Use fixtures from the two failed run shapes. One fixture has four requirement
IDs in one changed document. The other has eleven IDs in several changed
documents. Assert request classes and cache keys. Do not use live pull request
numbers or mutable GitHub state.

## End-to-end evidence

No application UI changes. Local evidence comes from mocked GitHub responses and
workflow contract tests. After deployment, manually dispatch coverage for a
covered pull request and an uncovered pull request. Exercise the override label
once. Record retry diagnostics for any real throttle. Compare request counts
with the pre-change 14- and 33-request replays. Do not intentionally cause live
rate limiting.

## Work orders

- [x] [Task 01: Add rate-aware API recovery](task-01-rate-aware-api-recovery.md)
- [x] [Task 02: Consolidate documentation API reads](task-02-consolidate-documentation-api-reads.md)
- [x] [Task 03: Remove size-label definition preflights](task-03-remove-size-label-definition-preflights.md)

Execute sequentially. No subagents are authorized.

## Verification

Run the exact commands in each work order. After all tasks:

```bash
node --test .github/scripts/pr-docs.test.cjs
python3 .github/scripts/pr-docs-workflow-contract_test.py
python3 .github/scripts/pr-size-label-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
python3 .github/scripts/lint-action-pinning.py
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
zizmor .github/workflows/pr-docs.yml
zizmor .github/workflows/pr-size-label.yml
git diff --check
```

## Risks

- Retrying a status write after an uncertain response can create a duplicate
  status record with the same context and state.
- A conservative secondary-limit delay consumes runner time. The 180-second
  sleep budget keeps the workflow inside its ten-minute timeout.
- Exact-head requirement optimization relies on the trusted base catalog's
  unique-ID validation and must scan every changed requirement document in the
  referenced directory.
- Missing-label errors must be identified narrowly so validation and permission
  failures do not create labels.
- Label-event gating must preserve merge-group and manual-dispatch paths.

## References

- [Documentation coverage requirements](../../specs/ci/requirements/pull-request-documentation-coverage.md)
- [Documentation coverage design](../../specs/ci/system-design/pull-request-documentation-coverage.md)
- [Pull request size requirements](../../specs/ci/requirements/pull-request-size-labels.md)
- [Pull request size design](../../specs/ci/system-design/pull-request-size-labels.md)
- [Documentation coverage decision](../../decisions/2026-09-10-pr-documentation-coverage.md)
- [GitHub REST API rate limits](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api)
- [GitHub REST API best practices](https://docs.github.com/rest/guides/best-practices-for-integrators)
- [Open retry implementation PR #3693](https://github.com/kdlbs/kandev/pull/3693)
