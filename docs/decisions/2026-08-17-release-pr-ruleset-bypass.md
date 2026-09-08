# ADR-2026-08-17-release-pr-ruleset-bypass: Give Stable Release PRs an Administrator Token Bypass

**Status:** accepted
**Date:** 2026-08-17
**Area:** infra, workflow, security

## Context

The Stable release workflow creates a generated version pull request and merges it immediately. The `main` ruleset added required checks and a merge queue on 2026-08-16.

GitHub CLI now rejects the `--delete-branch` merge option in the workflow when the queue is active. Queue enrollment also waits for checks that GitHub holds for approval on `GITHUB_TOKEN`-created pull requests.

Maintainers do not require CI for generated release commits. The workflow still needs a pull request for traceability and must bind the signed tag to the release merge.

## Decision

Normal Stable releases keep the release pull request. A fine-grained personal access token from an organization administrator merges that pull request with the GitHub CLI `--admin` option.

The token selects only `kdlbs/kandev`. It has `contents: write` repository permission. The existing organization-administrator bypass in the `main` ruleset authorizes the merge.

The protected `release` environment owns the token as the `RELEASE_PR_BYPASS_TOKEN` secret. Maintainers must rotate the token before it expires and replace it when its owner loses administrator access.

The normal `GITHUB_TOKEN` pushes the release branch, creates the pull request, and reads the merged state. The personal access token is available only to the step that merges the expected head.

The workflow removes `--delete-branch` and relies on the repository branch-cleanup setting. It verifies the reported merge commit on `origin/main` before it signs the release tag.

The repair contract is recorded in `docs/specs/release/requirements/release-pr-queue-bypass.md`.

## Consequences

Generated release pull requests merge without CI or queue latency. Ordinary pull requests remain subject to every required check and queue rule.

The person-owned token becomes a protected release dependency. A missing, expired, revoked, or insufficient token stops the release before tag creation.

The token owner can bypass repository rules as an organization administrator. Restricting the token to one repository and one workflow step limits its exposure, but not the owner's bypass capability.

Maintainers must configure the `release` environment before the next normal Stable release. Ruleset `13341245` already grants organization administrators bypass access.

## Alternatives Considered

- Wait for pull-request and merge-group CI. Rejected because maintainers do not require CI for generated release commits.
- Use a dedicated Release GitHub App. Rejected for the initial repair because its setup and key management add operational work. It remains the preferred replacement for a person-owned token.
- Add the built-in GitHub Actions app to the bypass list. Rejected because this gives the same bypass identity to other workflows.
- Push the release commit directly to `main`. Rejected because this removes the pull-request trail and requires a wider bypass.
- Remove required checks or the merge queue. Rejected because ordinary pull requests need those protections.
