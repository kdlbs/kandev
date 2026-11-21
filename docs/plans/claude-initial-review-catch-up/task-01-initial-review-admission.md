---
id: "01-initial-review-admission"
title: "Admit the missing initial Claude review"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-CI-CLAUDE-INITIAL-REVIEW-001
acceptance_criteria:
  - AC-CI-CLAUDE-INITIAL-REVIEW-001.1
  - AC-CI-CLAUDE-INITIAL-REVIEW-001.2
  - AC-CI-CLAUDE-INITIAL-REVIEW-001.3
  - AC-CI-CLAUDE-INITIAL-REVIEW-001.4
  - AC-CI-CLAUDE-INITIAL-REVIEW-001.5
  - AC-CI-CLAUDE-INITIAL-REVIEW-001.6
system_design:
  - ../../specs/ci/system-design/claude-initial-review.md
---

# Task 01: Admit the missing initial Claude review

## Summary

Add a read-only gate that starts the first automatic same-repository review
after a missed opening. Preserve the existing reviewer identity and suppress
later automatic rounds across changed commits.

## In scope

- Add the real admission module and behavior tests with injected GitHub responses.
- Add synchronization admission and non-cancelling same-PR serialization.
- Read all retained attempts and pages before declaring that no invocation exists.
- Preserve existing model-step names, fork gates, and manual review behavior.
- Register the Node tests in the existing action-pinning workflow.

## Out of scope

- The independent manual publication defect and model tool-policy changes.
- New approval labels, marker comments, provider tokens, or live GitHub mutations.

## Acceptance

1. The opening/conflict/resolution/later-push sequence invokes a stub reviewer
   exactly once through the actual admission module. Ordinary mergeable openings
   retain their existing route. A stale event never invokes the reviewer.
2. Mixed and paginated histories suppress duplicates across commits and earlier
   run attempts. Skipped actions do not count. Missing or ambiguous evidence fails
   before Claude, and later pushes do not cancel an active initial attempt.
3. The fork and manual contract suites remain valid, with unchanged action pin,
   authentication, model tools, and publication identity. CI runs the new suite.

## Verification

Run these commands from the repository root. Begin with the new Python contract
regression and record its expected failure against the current event filter.
Use Node's built-in runner, without installing frontend dependencies.

```bash
python3 .github/scripts/claude-code-review-workflow-contract_test.py
node --test .github/scripts/claude-initial-review.test.cjs
python3 .github/scripts/lint-action-pinning_test.py
zizmor .github/workflows
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Existing scanner findings must be reported separately. Do not weaken scanner
configuration or add broad suppressions to make the correction pass.

## Files likely touched

- `.github/scripts/claude-initial-review.cjs` (new).
- `.github/scripts/claude-initial-review.test.cjs` (new).
- `.github/scripts/claude-code-review-workflow-contract_test.py`.
- `.github/workflows/claude-code-review.yml`.
- `.github/workflows/lint-action-pinning.yml`.
- This plan, work order, and paired CI specification statuses/results.

## Dependencies

None.

## Risks

- Completed step history can lag. Ambiguity must fail closed, not admit twice.
- Administrator deletion of old runs removes the deduplication record.
- The same action run can contain several attempts. Inspect all of them.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ci/requirements/claude-initial-review.md).
- [System design](../../specs/ci/system-design/claude-initial-review.md).
- [Evidence and scope](plan.md#evidence-and-root-cause).
- `.github/AGENTS.md` and the existing workflow contract tests.

## Results

Implemented automatic catch-up for the initial same-repository review.

- Added `.github/scripts/claude-initial-review.cjs`, a read-only gate that reads
  the current PR head/state, lists associated `pull_request` runs of
  `claude-code-review.yml` from the PR creation date, reads every job attempt
  with `filter: all`, and treats a non-skipped started review step as a used
  attempt. Skipped actions, runs that stopped before the action, and other PRs
  do not consume an attempt.
- Recognizes GitHub's production skipped-step shape (`status: completed`,
  `conclusion: skipped`, nonempty `started_at`) before inspecting timestamps.
  A stale event that skipped the gated action no longer suppresses the next
  real review.
- Verifies history completeness against the declared `total_count`; a short or
  empty final page fails closed for both runs and jobs instead of admitting a
  duplicate. Workflow-run history is bounded to GitHub's 1,000-result API limit
  (10 pages); a larger declared total fails closed. Job pagination remains 20
  pages.
- Re-reads the PR immediately before a positive decision, so a head change or
  closure during history scanning skips the obsolete event. Regressions cover
  deferred head and state changes and the old-event-before-new-head ordering.
- Binds the same-repository review action to `CLAUDE_REVIEW_HEAD_SHA` and
  requires a head recheck before reviewing or posting, complementing the final
  admission read. Contract tests assert the admission call arguments and the
  head binding.
- Inspects completed attempts of a pending rerun before ignoring it, so a
  rerun cannot hide an earlier consumed invocation.
- Wired `synchronize` admission into `claude-code-review.yml` and made the
  same-repository concurrency group non-cancelling. A pending run behind the
  active group repeats admission; an in-progress foreign run, unknown status,
  malformed response, API error, or incomplete pagination fails closed before
  Claude. A closed PR and a changed event head skip with their own reason.
- Preserved the fork and manual paths, the pinned action, the App-token
  identity, and the model tool policy. Only `actions: read` was added to the
  same-repository job.
- Registered the Node suite in `.github/workflows/lint-action-pinning.yml`.
- Verification: 14 Python contract tests, 45 Node admission tests, 9 action
  pinning tests, catalog validation, all-spec lint, and `git diff --check`
  passed. The four remediation areas were red-verified against the prior
  behavior before the fix. `zizmor` reported only the existing workflow
  baseline findings.
- Residual risk: administrator deletion of retained run history removes the
  deduplication record, and the independent manual publication defect remains
  outside this work order.
