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
pull request, and no auto-merge. The generated pull request can require
maintainer approval for its checks because it is created by the built-in token.

## Scope

### In scope

- Change workflow permissions and authentication to the repository token.
- Update the workflow contract test to reject the unavailable App boundary and
  require the built-in token boundary.
- Reconcile the runtime-update requirement, system design, ADR index, and the
  completed workflow work-order record.

### Out of scope

- Creating or installing a GitHub App.
- Creating a personal access token or changing repository secrets/settings.
- Changing the npm updater, runtime catalogue, validation suite, or PR content.
- Designing a separate automatic PR-check approval workflow.

## Technical approach

- In `.github/workflows/update-agent-runtime-pins.yml`, grant only
  `contents: write` and `pull-requests: write`, remove
  `actions/create-github-app-token`, and use `${{ github.token }}` for Git and
  `gh` operations.
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
  repository token and reject the unavailable App credential boundary.
- The existing updater fixture tests remain green to prove the updater itself
  is unchanged.

## E2E tests

No browser or product UI behavior changes. GitHub Actions run history is not
mutated as part of local verification.

## Work orders

- [x] [Task 01: Use the built-in token for managed runtime pin PRs](task-01-built-in-token.md)

## Verification results

The updater tests passed 7/7, the workflow contract tests passed 8/8, the
action-pinning tests passed 9/9, the action-pinning linter accepted all 21
workflow files, and `zizmor .github/workflows/update-agent-runtime-pins.yml`
reported no findings. Specification lint passed for all specification files,
and `git diff --check` passed.

## Risks

- Pull-request checks created by `GITHUB_TOKEN` can require maintainer approval.
- Repository policy must continue to allow Actions to create pull requests.
- The workflow's write token must remain limited to this repository and the two
  required permissions.
