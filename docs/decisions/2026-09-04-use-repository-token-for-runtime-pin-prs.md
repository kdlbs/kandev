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
that events created by this token do not recursively start new workflows,
except for explicit `workflow_dispatch` and `repository_dispatch` events.

## Decision

The managed runtime pin workflow uses the repository's built-in
`GITHUB_TOKEN`. The workflow grants only `contents: write`,
`pull-requests: write`, and `actions: write`, configures Git authentication
after the trusted checkout, and uses the token for the branch push, pull
request, and explicit workflow-dispatch operations. It does not require a
GitHub App, an App private-key secret, or a personal access token.

The workflow continues to validate the catalogue and managed-runtime packages
before it pushes, keeps one stable updater branch and one grouped pull request,
and never auto-merges or activates a runtime. Each required validation workflow
declares `workflow_dispatch`, and the updater dispatches the six required
checks against the exact bot-branch commit after the PR exists. The repository
or organization setting **Allow GitHub Actions to create and approve pull
requests** must be enabled. Any additional maintainer approval required by
repository policy remains an operational control, not a missing credential.

See [GitHub's `GITHUB_TOKEN` documentation](https://docs.github.com/en/actions/concepts/security/github_token)
for the event-trigger behavior that informs this decision.

## Consequences

- Scheduled and manual runs work without repository-specific App setup.
- The workflow has no additional long-lived credential to rotate or expose.
- The built-in token remains scoped to the repository and three required write
  permissions.
- The six required validation workflows must retain their manual dispatch
  trigger and support the selected branch's baseline semantics.
- The Actions setting that allows workflows to create and approve pull
  requests must remain enabled.
- Repository policy can still require maintainer approval for generated runs.
- A future requirement for a different trust boundary must add a separately
  managed GitHub App or personal access token and update this decision.

## Alternatives Considered

- **Dedicated GitHub App:** Rejected for this repair because the required App
  credentials are not configured and the user does not have an App.
- **Personal access token:** Rejected because it adds a long-lived user-owned
  credential and still requires repository secret setup.
- **Built-in token without explicit validation dispatch:** Rejected because
  GitHub suppresses recursive `push` and `pull_request` workflow runs for
  events created by the built-in token.
