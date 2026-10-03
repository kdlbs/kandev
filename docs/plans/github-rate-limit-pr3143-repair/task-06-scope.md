---
id: "06-scope"
title: "Remove unrelated PR changes"
status: done
wave: 6
depends_on: ["05-agent-contract"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-RATE-001
  - REQ-INTEGRATIONS-GITHUB-RATE-002
  - REQ-INTEGRATIONS-GITHUB-RATE-003
  - REQ-INTEGRATIONS-GITHUB-RATE-004
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-RATE-001.4
  - AC-INTEGRATIONS-GITHUB-RATE-002.2
  - AC-INTEGRATIONS-GITHUB-RATE-003.5
  - AC-INTEGRATIONS-GITHUB-RATE-004.4
system_design:
  - ../../specs/integrations/system-design/github-rate-limit-coordination.md
---

# Task 06: Remove unrelated PR changes

## Summary

Keep this PR limited to GitHub rate coordination and Workflow Sync recovery. Preserve unrelated patches as local evidence for separate review.

## In scope

- Re-evaluate the final diff after rebase. Upstream may already contain an unrelated fix.
- Restore unrelated artifact-download and SSH keepalive test deltas to the resolved base.
- Preserve their commit IDs and rationale in the results. Do not create another task or PR without authorization.
- Keep sqlguard and auth-circuit changes that are necessary for this feature.
- Inspect the LSP-related historical commit only if it still produces a final diff.

## Out of scope

Other work orders, external GitHub writes, and unrelated refactoring.

## Regression evidence

No new runtime test is required for restoring exact base content. Equality against the resolved base proves the scope correction.

## Acceptance

- Every remaining changed file maps to the rate/recovery contract or required documentation.
- Unrelated CI and SSH test changes no longer appear in the final PR diff.
- No unrelated patch is lost without its source commit and disposition in Results.

## Verification

Run from the repository root after implementation:

```bash
git diff --exit-code origin/main -- .github/actions/download-artifact-retry/action.yml .github/scripts/e2e-tests-workflow-contract_test.py apps/backend/internal/agent/runtime/lifecycle/executor_ssh_keepalive_test.go
git diff --stat origin/main...HEAD
git diff --check
git status --short
```

## Files likely touched

- .github/actions/download-artifact-retry/action.yml
- .github/scripts/e2e-tests-workflow-contract_test.py
- apps/backend/internal/agent/runtime/lifecycle/executor_ssh_keepalive_test.go
- docs/plans/github-rate-limit-pr3143-repair/plan.md

## Dependencies

05-agent-contract. Execute sequentially.

## Risks

The artifact retry fix addresses a real independent concern. Removing it from this PR does not certify the base action as safe.

## Parallelism

`sequential`

## Inputs

Current final diff and source commits ffd4f76be, a6254608b, and d31152eab. Original maintainer request for one logical change.

## Results

Completed 2026-09-27. Rewrote the local branch history with
`git rebase --rebase-merges -i d31152eab`, dropping the unrelated artifact
retry change from contributor commit `ffd4f76be` (replayed after Task 01 as
`0bf380a40`) and the SSH test-only change `a6254608b`. The unmodified source
commits remain reachable from `backup/pr-3143-before-scope-cleanup-20260927`.

The final LSP broker content matches `origin/main`; the later LSP commit
`d31152eab` produces no final file delta, so no LSP patch was removed. During
the rebase, its upstream merge conflicted in that file; resolution used the
exact `origin/main` version. The active branch now has `origin/main` as an
ancestor at `95a30d761b88bab0fd6795e97ba6a5146cf0e6f7`.

Validation passed:

- `git diff --exit-code origin/main -- .github/actions/download-artifact-retry/action.yml .github/scripts/e2e-tests-workflow-contract_test.py apps/backend/internal/agent/runtime/lifecycle/executor_ssh_keepalive_test.go`
- `git diff --stat origin/main...HEAD` (74 files; remaining committed paths map to the GitHub rate/recovery contract and its documentation)
- `git diff --check`
- `git status --short` confirmed repair changes remain unstaged and uncommitted.

No product runtime test was needed for this task because the three unrelated
files now match `origin/main` exactly. No push occurred.
