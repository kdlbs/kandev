---
id: "04-resilient-api-evaluation"
title: "Make documentation coverage API evaluation resilient"
status: done
wave: 4
depends_on:
  - "01-coverage-validator"
  - "02-pr-workflow"
  - "03-queue-coverage"
plan: "plan.md"
requirements:
  - REQ-CI-PR-DOCS-003
acceptance_criteria:
  - AC-CI-PR-DOCS-003.1
  - AC-CI-PR-DOCS-003.2
  - AC-CI-PR-DOCS-003.3
system_design:
  - ../../specs/ci/system-design/pull-request-documentation-coverage.md
---

# Task 04: Make documentation coverage API evaluation resilient

## Summary

Prevent transient GitHub API failures from turning otherwise valid pull
requests into opaque documentation-coverage errors.

## Root cause

The validator makes several GitHub API reads for changed files and referenced
documents. A transient network, rate-limit, or server response currently fails
the whole evaluation immediately. The workflow writes the detailed error only
to the step summary, so the runner log exposes only `exit 1`.

## In scope

- `.github/scripts/pr-docs.cjs`
- `.github/scripts/pr-docs.test.cjs`
- `docs/specs/ci/system-design/pull-request-documentation-coverage.md`
- `docs/plans/pr-documentation-coverage/plan.md`

## Acceptance

- Retry transient API and transport failures with bounded backoff, while
  permanent client errors still fail closed.
- Print final policy or infrastructure reasons in the job log and retain the
  detailed step summary.
- Preserve current revision checks, security bounds, policy failures, and merge
  group behavior.

## Verification

```bash
node --test .github/scripts/pr-docs.test.cjs
python3 .github/scripts/pr-docs-workflow-contract_test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Results

Implemented in `.github/scripts/pr-docs.cjs` and covered by
`.github/scripts/pr-docs.test.cjs`.

- GitHub API requests retry transient network errors, HTTP 408/429/5xx
  responses, rate-limit responses, unreadable transient response bodies, and
  invalid JSON from retryable server responses.
- Backoff is bounded, honors numeric `Retry-After` and primary
  `X-RateLimit-Reset` hints within the workflow's operational limit, and stops
  immediately when an explicit server wait exceeds that limit. Permanent
  client errors remain fail-closed.
- Failed evaluations print bounded, newline-safe policy or infrastructure
  reasons to the runner log before writing and retaining the detailed step
  summary.
- Existing exact-head reads, ambiguity detection, security limits, and
  merge-group behavior remain unchanged.

Verification:

- `node --test .github/scripts/pr-docs.test.cjs` (63 passed)
- `python3 .github/scripts/pr-docs-workflow-contract_test.py` (5 passed)
- `python3 .github/scripts/lint-action-pinning_test.py` (9 passed)
- `python3 scripts/lint-spec-files.py --all` (passed)
- `git diff --check` (passed)
- Read-only PR #3677 evaluation returned `covered` at its current head.
