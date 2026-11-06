---
status: draft
system: workspaces
requirements:
  - REQ-WORKSPACES-ROOT-REVERT-001
---

# Root commit revert availability system design

## Purpose and boundaries

Workspaces owns the repository and Git operation; the Changes timeline is its presentation. This design adds a presentation guard using existing local Git ancestry. It does not alter Git, permissions, persistence, or provider contracts. The adjacent [PR-only commit requirement](../../ui/requirements/pr-only-commit-details.md) retains source identity and provider-only restrictions.

## Requirement mapping

- AC .1: local parent evidence and shared row action eligibility.
- AC .2: preserve missing versus empty parent and repository-qualified dispatch.
- AC .3: existing history adapters and source/pushed/latest gates remain authoritative.

## Data and flow

`GitOperator.GetLog` parses Git's `%P` and retains its first parent in `GitCommitInfo.ParentSHA`; an empty value identifies a root. The agentctl client decodes the same field, and `computeGitCommits` returns that Git log result as the `session.git.commits` response. `useSessionCommits` puts the response records in the environment-keyed `SessionCommit` store. The tracker event path also maps `parent_sha` through `git-status.ts` into that store.

`GitOperator.Commit` must obtain parent metadata for its emitted tracker notification using a successful Git query that returns the parent list, such as `git show -s --format=%P HEAD`. A successful empty result denotes a root; when multiple parents are returned, retain the first as `GetLog` does. The existing `HEAD~1` query fails for a root and its combined error output is currently forwarded as though it were a parent. A real tracker subscriber observed that payload and an ordinary commit's actual parent as a passing control. The event path updates the cached HEAD and does not itself force a commit-list refetch, so a later `%P` read is not an immediate correction.

`useCommitSections` chooses `mergeCommits` for unified history and `separateCommitHistories` for divergence. Both local mappings spread the original local record; their input and output types, and `CommitItem`, must explicitly carry an optional `parent_sha`. An explicit empty string is known parent absence; an omitted field is unknown legacy data and must not be defaulted to empty. Provider-only records do not supply local ancestry and stay read-only. Matching a provider record must not replace a local record's parent evidence.

`CommitHistoryRow` and `RepoGroup` continue to suppress handlers for pushed commits and determine latest eligibility. In `CommitRow`, derive an eligible revert handler or equivalent single predicate from local source, latest status, existing handler availability, and `parent_sha !== ""`. Apply that same result to `CommitContextMenu`, `CommitRowActions`, and any enclosing action-presence decision so a revert-only root does not create an empty menu or hover group. Preserve Amend and Reset independently. Dispatch still passes the row SHA and `repository_name` through the existing handler.

The context menu is the existing long-press surface on touch; fine-pointer rows also have hover actions. The correction changes eligibility only, without introducing a new surface, hit target, layout, or copy. Test both unified and separate mapping paths, both row adapters, actual desktop context and hover, coarse-pointer and phone long-press, plus an ordinary-commit control. Native browser checks must exercise the rendered menu, not a mocked primitive.

## Failure, security, and observability

Unknown ancestry preserves legacy eligibility, so the existing backend error remains possible if metadata is absent or stale. This correction applies authoritative root/first-parent interpretation only to successful parent queries. Preserve the producer's existing error handling when that query fails; the existing string field cannot represent a distinct unknown value and a failed query is not proof of root ancestry. If a failed query returns empty combined output, the existing event shape can still be mistaken for a known root by the UI. Preventing that case while retaining notification behavior requires a separate representation or recovery path and is outside this narrow correction. The backend verifies current HEAD and runs `git reset --soft HEAD~1`; this UI guard is not authorization or a concurrency guarantee. It adds no persistence, migration, telemetry, or logging.

## Related decisions

- [Provider history as Changes enrichment](../../../decisions/2026-08-13-provider-history-changes-enrichment.md)
- [Separate contribution and checkout histories](../../../decisions/2026-08-10-remote-contribution-head-drift.md)
