# ADR-2026-07-31-isolate-manual-pr-review-content: Isolate Manual PR Review Content

**Status:** accepted
**Date:** 2026-07-31
**Area:** infra, workflow, security

## Context

Manual Claude review rounds are triggered by an `issue_comment` event, whose
workflow and credentials come from the trusted default branch. Claude still
needs the current pull request files, including files absent from the default
branch. Checking an untrusted pull request head out at the workspace root makes
repository-controlled instructions available to a privileged agent run and
lets checkout-provided GitHub credentials remain in that worktree.

## Decision

`.github/workflows/claude.yml` keeps the default branch at the workspace root.
For a top-level pull request comment containing `@claude review`, it checks the
pull request head out under `pr-head/` with checkout credential persistence
disabled and passes that directory to Claude with `--add-dir`.

The manual review uses an explicit prompt so the Claude action selects its
automation mode rather than its write-capable mention mode. Its tool policy is
review-only: it may read pull request context and post findings, but it may not
edit files, mutate Git history, push, or fetch arbitrary network content.
Other `@claude` issue and comment requests keep the trusted default-branch
workspace and the existing generic behavior.

## Consequences

- Manual reviews can inspect newly added pull request files without trusting
  pull request configuration at agent startup.
- The checkout-provided GitHub token is not retained in either worktree. The
  Claude action still manages its own short-lived authentication, while its
  review process cannot use mutation or arbitrary network tools.
- The workflow performs a second checkout for explicit pull request reviews.
- Manual review mode is intentionally unsuitable for implementing requested
  changes; implementation requests continue through the generic mention path.

## Alternatives Considered

- Checking the pull request head out at the workspace root was rejected because
  it crosses the trusted workflow and untrusted content boundary.
- Adding only `persist-credentials: false` was rejected because it does not
  constrain project instructions, agent tools, or network access.
- Keeping only the default-branch checkout was rejected because Claude cannot
  reliably inspect files that exist only on the pull request head.
