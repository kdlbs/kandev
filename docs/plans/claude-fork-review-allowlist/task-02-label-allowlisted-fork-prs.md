---
id: "02-label-allowlisted-fork-prs"
title: "Label allowlisted fork pull requests"
status: done
wave: 1
depends_on: ["01-forward-fork-allowlist"]
plan: "plan.md"
spec: "../../specs/claude-fork-review-allowlist/spec.md"
---

# Task 02: Label allowlisted fork pull requests

## Acceptance

- When an allowlisted user opens a fork pull request, a base-controlled job adds `safe-to-review` and `safe-to-test` with one idempotent label API call; same-repository and non-allowlisted pull requests do not enter that job.
- The Claude fork review path remains directly gated by `CLAUDE_REVIEW_ALLOWLIST`, and the preview fork deployment path also accepts that allowlist for its existing non-closed events, so the automation does not depend on a `GITHUB_TOKEN`-emitted `labeled` event to start another workflow.
- Contract tests cover the new job, labels, permissions, preview gate, and action pinning CI coverage.

## Verification

- `python3 .github/scripts/claude-code-review-workflow-contract_test.py`
- `python3 .github/scripts/preview-env-workflow-contract_test.py`
- `python3 .github/scripts/lint-action-pinning.py`
- `git diff --check`

## Files likely touched

- `.github/workflows/claude-code-review.yml`
- `.github/workflows/preview-env.yml`
- `.github/workflows/lint-action-pinning.yml`
- `.github/scripts/claude-code-review-workflow-contract_test.py`
- `.github/scripts/preview-env-workflow-contract_test.py`
- `docs/specs/claude-fork-review-allowlist/spec.md`
- `docs/plans/claude-fork-review-allowlist/plan.md`
- `docs/decisions/2026-08-07-claude-allowlist-label-bridge.md`
- `docs/decisions/INDEX.md`

## Dependencies

Task 01 is complete; use the existing `allowed_non_write_users` forwarding contract and preserve its workflow behavior.

## Parallelism

`sequential`; the workflow gates, contract tests, CI registration, and security documentation are one coupled trust-boundary change.

## Inputs

- `docs/specs/claude-fork-review-allowlist/spec.md`, especially the permissions, failure modes, and new label/preview scenarios.
- `docs/plans/claude-fork-review-allowlist/plan.md`.
- `docs/decisions/2026-08-07-claude-allowlist-label-bridge.md`.
- Existing `safe-to-review` and `safe-to-test` gates in `.github/workflows/claude-code-review.yml`, `.github/workflows/opencode-code-review.yml`, and `.github/workflows/preview-env.yml`.

## Risks

- `CLAUDE_REVIEW_ALLOWLIST` now authorizes the preview workflow to execute fork preview code with `SPRITES_API_TOKEN`; do not broaden the condition beyond the exact external-PR allowlist gate.
- Labels added with `GITHUB_TOKEN` do not recursively trigger a new workflow run, so removing the direct preview allowlist branch would silently break the requested automation.
- The repository must already contain both labels; a missing label should fail visibly rather than silently inventing a trust marker.

## Output contract

Report the exact workflow conditions and label API call, files changed, RED and GREEN contract-test results, action-pinning result, `git diff --check` result, any live-fork verification still needed after merge, and synchronized task/plan statuses.

## Results

- RED: `python3 .github/scripts/claude-code-review-workflow-contract_test.py` failed with 2 expected missing-contract assertions (label job and CI registration); `python3 .github/scripts/preview-env-workflow-contract_test.py` failed with the expected missing `CLAUDE_REVIEW_ALLOWLIST` preview gate.
- GREEN: `python3 .github/scripts/claude-code-review-workflow-contract_test.py` — 9 tests passed.
- GREEN: `python3 .github/scripts/preview-env-workflow-contract_test.py` — 1 test passed.
- GREEN: `python3 .github/scripts/lint-action-pinning.py` — all 18 workflow files use SHA-pinned action refs.
- GREEN: `git diff --check` — no whitespace errors.
- Files changed: `.github/workflows/claude-code-review.yml`, `.github/workflows/preview-env.yml`, `.github/workflows/lint-action-pinning.yml`, both workflow contract tests, and the linked spec/plan/ADR records.
- No live fork workflow or GitHub label API verification was performed locally; that remains a post-merge validation of repository configuration and the trusted allowlist.
