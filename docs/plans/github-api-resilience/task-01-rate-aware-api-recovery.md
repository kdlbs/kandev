---
id: "01-rate-aware-api-recovery"
title: "Add rate-aware API recovery"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-CI-PR-DOCS-003
acceptance_criteria:
  - AC-CI-PR-DOCS-003.6
  - AC-CI-PR-DOCS-003.7
system_design:
  - ../../specs/ci/system-design/pull-request-documentation-coverage.md
---

# Task 01: Add rate-aware API recovery

## Summary

Make the custom documentation evaluator recover from transient GitHub failures
and expose useful, sanitized failure diagnostics.

## In scope

- `.github/scripts/pr-docs.cjs`
- `.github/scripts/pr-docs.test.cjs`
- This work order's implementation results

## Out of scope

Request consolidation, workflow trigger changes, size-label behavior, token
changes, and live GitHub mutations outside normal workflow status publication.

## Acceptance

- Make at most three attempts for classified transport, 408, 429, retryable
  5xx, and rate-limit 403 errors. Fail permanent client errors immediately.
- Honor usable `Retry-After` and primary reset headers. Use 60 then 120 seconds
  for a secondary rate limit without usable guidance, with a 180-second total
  sleep budget.
- Log bounded request class, status/category, attempt, delay, and stop reason.
  Never log credentials, response bodies, query text, or document contents.

## Verification

Run from the repository root:

```bash
node --test .github/scripts/pr-docs.test.cjs
python3 .github/scripts/pr-docs-workflow-contract_test.py
git diff --check
```

## Files likely touched

- `.github/scripts/pr-docs.cjs`
- `.github/scripts/pr-docs.test.cjs`
- `docs/plans/github-api-resilience/task-01-rate-aware-api-recovery.md`

## Dependencies

None. Reconcile and reuse PR #3693 rather than independently duplicating its
client and diagnostic changes.

## Risks

Do not apply the short transport/server backoff to a secondary rate limit.
Retry status writes only with the exact original payload.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ci/requirements/pull-request-documentation-coverage.md)
- [System design](../../specs/ci/system-design/pull-request-documentation-coverage.md)
- [Plan](plan.md)
- [PR #3693](https://github.com/kdlbs/kandev/pull/3693)

## Results

Implemented the bounded GitHub API client recovery path in
`.github/scripts/pr-docs.cjs` and covered it with focused fixtures in
`.github/scripts/pr-docs.test.cjs`.

- Classified transport and timeout failures, HTTP 408, 429, retryable 5xx,
  and rate-limit 403 responses for at most three attempts.
- Permanent 4xx responses fail immediately when their body cannot be read or
  contains invalid JSON.
- Honored numeric and date `Retry-After` values and primary reset headers.
  Secondary rate-limit responses without usable headers wait 60 seconds and
  then 120 seconds, within a 180-second total sleep budget.
- Added bounded request-class diagnostics for retries, permanent failures,
  exhausted attempts, and wait-budget stops. Diagnostics contain no tokens,
  query text, response bodies, or document contents.

Verification:

- `node --test .github/scripts/pr-docs.test.cjs` (72 passed)
- `python3 .github/scripts/pr-docs-workflow-contract_test.py` (6 passed)
- `python3 .github/scripts/lint-action-pinning_test.py` (9 passed)
- `python3 .github/scripts/lint-action-pinning.py` (24 workflows passed)
- `zizmor .github/workflows/pr-docs.yml` (no findings)
- `git diff --check` (passed)
