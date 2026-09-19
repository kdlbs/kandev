---
id: "02-release-workflow"
title: "Wire Scoop release publication"
status: done
wave: 2
depends_on: ["01-scoop-publisher"]
plan: "plan.md"
spec: "../../specs/release/requirements/scoop-release-automation.md"
---

# Task 02: Wire Scoop release publication

## Acceptance

- Stable and backfill runs start Scoop publication only after the GitHub
  Release succeeds. Nightly, dry run, and Desktop validation do not start it.
- The job uses `github.workflow_sha`, `GITHUB_TOKEN`, and the separate
  `SCOOP_BUCKET_DEPLOY_KEY` secret. It fails clearly when the secret is absent.
- Release contract CI detects missing job dependencies, skipped-channel gates,
  backfill exclusion, secret wiring, or omitted helper tests.

## Verification

- `python3 .github/scripts/release-workflow-contract_test.py`
- `python3 .github/scripts/lint-action-pinning_test.py`
- `git diff --check -- .github/workflows/release.yml .github/workflows/lint-action-pinning.yml .github/scripts/release-workflow-contract_test.py`

## Files likely touched

- `.github/workflows/release.yml`
- `.github/workflows/lint-action-pinning.yml`
- `.github/scripts/release-workflow-contract_test.py`

## Dependencies

Task 01 must pass before this task starts.

## Parallelism

Sequential. The workflow calls the publisher interface from Task 01.

## Inputs

- The Stable, backfill, and excluded-mode scenarios in the repair spec.
- The `update-homebrew-tap` job and its explicit dependency gates.
- ADR 0029 for the `github.workflow_sha` control-plane rule.
- The existing release workflow contract tables for Stable and Nightly jobs.

## Output contract

Report the changed workflow files, Red and Green contract results, and the
secret boundary. Update this task and `plan.md` in the same conversation.

## Results

Red:

- `python3 .github/scripts/release-workflow-contract_test.py` initially found
  the missing Scoop job in five contract assertions.

Green:

- `python3 .github/scripts/release-workflow-contract_test.py` passed: 25 tests.
- `python3 .github/scripts/lint-action-pinning_test.py` passed: 9 tests.
- `git diff --check -- .github/workflows/release.yml
  .github/workflows/lint-action-pinning.yml
  .github/scripts/release-workflow-contract_test.py` passed.

The new job is a Stable-only sibling of npm and Homebrew. It requires successful
`prepare` and `publish-release` jobs, remains eligible for `backfill_tag`, reads
release assets with `GITHUB_TOKEN`, checks out `github.workflow_sha` for current
control logic, and uses only `SCOOP_BUCKET_DEPLOY_KEY` for the bucket push.
