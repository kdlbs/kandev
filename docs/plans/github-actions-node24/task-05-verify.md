---
id: "05-verify"
title: "Verify no Node-20 action pins remain"
status: done
wave: 2
depends_on: ["01-test-workflows", "02-container-workflows", "03-release-workflow", "04-review-misc-workflows"]
plan: "plan.md"
---

# Task 05: Verify no Node-20 action pins remain

- **Acceptance:** a full audit of every `uses:` pin in `.github/workflows/` shows zero `runs.using: node20` (all node24 or composite), the pinning lint and all workflow contract tests pass, and every changed workflow is valid YAML.
- **Verification:** run, and record outputs of:
  - full audit per `audit.md` across all 18 workflow files
  - `python3 .github/scripts/lint-action-pinning.py` and `python3 .github/scripts/lint-action-pinning_test.py`
  - `python3 .github/scripts/release-workflow-contract_test.py`
  - `python3 .github/scripts/claude-code-review-workflow-contract_test.py`
  - `python3 .github/scripts/preview-env-workflow-contract_test.py`
  - `python3 -c "import yaml,sys; [yaml.safe_load(open(f)) for f in sys.argv[1:]]" .github/workflows/*.yml`
  - `git diff --check`
- **Files likely touched:** none (verification only).
- **Dependencies:** tasks 01–04.
- **Parallelism:** sequential (verification last).

## Checklist

1. Re-run step 1–2 of `audit.md`: every pinned SHA's `action.yml` reports `runs.using: node24` (or `composite` with node24 inner actions). Confirm the final output contains no `node20` line.
2. Confirm the SHA-pinning lint and its tests still pass (all refs remain 40-char SHAs).
3. Confirm the three workflow contract tests pass — the SHA-asserting ones only pin `crazy-max/ghaction-import-gpg@2dc316d…` and `actions/setup-node@48b55a0…`, both unchanged by this plan.
4. Confirm YAML parses for all workflows.
5. Record the first post-merge workflow run (e.g. `backend-tests`) where the Node-20 deprecation warning no longer appears.

## Inputs

plan.md verification section; `audit.md`.

## Output contract

Full audit output, all test command outputs, final status of `plan.md` and tasks 01–04.

## Results

- Full audit (audit_final.py) over all 18 workflows: 25 distinct pinned actions, every `runs.using` = `node24` or `composite` (rust-cache/checkout v6/setup-node v6/setup-go v6/pnpm v5/import-gpg v7.0.0/create-github-app-token v3.2.0 and the three composite actions dereferenced and re-checked at their commits). Zero `node20`.
- `lint-action-pinning.py` → all SHA-pinned; `lint-action-pinning_test.py` → 9 OK.
- Contract tests: release 24 OK, claude-code-review 9 OK, preview-env 1 OK.
- YAML parse: 18/18 OK; `git diff --check` clean.
- Post-merge warning check: pending — first `backend-tests`/`e2e-tests` run after merge should show no Node-20 deprecation warning.
