---
spec: docs/specs/claude-fork-review-allowlist/spec.md
created: 2026-08-22
status: implemented
---

# Implementation Plan: Persist Fork Approval Labels

## Overview

Keep `safe-to-test` and `safe-to-review` as durable maintainer markers while
preserving the privileged preview reapproval boundary. Remove the
synchronize-time cleanup jobs, keep the preview label path blocked on new fork
heads, allow the constrained OpenCode label path on follow-up pushes, and add
contract coverage for both policies. Update the related security decision and
allowlist spec in the same change.

## Confirmed root cause

PR #2815 is a fork pull request. The GitHub audit trail shows both labels added
by a maintainer and later removed by `github-actions[bot]` after contributor
pushes. The corresponding successful workflow runs executed
`strip-safe-to-test` and `strip-safe-to-review`. The jobs and the
`event.action != 'synchronize'` guards are explicit in the current workflow
files.

## Workflow changes

- `.github/workflows/preview-env.yml`
  - Keep `safe-to-test` in the `deploy-fork` authorization expression and keep
    its `github.event.action != 'synchronize'` exclusion for privileged preview
    execution of new fork heads.
  - Remove the `strip-safe-to-test` job so the label remains visible after
    pushes, and update comments to explain the fresh approval boundary.
- `.github/workflows/opencode-code-review.yml`
  - Keep `safe-to-review` in the fork review authorization expression.
  - Remove the synchronize exclusion from that label path.
  - Remove the `strip-safe-to-review` job and stale per-commit comments.
- `.github/workflows/lint-action-pinning.yml`
  - Run the OpenCode workflow contract test in the existing always-on workflow
    contract suite.

## Contract tests

- `.github/scripts/preview-env-workflow-contract_test.py`
  - Assert the preview workflow keeps the label authorization path and its
    synchronize exclusion.
  - Assert the `strip-safe-to-test` job and label-removal call are absent.
- `.github/scripts/opencode-code-review-workflow-contract_test.py`
  - Add focused coverage for the fork label gate, synchronize eligibility, and
    absence of `strip-safe-to-review`/`removeLabel` cleanup.
- Keep the Claude workflow contract test passing to prove the separate
  open/labeled-only Claude review policy remains unchanged.

## Documentation and decision records

- Amend `docs/specs/claude-fork-review-allowlist/spec.md` with durable-label
  scenarios and the changed OpenCode/preview behavior.
- Amend `docs/decisions/2026-08-07-claude-allowlist-label-bridge.md` to point
  the superseded per-commit clause at the new decision.
- Add `docs/decisions/2026-08-22-persistent-fork-approval-labels.md` and index
  it in `docs/decisions/INDEX.md`.

## Tests

- **What:** The preview label remains visible without authorizing a new fork
  head on `synchronize`, and no cleanup job removes `safe-to-test`.
  **File:** `.github/scripts/preview-env-workflow-contract_test.py`.
  **How:** Read the workflow text and assert the label expression remains, the
  synchronize exclusion is present, and the cleanup job is absent.
- **What:** The OpenCode label path authorizes a fork `synchronize` event and
  no cleanup job removes `safe-to-review`.
  **File:** `.github/scripts/opencode-code-review-workflow-contract_test.py`.
  **How:** Read the workflow text and assert the trigger, label gate, and
  absence of cleanup and synchronize exclusion.
- **What:** Existing fork allowlist labeling and Claude review behavior remain
  intact.
  **File:** `.github/scripts/claude-code-review-workflow-contract_test.py`.
  **How:** Run the existing contract suite without changing its open/labeled
  assertions.

## Verification commands

Run from the repository root:

```bash
python3 .github/scripts/preview-env-workflow-contract_test.py
python3 .github/scripts/opencode-code-review-workflow-contract_test.py
python3 .github/scripts/claude-code-review-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
python3 .github/scripts/lint-action-pinning.py
zizmor .github/workflows
git diff --check
```

## Implementation Waves

Wave 1 (sequential; one coupled security-boundary change):

- [x] [Task 01: Persist fork approval labels](task-01-persist-fork-approval-labels.md)

## Risks and out of scope

- `safe-to-review` intentionally applies to future constrained OpenCode fork
  heads. `safe-to-test` remains visible but requires fresh approval before
  privileged preview execution of a new fork head.
- Maintainers must remove labels when trust should be revoked.
- `CLAUDE_REVIEW_ALLOWLIST` and the other direct allowlist paths remain
  independent and fail-closed.
- The Claude review workflow does not gain a `synchronize` trigger in this
  change. This task does not alter same-repository review behavior or add a new
  token/App-based label event bridge.

## Verification Results

- RED: The updated preview contract assertion failed against the durable-label
  implementation because its synchronize exclusion was absent.
- GREEN: `preview-env-workflow-contract_test.py` (2 tests),
  `opencode-code-review-workflow-contract_test.py` (3 tests), and the existing
  `claude-code-review-workflow-contract_test.py` (9 tests) pass.
- Security checks: `lint-action-pinning_test.py` (9 tests) and
  `lint-action-pinning.py` pass; `git diff --check` passes.
- `zizmor .github/workflows` ran but exits non-zero on existing repository-wide
  findings, including pre-existing dangerous-trigger, permissions, artifact,
  cache, and template-injection findings. This change did not expand that
  audit scope.
