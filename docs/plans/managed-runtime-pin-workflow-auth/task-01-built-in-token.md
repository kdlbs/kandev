---
id: "01-built-in-token"
title: "Use the built-in token for managed runtime pin PRs"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - ../../specs/agents/requirements/runtime-updates.md
acceptance_criteria:
  - AC-AGENTS-RUNTIME-UPDATES-001.9
  - AC-AGENTS-RUNTIME-UPDATES-001.10
system_design:
  - ../../specs/agents/system-design/runtime-updates-01.md
---

# Task 01: Use the built-in token for managed runtime pin PRs

## Summary

Make the weekly/manual managed runtime pin workflow runnable without a
GitHub-App credential. Use the repository-scoped built-in Actions token with
the minimum permissions for branch, PR, and explicit validation-workflow
dispatches, while preserving validation, branch, and grouped PR behavior.

## In scope

- Replace App-token creation and its unavailable inputs in the workflow.
- Configure Git and `gh` with `${{ github.token }}`.
- Dispatch the six required validation workflows explicitly against the bot
  branch, and add their `workflow_dispatch` triggers.
- Update the workflow contract test and the completed workflow record.
- Keep the runtime updater and catalogue unchanged.

## Out of scope

- GitHub App or personal access token provisioning.
- Automatic approval of generated PR checks.
- Changes to runtime selection or npm metadata logic.

## Acceptance

- The workflow grants only `contents: write`, `pull-requests: write`, and
  `actions: write`, and uses `${{ github.token }}` for branch, PR, and
  validation-dispatch mutations.
- The workflow explicitly dispatches all six required validation workflows
  after the generated PR exists, and each target accepts `workflow_dispatch`.
- The contract test fails against the current App-token workflow and passes
  after the workflow uses the built-in token.
- The updater still validates before any commit or push, creates at most one
  grouped PR, and never auto-merges.

## Verification

```bash
node --test scripts/update-agent-runtime-pins.test.mjs && python3 .github/scripts/update-agent-runtime-pins-workflow-contract_test.py && python3 .github/scripts/lint-action-pinning_test.py && python3 .github/scripts/lint-action-pinning.py && zizmor .github/workflows/update-agent-runtime-pins.yml && python3 scripts/lint-spec-files.py --all && git diff --check
```

## Files likely touched

- `.github/workflows/update-agent-runtime-pins.yml`
- `.github/scripts/update-agent-runtime-pins-workflow-contract_test.py`
- `docs/specs/agents/requirements/runtime-updates.md`
- `docs/specs/agents/system-design/runtime-updates-01.md`
- `docs/plans/managed-runtime-version-awareness/task-04-weekly-pin-workflow.md`

## Dependencies

None.

## Risks

- The repository or organization setting **Allow GitHub Actions to create and
  approve pull requests** must be enabled for the PR mutation.

## Parallelism

`sequential`

## Inputs

- [Managed runtime requirements](../../specs/agents/requirements/runtime-updates.md).
- [Managed runtime system design](../../specs/agents/system-design/runtime-updates-01.md).
- [Token decision](../../decisions/2026-09-04-use-repository-token-for-runtime-pin-prs.md).
- [Scoped GitHub Actions guidance](../../../.github/AGENTS.md).

## Results

Implemented the repository-scoped `GITHUB_TOKEN` boundary and removed the
unconfigured GitHub-App token step. The workflow retains trusted-main checkout,
validation before mutation, the stable updater branch, grouped PR behavior, and
no auto-merge. It explicitly dispatches the six required validation workflows
after creating or refreshing the PR, and the target workflows accept the manual
event. The contract test covers the token, permissions, target list, and target
triggers.

Verification passed:

- `node --test scripts/update-agent-runtime-pins.test.mjs`: 7/7.
- `python3 .github/scripts/update-agent-runtime-pins-workflow-contract_test.py`: 9/9.
- `python3 .github/scripts/lint-action-pinning_test.py`: 9/9.
- `python3 .github/scripts/lint-action-pinning.py`: 21 workflows accepted.
- `zizmor .github/workflows/update-agent-runtime-pins.yml`: no findings.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check`: passed.
