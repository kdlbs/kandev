# ADR-2026-08-22-persistent-fork-approval-labels: Persist Fork Approval Labels Across Pushes

**Status:** accepted (amended by 2026-08-24-unified-fork-approval-label)
**Date:** 2026-08-22
**Area:** infra, workflow, security

> This decision records the persistence rule. The current approval-label
> contract, including the removal of `safe-to-test`, is defined by
> [ADR-2026-08-24-unified-fork-approval-label](2026-08-24-unified-fork-approval-label.md).

## Context

The `safe-to-test` and `safe-to-review` labels are explicit maintainer approval
markers for fork pull requests. The preview and OpenCode workflows currently
remove those labels on every `pull_request_target` `synchronize` event, which
forces maintainers to repeat the same approval after routine follow-up commits.
This does not match the repository's normal review workflow, where contributors
often push several fixes before a pull request is complete.

## Decision

Treat `safe-to-test` and `safe-to-review` as durable maintainer approvals that
remain on a fork pull request until a maintainer removes them. The preview and
OpenCode fork jobs evaluate those labels on `synchronize` events, so an approved
fork pull request remains eligible for the current head after follow-up pushes.

Remove the per-commit label cleanup jobs and the corresponding synchronize
exclusions from `.github/workflows/preview-env.yml` and
`.github/workflows/opencode-code-review.yml`.

The direct `PREVIEW_ENV_ALLOWLIST`, `CLAUDE_REVIEW_ALLOWLIST`, and
`OPENCODE_REVIEW_ALLOWLIST` paths remain independent authorization sources.
Labels added by `GITHUB_TOKEN` still do not recursively trigger another
workflow run. The Claude review workflow keeps its existing open/labeled-only
follow-up policy; label persistence alone does not make Claude review every
push.

## Consequences

- Maintainers apply each approval label once and can push multiple follow-up
  commits without repeating the approval.
- An approved fork push can run the preview code with its existing deployment
  credentials and can start the existing OpenCode review path for the new head.
- Maintainers must remove the relevant label when approval is revoked; there is
  no automatic per-commit safety reset.
- The workflow contract tests must protect both durable label presence and
  synchronize-event eligibility.

## Alternatives Considered

- Keep per-commit cleanup and require re-approval after every push. Rejected
  because it creates repeated maintenance work for normal review iterations.
- Persist labels but keep synchronize exclusions. Rejected because the labels
  would remain visible while not authorizing the workflows on the events that
  matter for follow-up commits.
- Replace labels with a new token or GitHub App approval mechanism. Rejected
  because it adds another credential and does not improve the requested
  maintainer workflow.
