---
created: 2026-08-31
status: done
requirements:
  - REQ-CI-MERGE-APPROVAL-001
system_design:
  - ../../specs/ci/system-design/contributor-merge-approval-revocation.md
legacy_specs: []
---

# Implementation Plan: Contributor merge approval revocation

## Overview

Add one trusted GitHub Actions workflow that removes stale `ready-to-merge`
approval after a non-write pusher updates a pull request. The workflow will
also disable active auto-merge and dequeue the pull request, while exempting
pushes from current write-capable users. Contract tests and the existing action
pinning and workflow security checks will verify the implementation.

## Scope

### In scope

- A `pull_request_target` synchronize workflow from the trusted base branch.
- Current permission lookup for the synchronize event sender.
- Exact `ready-to-merge` label removal for untrusted pushes.
- Active auto-merge disable and merge-queue dequeue operations.
- Idempotent cleanup and per-pull-request concurrency.
- Static workflow contract coverage and CI registration.

### Out of scope

- Changes to `safe-to-review` or other contributor review and preview labels.
- Changes to Kandev application code or merge-queue integration.
- Code review, CI re-evaluation, or automatic re-approval.
- UI, application code, or non-GitHub providers.

## Technical approach

Implement `.github/workflows/revoke-ready-to-merge.yml` with only the
`pull_request_target` `synchronize` activity. Run the pinned
`actions/github-script` action without checkout. Resolve
`github.event.sender.login` through the collaborator permission endpoint and
return for `write` or `admin`. For non-write, unknown, or failed permission
results, require the event label snapshot to contain `ready-to-merge`, remove
that exact label, read fresh pull-request GraphQL state, and independently
disable auto-merge and dequeue an active queue entry.

Add `.github/scripts/revoke-ready-to-merge-workflow-contract_test.py` to assert
the trigger, pusher permission gate, exact label, GraphQL mutation names,
idempotent cleanup intent, no checkout or unsafe execution, least-privilege
permission block, pinned action, and per-PR non-canceling concurrency. Register
the test in `.github/workflows/lint-action-pinning.yml`.

## Tests

- `AC-CI-MERGE-APPROVAL-001.1` and `.2`: contract assertions for the exact
  label removal and both merge-state mutations.
- `AC-CI-MERGE-APPROVAL-001.3` and `.4`: contract assertions for event-label
  gating and write/admin pusher exemption.
- `AC-CI-MERGE-APPROVAL-001.5`: contract assertion for sender permission
  lookup and fail-closed handling.
- `AC-CI-MERGE-APPROVAL-001.6`: contract assertions for idempotent, independent
  cleanup and non-canceling per-PR concurrency.
- `AC-CI-MERGE-APPROVAL-001.7`: contract assertions for trusted workflow
  provenance, no checkout, pinned action, and `pull-requests: write` only.

## Operational validation

After deployment, verify a disposable external-contributor PR in these states:

1. With `ready-to-merge` and no active merge operation, a contributor push
   removes only the label.
2. With `ready-to-merge` and auto-merge enabled, a contributor push removes
   the label and disables auto-merge.
3. With `ready-to-merge` and an active queue entry, a contributor push removes
   the label and dequeues the PR.
4. A maintainer push to the same external PR leaves all three states unchanged.
5. A push without the label does not alter queue or auto-merge state.

## Work orders

- [x] [Task 01: Add merge approval revocation workflow](task-01-add-revocation-workflow.md)

## Verification results

- RED: the new workflow contract test failed because the revocation workflow
  did not exist yet.
- GREEN: `python3
  .github/scripts/revoke-ready-to-merge-workflow-contract_test.py` passed, 10
  tests.
- `python3 .github/scripts/lint-action-pinning_test.py` passed, 9 tests, and
  `python3 .github/scripts/lint-action-pinning.py` passed for 21 workflows.
- `python3 scripts/lint-spec-files.test.py` passed, 20 tests, and
  `python3 scripts/lint-spec-files.py --all` passed.
- `git diff --check` passed.
- `zizmor .github/workflows` completed with the repository's existing findings
  plus the required workflow's generic `dangerous-triggers` finding for
  `pull_request_target`; the targeted audit reported no other finding.

## Risks

- A permission API outage is fail-closed and can revoke approval for a
  write-capable pusher; the workflow must make the lookup failure visible.
- GitHub may change GraphQL error wording for already-clean queue or
  auto-merge state; tests and cleanup handling must classify only known
  idempotent outcomes as successful no-ops.
- A delayed workflow must not remove a label added after an unlabelled push;
  the event label snapshot is part of the cleanup gate.
