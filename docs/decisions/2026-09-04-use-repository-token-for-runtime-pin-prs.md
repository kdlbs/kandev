# ADR-2026-09-04-use-repository-token-for-runtime-pin-PRs: Use the built-in Actions token for runtime pin PRs

**Status:** accepted  
**Date:** 2026-09-04  
**Area:** workflow, security

## Context

The managed runtime pin workflow was merged with inputs for a dedicated GitHub
App client ID and private key. Those repository credentials were never
configured, so both scheduled runs reached the token step and stopped before
the updater, branch push, or pull-request creation.

The workflow only needs to write the trusted runtime catalogue to one
repository branch and create or edit one pull request. The repository already
provides a short-lived `GITHUB_TOKEN` for each workflow job. GitHub documents
that events created by this token can require approval when they create or
update a pull request, while a GitHub App installation token or personal access
token can trigger those events without that approval boundary.

## Decision

The managed runtime pin workflow uses the repository's built-in
`GITHUB_TOKEN`. The workflow grants only `contents: write` and
`pull-requests: write`, configures Git authentication after the trusted
checkout, and uses the token for the branch push and pull-request operations.
It does not require a GitHub App, an App private-key secret, or a personal
access token.

The workflow continues to validate the catalogue and managed-runtime packages
before it pushes, keeps one stable updater branch and one grouped pull request,
and never auto-merges or activates a runtime. Pull-request checks created by
this token can require a maintainer to approve the generated workflow runs.
That approval requirement is accepted until unattended PR checks become a
separate operational requirement.

See [GitHub's `GITHUB_TOKEN` documentation](https://docs.github.com/en/actions/concepts/security/github_token)
for the event-trigger behavior that informs this decision.

## Consequences

- Scheduled and manual runs work without repository-specific App setup.
- The workflow has no additional long-lived credential to rotate or expose.
- The built-in token remains scoped to the repository and the two required
  write permissions.
- Maintainers may need to approve checks on a generated pull request before
  those checks run.
- A future requirement for fully unattended PR checks must add a separately
  managed GitHub App or personal access token and update this decision.

## Alternatives Considered

- **Dedicated GitHub App:** Rejected for this repair because the required App
  credentials are not configured and the user does not have an App.
- **Personal access token:** Rejected because it adds a long-lived user-owned
  credential and still requires repository secret setup.
- **Built-in token with a broad permission set:** Rejected because the updater
  needs only repository contents and pull-request writes.
