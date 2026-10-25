---
id: "01-guard-ineligible-triggers"
title: "Guard ineligible walkthrough triggers"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-CI-PR-WALK-003
  - REQ-CI-PR-TRUST-001
  - REQ-CI-PR-FAIL-005
acceptance_criteria:
  - AC-CI-PR-WALK-003.1
  - AC-CI-PR-WALK-003.5
  - AC-CI-PR-TRUST-001.2
  - AC-CI-PR-FAIL-005.1
system_design:
  - ../../specs/ci/system-design/unified-contributor-pr-automation.md
---

# Task 01: Guard ineligible walkthrough triggers

## Summary

Adjust the PR walkthrough workflow concurrency expression so an event rejected
by the full generation gate cannot cancel an eligible run for the same pull
request. Add contract coverage for label and non-labeled rejected events and
preserve the existing job-level authorization behavior.

## In scope

- Update the top-level concurrency group in
  `.github/workflows/pr-walkthrough.yml`.
- Add assertions to
  `.github/scripts/pr-walkthrough-workflow-contract_test.py`.
- Verify the existing `safe-to-review`, allowlist, draft, toggle, and
  same-repository rerun gates remain unchanged.

## Out of scope

- Walkthrough content, renderer behavior, R2 publication, or PR-body linking.
- A new manual-dispatch workflow input.
- Changes to preview or code-review workflows.

## Acceptance

- Contributor `safe-to-review` events and same-repository
  `generate-pr-walkthrough` events remain in the canceling per-PR group.
- An ineligible label event such as `safe-to-test` and an unauthorized fork
  non-labeled event use a separate group and cannot cancel an eligible
  walkthrough pipeline.
- The contract test passes and still rejects `safe-to-test` as an authorization
  source.

## Verification

```bash
python3 .github/scripts/pr-walkthrough-workflow-contract_test.py
```

## Files likely touched

- `.github/workflows/pr-walkthrough.yml`
- `.github/scripts/pr-walkthrough-workflow-contract_test.py`

## Dependencies

None.

## Risks

- A mismatch between workflow-level group selection and the job-level gate
  could either waste a run or allow an unintended cancellation path.

## Parallelism

`sequential`

## Inputs

- `REQ-CI-PR-WALK-003.1` and `REQ-CI-PR-WALK-003.5`.
- `REQ-CI-PR-TRUST-001.2` and `REQ-CI-PR-FAIL-005.1`.
- [Unified contributor pull request automation system design](../../specs/ci/system-design/unified-contributor-pr-automation.md).
- [Eligibility concurrency ADR](../../decisions/2026-09-22-pr-walkthrough-eligibility-concurrency.md).

## Results

- Updated the workflow-level concurrency expression to mirror the complete
  generation gate. Every rejected event uses a per-pull-request
  `-ineligible` group while eligible triggers retain the canceling shared
  group.
- Added contract coverage for source-specific labels, unauthorized fork
  non-labeled events, the global gate conditions, the ineligible suffix, and
  latest-run cancellation.
- `python3 .github/scripts/pr-walkthrough-workflow-contract_test.py` passed
  with 29 tests.
- `python3 .github/scripts/lint-action-pinning_test.py` and
  `git diff --check` passed.
- `go run github.com/rhysd/actionlint/cmd/actionlint@latest` with
  `.github/workflows/pr-walkthrough.yml` passed.
