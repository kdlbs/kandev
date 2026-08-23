---
status: in_progress
created: 2026-08-23
updated: 2026-08-23
spec: ../../specs/e2e-duration-aware-sharding/spec.md
---

# Keep merge-queue E2E checks within the response budget

## Root cause

PR #2950 was removed from the merge queue because the required `E2E Tests
Passed` check failed. The normal shard 14 job reached its 25-minute job
timeout twice, at `14:11:29Z` and `13:46:49Z`, after the same 167-test serial
plan had run for about 24 minutes. The report job and aggregate gate then
failed because the shard was cancelled. The queue ruleset allows 60 minutes
for required checks, so the 25-minute shard cap was the smaller limit.

The run used the deterministic count fallback because no successful `main`
timing-profile artifact existed yet. The fix gives that fallback's normal
shard enough bounded execution time to finish without changing per-test
timeouts, retry behavior, or the fail-closed aggregate gate.

## Outcome

Set the normal E2E matrix job timeout to 35 minutes and protect that queue
budget with the existing workflow contract test. Keep the container, desktop,
report, and Playwright timeouts unchanged.

## Wave 1: queue-safe normal shard budget

Task 01 updates the workflow contract, the E2E workflow, and the affected
spec/plan records. It runs the focused contract test and checks that the
worktree diff is valid.

## Verification

```text
python3 .github/scripts/e2e-tests-workflow-contract_test.py
git diff --check
```

The next merge-group run is the operational verification: shard 14 must reach
a terminal test result, the report must merge, and `E2E Tests Passed` must be
successful for the PR to remain in the queue.

## Status

- [x] Confirm queue removal event and failed synthetic merge-group checks.
- [x] Confirm the repeated shard timeout and ruleset response budget.
- [x] Add the regression contract and raise only the normal shard job budget.
- [ ] Push the fix to PR #2950 and re-check the current-head queue state.

## Local implementation result

- Added a workflow contract assertion for the normal 35-minute shard budget.
- The new assertion failed against the old 25-minute workflow, then passed
  after the workflow change.
- `python3 .github/scripts/e2e-tests-workflow-contract_test.py` passed, 5 tests.
- `git diff --check` passed.
