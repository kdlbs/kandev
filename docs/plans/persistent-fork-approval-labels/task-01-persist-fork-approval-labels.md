---
id: "01-persist-fork-approval-labels"
title: "Persist fork approval labels"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/requirements/claude-fork-review-allowlist.md"
---

# Task 01: Persist fork approval labels

## Acceptance

- `safe-to-test` remains on a fork pull request after a contributor push and
  keeps the preview fork job eligible for the current head on `synchronize`.
- `safe-to-review` remains on a fork pull request after a contributor push and
  keeps the OpenCode fork-review job eligible for the current head on
  `synchronize`.
- Neither workflow contains a synchronize-time label cleanup job, and the
  existing Claude open/labeled-only review policy remains unchanged.

## Verification

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

## Files likely touched

- `.github/workflows/preview-env.yml`
- `.github/workflows/opencode-code-review.yml`
- `.github/workflows/lint-action-pinning.yml`
- `.github/scripts/preview-env-workflow-contract_test.py`
- `.github/scripts/opencode-code-review-workflow-contract_test.py`
- `docs/specs/integrations/requirements/claude-fork-review-allowlist.md`
- `docs/decisions/2026-08-07-claude-allowlist-label-bridge.md`
- `docs/decisions/2026-08-22-persistent-fork-approval-labels.md`
- `docs/decisions/INDEX.md`

## Dependencies

None. The root cause and desired behavior are confirmed.

## Parallelism

`sequential`; workflow gates, contract tests, and security records form one
coupled change.

## Inputs

- `docs/specs/integrations/requirements/claude-fork-review-allowlist.md`
- `docs/decisions/2026-08-07-claude-allowlist-label-bridge.md`
- `docs/decisions/2026-08-22-persistent-fork-approval-labels.md`
- `.github/AGENTS.md`
- `.github/workflows/preview-env.yml`
- `.github/workflows/opencode-code-review.yml`

## Risks

Persisting a label makes a maintainer approval apply to future fork heads.
Do not broaden the workflow conditions beyond the existing external-PR label
gates or remove the direct allowlist paths.

## Output contract

Report the exact workflow and contract-test changes, RED and GREEN results for
the focused tests, security lint results, `git diff --check`, any live fork
verification still needed after merge, and synchronized task/plan statuses.

## Results

- Removed the preview and OpenCode synchronize-time label cleanup jobs.
- Removed both label-path `synchronize` exclusions so existing maintainer
  approvals authorize follow-up fork heads.
- Added preview and OpenCode contract tests, and registered the OpenCode test
  in the always-on action-pinning workflow.
- RED passed as an expected failure against the old workflows. GREEN passed
  with 2 preview tests, 3 OpenCode tests, and 9 Claude tests.
- `lint-action-pinning_test.py` (9 tests), `lint-action-pinning.py`, and
  `git diff --check` pass.
- `zizmor .github/workflows` ran and reported existing repository-wide
  findings; it is not a clean baseline gate for this change.
