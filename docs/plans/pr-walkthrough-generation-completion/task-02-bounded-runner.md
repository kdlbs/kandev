---
id: "02-bounded-runner"
title: "Supervise generation and narrow Git fetching"
status: done
wave: 2
depends_on: ["01-render-completion"]
plan: "plan.md"
requirements:
  - REQ-UI-PR-WALKTHROUGH-001
acceptance_criteria:
  - AC-UI-PR-WALKTHROUGH-001.6
  - AC-UI-PR-WALKTHROUGH-001.7
  - AC-UI-PR-WALKTHROUGH-001.13
  - AC-UI-PR-WALKTHROUGH-001.14
system_design:
  - ../../specs/ui/system-design/pr-walkthrough.md
---

# Task 02: Supervise generation and narrow Git fetching

## Summary

Complete generation after a verified render and bound all attempts by one deadline. Fetch only the two histories needed for context while preserving an exact trusted checkout.

## In scope

- Add a trusted Python runner adapter with standard-library process-group supervision.
- Poll the receipt every 250 milliseconds and terminate the owned agent group after observing completion.
- Apply one 600-second monotonic deadline across both attempts. Reserve a separate cleanup interval of at most 10 seconds.
- Use SIGTERM, then SIGKILL after five seconds, and reap owned processes before verification.
- Distinguish supervised completion from an unexpected non-zero process exit.
- Keep one clean retry for zero-exit incomplete output within the remaining deadline.
- Treat external cancellation as terminal failure even if output files exist.
- Capture start/end timestamps, elapsed time, raw exit status, stop reason, verification result, streams, and draft for each attempt.
- Set the generation job limit to 20 minutes. Bound checkout to two minutes, history fetching to four, context preparation to one, and installation to one.
- Bound independent verification to 30 seconds. Retain at least one minute for artifact upload after the preparation and generation budgets.
- Add `.github/scripts/pr-walkthrough-history.sh` and its local Git regression suite.
- Check out depth one at `github.workflow_sha`. Fetch complete histories only for that SHA and the PR-head ref, then validate exact head and merge base.
- Wire the adapter, verifier, and new tests into the walkthrough and CI lint workflows.

## Out of scope

- Retry after timeout, provider errors, unexpected non-zero exits, cancellation, or invalid receipts.
- Granting the agent access to the supervisor, verifier, receipts, or output files.
- Changing provider, publication gates, workflow concurrency, or PR-description behavior.

## Acceptance

- `test_render_then_idle_completes` invokes the real renderer from a fake agent that then waits forever. The runner stops its group and accepts the verified outputs.
- Tests reject stale or invalid receipts, partial or mismatched files, unexpected exits, and external cancellation. They prove deadline sharing, retry limits, and cleanup of owned descendants.
- A shallow local clone with divergent trusted and PR branches gains a merge base through targeted fetching. The checkout stays trusted and unrelated branches and tags are absent. Existing workflow security assertions continue to pass.

## Verification

