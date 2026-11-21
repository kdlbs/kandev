---
created: 2026-11-09
status: completed
requirements:
  - REQ-CI-CLAUDE-INITIAL-REVIEW-001
system_design:
  - ../../specs/ci/system-design/claude-initial-review.md
legacy_specs: []
---

# Implementation plan: Claude initial review catch-up

## Overview

Admit a missing initial same-repository review after conflict resolution. Keep
the existing Claude App identity and fork/manual paths. Deliver the admission
module, workflow wiring, and regression coverage in one work order.

The working scope interprets the user's "other issue" as automatic catch-up.
The optional scope question received no answer when this package was drafted.
Manual publication enforcement is excluded from this proposed correction.

## Evidence and root cause

[PR #5208](https://github.com/kdlbs/kandev/pull/5208) opened at
2026-11-08 22:04:41 UTC. Original head `f4e992ee76ac2052044074ed27ee92ad2268fff3`
conflicts with then-main `58cf68ddd2ea411df98e012e5a2e406393e78cbf` in
`apps/backend/internal/agent/runtime/lifecycle/executor_backend.go`.
`git merge-tree --write-tree` reproduced that conflict without changing the
worktree. GitHub suppresses `pull_request` workflows for conflicting PRs.

[Run 42528381199](https://github.com/kdlbs/kandev/actions/runs/42528381199)
used `pull_request_target` and skipped every job, as expected for a
same-repository PR. Commit `85298d7c36667b18f28c11812e51616c6034fd1f` resolved the
conflict later. The opened-only review trigger could not catch up.

The separate [manual run](https://github.com/kdlbs/kandev/actions/runs/42599599896)
returned SDK `subtype=success`, `is_error=false`, and four permission denials.
The pinned action does not fail success results for tool denials. Its custom
prompt selects agent mode with no automatic tracking comment. The workflow does
not verify publication. Detailed denied calls were hidden and not retained.

## Scope

### In scope

- Add a same-repository initial-attempt gate across retained PR run history.
- Admit opening and synchronization events only while no attempt exists.
- Serialize same-PR admission without cancelling an active review.
- Register focused behavior and workflow-contract tests in CI.

### Out of scope

- Changing the fork target trigger, allowlists, or approval labels.
- Changing the manual workflow, action pin, credentials, or publication identity.
- Repairing silent manual publication or broadening model tool permissions.
- Adding automatic follow-up review rounds or persistent marker labels.
- Publishing GitHub comments, rerunning Actions, or creating a live test PR.

## Technical approach

Follow the [owning design](../../specs/ci/system-design/claude-initial-review.md).
Add the proposed `shouldStartInitialReview` entry point in
`.github/scripts/claude-initial-review.cjs`. Read history through the provided
Octokit client. Preserve job and model-step names for compatibility with old runs.

Wire the gate into `.github/workflows/claude-code-review.yml`, before the
existing App-token action. Add only `actions: read`. Update the concurrency
cancellation expression and add `synchronize` to the ordinary PR event.
Register the Node suite in `.github/workflows/lint-action-pinning.yml`.

## Tests

| Acceptance | Evidence |
| --- | --- |
| `.1`, `.2` | Python contract `test_same_repository_conflict_resolution_can_start_initial_review`; Node opening/conflict/resolution event sequence |
| `.3` | Node mixed history, old head, failed/cancelled step, skipped action, other PR, and earlier run-attempt cases |
| `.4` | Python non-cancelling group and action gate assertions; Node sequential event sequence and stale-head case |
| `.5` | Node malformed responses, active ambiguity, API failure, and pagination exhaustion cases |
| `.6` | Existing fork and manual contract tests, plus unchanged author/token assertions |

The first regression extends the Python contract to require synchronization
catch-up. It fails against the current opened-only workflow before production
changes. Then develop admission behavior with failing Node cases.

## End-to-end evidence

Use a local event-fixture sequence through the actual admission module and a
stub review invocation. Supply no opening run while the fixture conflicts.
Admit the resolution push, record its started action, then push another head.
The second push must skip, with exactly one stub invocation across the sequence.
This exercises admission without spending model tokens or posting externally.
GitHub's conflict suppression remains an external platform behavior, confirmed
by the recorded PR evidence and official documentation.

## Work orders

- [x] [Task 01: Admit the missing initial review](task-01-initial-review-admission.md)

## Verification results

Implemented on 2026-11-09.

- Red: the new Python contract regression failed against the opened-only
  workflow: `['opened', 'synchronize'] != ['opened']`, and the same-repository
  job had no admission step, gate, or `actions: read`.
- Green: `python3 .github/scripts/claude-code-review-workflow-contract_test.py`
  passed 14 tests.
- Green: `node --test .github/scripts/claude-initial-review.test.cjs` passed 45
  tests, including regressions for skipped-step classification, final-page
  completeness, final head/state revalidation, and pending-rerun evidence. The
  four remediation areas were verified red against the prior behavior before
  the fix. The opening/conflict/resolution/later-push sequence invoked the stub
  exactly once.
- Green: `python3 .github/scripts/lint-action-pinning_test.py` passed 9 tests,
  and `python3 .github/scripts/lint-action-pinning.py` verified all 30 pinned
  workflow files.
- Green: `python3 scripts/list-docs.py validate` validated 441 decisions and
  1951 specifications.
- Green: `python3 scripts/lint-spec-files.test.py` passed 36 tests and
  `python3 scripts/lint-spec-files.py --all` passed.
- Green: `git diff --check`.
- `zizmor .github/workflows` reported the existing repository baseline (238
  findings, 2 ignored and 226 suppressed). The only finding that names the
  changed workflow is its pre-existing `dangerous-triggers` warning for the
  fork `pull_request_target` path. The admission step added no finding.

## Risks

- API latency, incomplete history, or the pagination limit can block catch-up.
  Fail closed and retain explicit manual recovery.
- A failed or cancelled action invocation consumes the initial attempt.
- History deletion removes evidence. Do not treat deliberate deletion as a
  normal webhook flow.
- GitHub concurrency behavior and history visibility require a conservative
  response to any ambiguous active run.
- The independent manual publication defect remains. This correction does not
  establish that an SDK success result proves publication.

## References

- [GitHub pull-request event behavior](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#pull_request)
- [Pinned SDK success rule](https://github.com/anthropics/claude-code-action/blob/ae68417a745a5df4b2124873632bd73a2255e6f6/base-action/src/run-claude-sdk.ts#L252-L256)
- [Pinned agent-mode publication behavior](https://github.com/anthropics/claude-code-action/blob/ae68417a745a5df4b2124873632bd73a2255e6f6/src/modes/agent/index.ts)
