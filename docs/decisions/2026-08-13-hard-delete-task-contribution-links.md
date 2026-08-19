# ADR-2026-08-13-hard-delete-task-contribution-links: Hard delete owns task contribution links

**Status:** accepted
**Date:** 2026-08-13
**Area:** backend

## Context

GitHub and GitLab contribution tables associate a pull request or merge request with a task, but their existing SQLite schemas do not enforce that ownership with a foreign key. Keeping those rows after a hard task delete produces inaccessible orphaned state, while archive remains the recoverable task lifecycle.

## Decision

Task contribution associations and their refresh watches survive backend restart and archive/unarchive. A hard `task.deleted` event removes the GitHub and GitLab rows by task ID, and provider-store initialization removes historical rows without a matching `tasks.id`.

## Consequences

Provider cleanup is idempotent and repairs missed event delivery on the next startup without rebuilding existing SQLite tables. A future contribution-history feature must introduce an explicitly owned history model rather than relying on deleted task associations.

## Alternatives Considered

Rebuilding the association tables with foreign keys would make deletion atomic but has unnecessary migration risk. Retaining rows as implicit history was rejected because the rows have no first-class history owner or usable task surface.
