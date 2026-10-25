---
spec: docs/specs/integrations/requirements/claude-fork-review-allowlist.md
created: 2026-08-22
status: implemented
---

# Implementation Plan: Persist Fork Approval Labels

## Overview

Change the fork approval workflows so `safe-to-test` and `safe-to-review` are
durable maintainer approvals. Remove the synchronize-time cleanup jobs, allow
the existing label paths on follow-up pushes, and add contract coverage that
prevents either workflow from silently returning to per-commit re-approval.
Update the related security decision and allowlist spec in the same change.

## Confirmed root cause

PR #2815 is a fork pull request. The GitHub audit trail shows both labels added
by a maintainer and later removed by `github-actions[bot]` after contributor
pushes. The corresponding successful workflow runs executed
`strip-safe-to-test` and `strip-safe-to-review`. The jobs and the
`event.action != 'synchronize'` guards are explicit in the current workflow
files.

## Workflow changes

- `.github/workflows/preview-env.yml`
  - Keep `safe-to-test` in the `deploy-fork` authorization expression.
  - Remove the `github.event.action != 'synchronize'` exclusion from that label
    path so an existing approval authorizes the current fork head after pushes.
  - Remove the `strip-safe-to-test` job and stale per-commit comments.
- `.github/workflows/opencode-code-review.yml`
  - Keep `safe-to-review` in the fork review authorization expression.
  - Remove the synchronize exclusion from that label path.
  - Remove the `strip-safe-to-review` job and stale per-commit comments.
- `.github/workflows/lint-action-pinning.yml`
  - Run the OpenCode workflow contract test in the existing always-on workflow
    contract suite.

## Contract tests

- `.github/scripts/preview-env-workflow-contract_test.py`
  - Assert the preview workflow keeps the label authorization path.
  - Assert the synchronize exclusion and `strip-safe-to-test` job are absent.
- `.github/scripts/opencode-code-review-workflow-contract_test.py`
  - Add focused coverage for the fork label gate, synchronize eligibility, and
    absence of `strip-safe-to-review`/`removeLabel` cleanup.
- Keep the Claude workflow contract test passing to prove the separate
  open/labeled-only Claude review policy remains unchanged.

## Documentation and decision records

- Amend `docs/specs/integrations/requirements/claude-fork-review-allowlist.md` with durable-label
  scenarios and the changed OpenCode/preview behavior.
- Amend `docs/decisions/2026-08-07-claude-allowlist-label-bridge.md` to point
  the superseded per-commit clause at the new decision.
- Add `docs/decisions/2026-08-22-persistent-fork-approval-labels.md` and index
  it in `docs/decisions/INDEX.md`.

## Tests

- **What:** The preview label path authorizes a fork `synchronize` event and no
  cleanup job removes `safe-to-test`.
  **File:** `.github/scripts/preview-env-workflow-contract_test.py`.
  **How:** Read the workflow text and assert the label expression remains, the
  synchronize exclusion is absent, and the cleanup job is absent.
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

- This intentionally makes a maintainer label approval apply to future fork
  heads. Maintainers must remove labels when trust should be revoked.
- `CLAUDE_REVIEW_ALLOWLIST` and the other direct allowlist paths remain
  independent and fail-closed.
- The Claude review workflow does not gain a `synchronize` trigger in this
  change. This task does not alter same-repository review behavior or add a new
  token/App-based label event bridge.

## Verification Results

- RED: The new preview and OpenCode contract assertions failed against the
  pre-change workflows because the synchronize exclusions and cleanup jobs
  were still present.
- GREEN: `preview-env-workflow-contract_test.py` (2 tests),
  `opencode-code-review-workflow-contract_test.py` (3 tests), and the existing
  `claude-code-review-workflow-contract_test.py` (9 tests) pass.
- Security checks: `lint-action-pinning_test.py` (9 tests) and
  `lint-action-pinning.py` pass; `git diff --check` passes.
- `zizmor .github/workflows` ran but exits non-zero on existing repository-wide
  findings, including pre-existing dangerous-trigger, permissions, artifact,
  cache, and template-injection findings. This change did not expand that
  audit scope.
