---
id: "01-add-revocation-workflow"
title: "Add merge approval revocation workflow"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-CI-MERGE-APPROVAL-001
acceptance_criteria:
  - AC-CI-MERGE-APPROVAL-001.1
  - AC-CI-MERGE-APPROVAL-001.2
  - AC-CI-MERGE-APPROVAL-001.3
  - AC-CI-MERGE-APPROVAL-001.4
  - AC-CI-MERGE-APPROVAL-001.5
  - AC-CI-MERGE-APPROVAL-001.6
  - AC-CI-MERGE-APPROVAL-001.7
system_design:
  - ../../specs/ci/system-design/contributor-merge-approval-revocation.md
---

# Task 01: Add merge approval revocation workflow

## Summary

Create the trusted base-controlled workflow that revokes `ready-to-merge` and
withdraws active merge automation after an untrusted contributor push. Add
static contract coverage and register it with the existing workflow lint job.

## In scope

- Add `.github/workflows/revoke-ready-to-merge.yml`.
- Add the workflow contract test.
- Register the contract test with action-pinning CI.
- Preserve the pusher-based permission exemption and event-label race guard.

## Out of scope

- Changes to Kandev application code or merge-queue integration.
- Changes to existing contributor review and preview workflows.
- Automatic re-approval or validation of the new pull-request revision.

## Acceptance

- A non-write, event-labeled synchronize path removes the exact label and
  independently attempts auto-merge disable and queue dequeue; write/admin
  pushers and unlabelled event snapshots are no-ops.
- The workflow uses only trusted base content, has `pull-requests: write` as
  its mutation permission, and pins the external action.
- The contract test runs in the existing lint workflow and passes with the
  action-pinning and workflow security checks.

## Verification

```bash
python3 .github/scripts/revoke-ready-to-merge-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
python3 .github/scripts/lint-action-pinning.py
zizmor .github/workflows
git diff --check
```

## Files likely touched

- `.github/workflows/revoke-ready-to-merge.yml`
- `.github/scripts/revoke-ready-to-merge-workflow-contract_test.py`
- `.github/workflows/lint-action-pinning.yml`

## Dependencies

None.

## Risks

- The implementation must preserve cleanup if one GitHub mutation is already
  satisfied or another mutation fails.
- The permission lookup must use the event sender, not the pull-request author.

## Parallelism

`sequential`

## Inputs

- `docs/specs/ci/requirements/contributor-merge-approval-revocation.md`
- `docs/specs/ci/system-design/contributor-merge-approval-revocation.md`
- `docs/decisions/2026-08-31-revoke-merge-approval-after-untrusted-push.md`
- `.github/AGENTS.md` and existing workflow contract tests.

## Results

- Added the base-controlled `pull_request_target` synchronize workflow with
  current sender permission lookup, exact event-label gating, and write/admin
  exemption.
- Added independent label, auto-merge, and merge-queue cleanup with explicit
  idempotent outcomes and failure reporting.
- Added 10 workflow contract tests and registered them in the always-on action
  pinning workflow.
- RED failed against the missing workflow, then GREEN passed all 10 contract
  tests.
- `lint-action-pinning_test.py` passed, 9 tests, and `lint-action-pinning.py`
  passed for 21 workflows.
- Specification tests and all-file specification lint passed.
- `git diff --check` passed.
- `zizmor .github/workflows` completed with existing repository findings and
  the intentional `pull_request_target` dangerous-trigger finding for this
  required workflow; no other finding was reported by its targeted audit.