```bash
python3 .github/scripts/pr-walkthrough-runner_test.py
bash .github/scripts/pr-walkthrough-history_test.sh
python3 .github/scripts/pr-walkthrough-workflow-contract_test.py
python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-render.test.py
python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-verify.test.py
python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-context.test.py
python3 scripts/pr-walkthrough-pr-body.test.py
python3 .github/scripts/lint-action-pinning_test.py
python3 .github/scripts/lint-action-pinning.py
actionlint .github/workflows/pr-walkthrough.yml .github/workflows/lint-action-pinning.yml
zizmor .github/workflows
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Record existing unrelated zizmor findings separately. The changed jobs must introduce no new finding.

## Files likely touched

- `.github/scripts/pr-walkthrough-runner.py` (new)
- `.github/scripts/pr-walkthrough-runner_test.py` (new)
- `.github/scripts/pr-walkthrough-history.sh` (new)
- `.github/scripts/pr-walkthrough-history_test.sh` (new)
- `.github/scripts/pr-walkthrough-workflow-contract_test.py`
- `.github/workflows/pr-walkthrough.yml`
- `.github/workflows/lint-action-pinning.yml`
- `docs/specs/ui/requirements/pr-walkthrough.md`
- `docs/specs/ui/system-design/pr-walkthrough.md`
- This plan and both work-order result sections.

## Dependencies

Task 01 supplies the receipt and verifier. Keep the workflow and helper revision aligned at the trusted workflow SHA.

## Risks

- Cancellation races must never become publication success.
- Test clocks and short fixture deadlines must replace long sleeps.
- Remove old static assertions for the inline retry loop only after behavioral runner tests cover the same contract.
- Targeted unshallow fetching still needs sufficient history for old or divergent PR heads.

## Parallelism

`sequential`

## Inputs

- Requirement AC-UI-PR-WALKTHROUGH-001.6, .7, .13, and .14.
- System-design sections: Runner supervision, Git history preparation, and Generation diagnostics.
- `.github/AGENTS.md` and the existing workflow contract suite.
- [Previous retry package](../pr-walkthrough-runner-reliability-fix/task-02-retry-incomplete-opencode-output.md).

## Results

Added the trusted runner with receipt polling, process-group cleanup, a shared
600-second deadline, one clean incomplete-output retry, cancellation handling,
independent verification, per-attempt diagnostics, and a workflow summary.
Added targeted history fetching that preserves the trusted checkout and rejects
an unexpected PR head or missing merge base. The workflow now uses depth-one
checkout and bounded preparation and generation steps with time for artifact
upload.

Follow-up review fixes classify an already-observed natural process exit before
accepting a receipt, while preserving valid natural zero-exit and supervised
completion. Failed owned-group cleanup on an incomplete zero-exit attempt now
terminates the run before verification or retry.

Verification passed:

- `python3 .github/scripts/pr-walkthrough-runner_test.py` (17 tests, including regressions for observed natural exits, receipt-before-renderer-exit completion, zombie-only process groups, terminal cleanup failure, and cleanup-summary diagnostics)
- `bash .github/scripts/pr-walkthrough-history_test.sh`
- `python3 .github/scripts/pr-walkthrough-workflow-contract_test.py` (30 tests)
- `python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-render.test.py` (10 tests)
- `python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-verify.test.py` (8 tests, including renderer import and syntax diagnostics)
- `python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-context.test.py` (4 tests)
- `python3 scripts/pr-walkthrough-pr-body.test.py` (9 tests)
- `python3 .github/scripts/lint-action-pinning_test.py` (9 tests)
- `python3 .github/scripts/lint-action-pinning.py` (25 workflow files)
- `go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/pr-walkthrough.yml .github/workflows/lint-action-pinning.yml`
- Python compile checks, Bash syntax checks, harness lint, specification validation and lint, and `git diff --check`

The full `zizmor .github/workflows` audit remains nonzero because of existing
repository findings. The focused audit reports the unchanged
`pull_request_target` trigger finding, which also appears on the workflow at
`HEAD`; no new finding was introduced.


### Shared-deadline CI test remediation, 2026-10-06

Hosted action-pinning workflow run `37449612632`, attempt 1 on PR head
`414143e509e759fb03fe08b8ad66a729009114ef`, failed in the walkthrough runner
suite: its real-clock duration was 0.7406 seconds against a 0.7-second
assertion. The workflow's action references passed local pinning validation;
the leaf failure was the runner test, not the pinning contract.

The shared-deadline test now uses a controlled monotonic clock, fake process
outcomes, and the existing runner boundary. The first incomplete attempt
consumes 0.28 seconds; the second receives only the remaining 0.22 seconds.
It asserts both cleanup calls and persisted attempt outcomes. Actual process
containment remains covered by the existing process-group tests.
No production runner, workflow permission, deadline, or public behavior changed.

Validation passed:

- `python3 .github/scripts/pr-walkthrough-runner_test.py` (17 tests).
- `python3 .github/scripts/pr-walkthrough-workflow-contract_test.py` (30 tests).
- `python3 .github/scripts/lint-action-pinning_test.py` (9 tests).
- `python3 .github/scripts/lint-action-pinning.py` (26 workflow files).
- An in-memory mutation that resets the deadline at each attempt fails the
  new regression at 0.78 seconds instead of 0.50 seconds. Production files
  remained unchanged during this check.

Hosted CI for the subsequently delivered head remains pending.
