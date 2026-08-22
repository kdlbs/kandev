# ADR-2026-08-22-persistent-fork-approval-labels: Persist Fork Approval Labels Across Pushes

**Status:** accepted
**Date:** 2026-08-22
**Area:** infra, workflow, security

## Context

The `safe-to-test` and `safe-to-review` labels are explicit maintainer approval
markers for fork pull requests. The preview and OpenCode workflows currently
remove those labels on every `pull_request_target` `synchronize` event, which
loses useful approval state and forces maintainers to repeat label operations
after routine follow-up commits. Preview execution is a privileged operation,
so a persistent `safe-to-test` marker must not authorize an unreviewed new fork
head.

## Decision

Treat `safe-to-test` and `safe-to-review` as durable maintainer markers that
remain on a fork pull request until a maintainer removes them. The `safe-to-test`
label remains visible after a push but its label path stays blocked on
`synchronize`, requiring fresh maintainer approval before privileged preview
execution. The `safe-to-review` label remains an approval for the constrained
OpenCode fork-review job on subsequent `synchronize` events.

Remove the per-commit label cleanup jobs from
`.github/workflows/preview-env.yml` and
`.github/workflows/opencode-code-review.yml`. Keep the preview
`safe-to-test` synchronize exclusion while removing the OpenCode exclusion.

The direct `PREVIEW_ENV_ALLOWLIST`, `CLAUDE_REVIEW_ALLOWLIST`, and
`OPENCODE_REVIEW_ALLOWLIST` paths remain independent authorization sources.
Labels added by `GITHUB_TOKEN` still do not recursively trigger another
workflow run. The Claude review workflow keeps its existing open/labeled-only
follow-up policy; label persistence alone does not make Claude review every
push.

## Consequences

- Maintainers apply each label once and can push multiple follow-up commits
  without losing the visible approval marker.
- A follow-up fork push can start the constrained OpenCode review path when
  `safe-to-review` is present. Privileged preview execution still requires
  fresh `safe-to-test` approval for the new head.
- Maintainers must remove the relevant label when approval is revoked; there is
  no automatic per-commit safety reset.
- The workflow contract tests must protect durable label presence, the
  preview reapproval boundary, and OpenCode synchronize-event eligibility.

## Alternatives Considered

- Keep per-commit cleanup and require re-approval after every push. Rejected
  because it creates repeated maintenance work for normal review iterations.
- Persist labels while keeping the preview synchronize exclusion. Accepted
  because the label remains useful as visible state without turning a prior
  privileged approval into approval for arbitrary new fork code.
- Replace labels with a new token or GitHub App approval mechanism. Rejected
  because it adds another credential and does not improve the requested
  maintainer workflow.
