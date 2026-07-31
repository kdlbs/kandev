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

`.github/workflows/claude.yml` keeps the default branch at the workspace root
and does not check out pull request content. For a top-level pull request
comment containing `@claude review`, Claude reads the exact current pull
request through constrained `gh pr diff` and `gh pr view` commands.

The manual review uses an explicit prompt so the Claude action selects its
automation mode rather than its write-capable mention mode. Its tool policy is
review-only: it may read pull request context and post review comments, but it
may not edit repository files, mutate Git history, push, or fetch arbitrary
network content. Headless permission requests fail closed.
Other `@claude` issue and comment requests keep the trusted default-branch
workspace and the existing generic behavior.

## Consequences

- Manual reviews can inspect newly added pull request files without placing
  pull request content in the privileged runner workspace.
- The checkout-provided GitHub token is not retained in the trusted worktree.
  The Claude action still manages its own short-lived authentication, while
  its review process can write review comments but cannot use repository
  mutation or arbitrary network tools.
- Reviewing files beyond the pull request diff requires adding another
  explicitly allowlisted read-only GitHub command.
- Manual review mode is intentionally unsuitable for implementing requested
  changes; implementation requests continue through the generic mention path.

## Alternatives Considered

- Checking the pull request head out at the workspace root was rejected because
  it crosses the trusted workflow and untrusted content boundary.
- Adding only `persist-credentials: false` was rejected because it does not
  constrain project instructions, agent tools, or network access.
- Checking the pull request head out in a separate subdirectory and passing it
  with `--add-dir` was rejected because CodeQL still treats the untrusted
  checkout as flowing into a privileged comment-triggered action.
- Keeping only the default-branch checkout without read-only pull request API
  access was rejected because Claude could not inspect newly added files.
