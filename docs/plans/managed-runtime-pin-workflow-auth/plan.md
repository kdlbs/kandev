---
created: 2026-09-04
status: complete
requirements:
  - ../../specs/agents/requirements/runtime-updates.md
system_design:
  - ../../specs/agents/system-design/runtime-updates-01.md
legacy_specs: []
decisions:
  - ../../decisions/2026-09-04-use-repository-token-for-runtime-pin-prs.md
---

# Implementation Plan: Restore managed runtime pin PR automation

## Overview

Replace the unavailable GitHub App credential in the managed runtime pin
workflow with the repository's built-in `GITHUB_TOKEN`. Preserve trusted-main
checkout, validation before mutation, the stable updater branch, one grouped
pull request, and no auto-merge. Explicitly dispatch the required validation
workflows because built-in-token branch and pull-request events do not
recursively start them.

## Scope

### In scope

- Change workflow permissions and authentication to the repository token.
- Add manual dispatch support to every required validation workflow and
  dispatch each one against the exact generated branch commit.
- Update the workflow contract test to reject the unavailable App boundary and
  require the built-in token and explicit validation-dispatch boundaries.
- Reconcile the runtime-update requirement, system design, ADR index, and the
  completed workflow work-order record.

### Out of scope

- Creating or installing a GitHub App.
- Creating a personal access token or changing repository secrets/settings.
- Changing the npm updater, runtime catalogue, validation suite, or PR content.
- Designing a separate automatic PR-check approval workflow.

## Technical approach

- In `.github/workflows/update-agent-runtime-pins.yml`, grant only
  `contents: write`, `pull-requests: write`, and `actions: write`, remove
  `actions/create-github-app-token`, and use `${{ github.token }}` for Git,
  `gh`, and workflow-dispatch operations.
- Add `workflow_dispatch` to the six required validation workflows. Make the
  manual architecture run use the `main` fork point and let the other gates
  fail open to a full validation run when no event base exists.
- Dispatch `backend-tests.yml`, `frontend-tests.yml`, `e2e-tests.yml`,
  `architecture-lint.yml`, `lint-action-pinning.yml`, and
  `lint-harness-files.yml` after the grouped PR is created or refreshed.
- Keep `persist-credentials: false`, the trusted `main` checkout, updater and
  Go validation order, stable branch, grouped PR selection, and no-auto-merge
  behavior unchanged.
- In `.github/scripts/update-agent-runtime-pins-workflow-contract_test.py`, add
  a regression assertion for the built-in token and permissions, and assert
  that App inputs and App-token action usage are absent.
- Update the runtime-update requirement/design and link the accepted token
  decision so the documented CI approval consequence is explicit.

## Tests

- `AC-AGENTS-RUNTIME-UPDATES-001.9`: workflow contract tests preserve schedule,
  validation-before-mutation, stable branch, grouped PR, and no-auto-merge
  behavior.
- `AC-AGENTS-RUNTIME-UPDATES-001.10`: workflow contract tests require the
  repository token, explicit required-check dispatch, and reject the
  unavailable App credential boundary.
- The existing updater fixture tests remain green to prove the updater itself
  is unchanged.

## E2E tests

No browser or product UI behavior changes. GitHub Actions run history is not
mutated as part of local verification.

## Work orders

- [x] [Task 01: Use the built-in token for managed runtime pin PRs](task-01-built-in-token.md)

## Verification results

The updater tests passed 7/7, the workflow contract tests passed 9/9, the
action-pinning tests passed 9/9, the action-pinning linter accepted all 21
workflow files, and `zizmor .github/workflows/update-agent-runtime-pins.yml`
reported no findings. Specification lint passed for all specification files,
and `git diff --check` passed.

## Risks

- GitHub's event suppression for built-in-token pushes and PRs is avoided by
  explicit manual dispatch of every required validation workflow.
- Repository policy must continue to allow Actions to create and approve pull
  requests.
- The workflow's write token must remain limited to this repository and the
  three required permissions.
