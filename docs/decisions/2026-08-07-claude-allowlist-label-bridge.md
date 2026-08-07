# ADR-2026-08-07-claude-allowlist-label-bridge: Use the Claude Allowlist as a Trusted Preview Gate

**Status:** accepted  
**Date:** 2026-08-07  
**Area:** infra, workflow, security

## Context

Fork pull requests listed in `CLAUDE_REVIEW_ALLOWLIST` already receive an
automatic Claude review, while the preview workflow has a separate
`PREVIEW_ENV_ALLOWLIST` and a maintainer-applied `safe-to-test` label path.
Maintaining two lists makes a trusted contributor's pull request reviewable but
not previewable by default. Adding labels from a workflow with the repository's
`GITHUB_TOKEN` also does not create a new workflow run for the resulting
`labeled` event, so a label-only bridge would not reliably start the preview or
review workflows.

## Decision

On the `pull_request_target` `opened` event, a base-controlled job in
`.github/workflows/claude-code-review.yml` adds `safe-to-review` and
`safe-to-test` to fork pull requests whose author is present in
`CLAUDE_REVIEW_ALLOWLIST`. The job does not check out pull-request content and
uses only the issue-label API with `issues: write`.

The existing direct allowlist gate remains the source of authorization for the
Claude review job. `.github/workflows/preview-env.yml` also treats
`CLAUDE_REVIEW_ALLOWLIST` as a trusted fork-preview gate for its existing
non-closed pull-request-target events. The labels remain visible approval
markers and continue to support the manual label paths; privileged workflow
execution does not depend on a recursive `labeled` event.

The new labeling job applies only to fork pull requests on open. Existing
per-commit cleanup and manual re-approval behavior for `safe-to-test` remains
unchanged.

## Consequences

- Trusted fork contributors use one allowlist for automatic review labels and
  preview eligibility, without a second duplicated identity list.
- `CLAUDE_REVIEW_ALLOWLIST` becomes a high-trust security boundary: every entry
  is authorized to run the fork preview code with `SPRITES_API_TOKEN` on the
  preview workflow's supported non-closed events.
- A missing label or label API failure is visible as a failed labeling job;
  direct review and preview authorization still come from the allowlist gate.
- Labels added by `GITHUB_TOKEN` do not recursively run downstream workflows,
  so direct conditions must remain synchronized with the labels' intended
  meaning.

## Alternatives Considered

- Keeping `PREVIEW_ENV_ALLOWLIST` completely separate was rejected because it
  requires maintainers to duplicate trusted contributor identities for the
  requested review-plus-preview behavior.
- Relying only on the newly added labels to trigger existing workflows was
  rejected because GitHub suppresses new workflow runs for most events emitted
  by `GITHUB_TOKEN`, including label events.
- Introducing a personal access token or a new GitHub App solely to emit a
  recursive label event was rejected because it adds another long-lived trust
  credential and a broader workflow recursion surface.
