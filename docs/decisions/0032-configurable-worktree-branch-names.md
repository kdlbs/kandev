# 0032: Configurable Worktree Branch Names

**Status:** accepted
**Date:** 2026-07-07
**Area:** backend, frontend

## Context

Kandev generated worktree branch names from a fixed format: repository branch
prefix, sanitized task title, and a random suffix. Teams need repository-specific
branch policies, such as ticket-first branch names, and users need a visible way
to rename an already-created worktree branch without leaving Kandev.

## Decision

Repositories get a persisted `worktree_branch_template` setting. Branch
generation uses a small backend template renderer with documented placeholders:
`{title}`, `{title_full}`, `{task_id}`, `{suffix}`, `{ticket}`, and
`{issue_key}`. Users write literal prefixes directly in the template, such as
`feature/{ticket}-{title}`; `{prefix}` is not a placeholder. `{ticket}` and
`{issue_key}` resolve to the same external reference token from task identifier
or integration metadata.

The Changes panel exposes branch rename through the existing
`worktree.rename_branch` action. Rename is repo-scoped for multi-repo tasks, and
any persisted branch snapshots must be updated only for the renamed repository.

## Consequences

Existing repositories keep the same generated branch names by backfilling the
new template from the legacy `worktree_branch_prefix`. Templates may omit
`{suffix}`, but collisions are then possible and are reported from git instead
of being silently repaired. The backend has a single place to validate template
output before branch names reach git.

Multi-repo rename requires care because the existing session-wide cached branch
update helper is not safe for a single repository rename.

## Alternatives Considered

Keeping only `worktree_branch_prefix` was rejected because it cannot express
ticket-first or username-first policies. Making `{suffix}` mandatory was
rejected because some teams intentionally use deterministic branch names and are
willing to handle collisions.
