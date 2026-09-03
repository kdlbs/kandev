---
id: "02-publish-mixed-change-facets"
title: "Publish mixed-change facets"
status: completed
wave: 2
depends_on:
  - "01-capture-mixed-change-regression"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
acceptance_criteria:
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.7
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.8
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.9
system_design:
  - ../../specs/platform/system-design/workspace-git-status.md
---

# Task 02: Publish mixed-change facets

## Summary

Extend the agentctl status wire additively and populate independent staged and unstaged metadata for
mixed paths. Preserve legacy fields and the established enrichment limits.

## In scope

- Add the optional facet type and fields to `FileInfo`.
- Parse index and working-tree porcelain columns independently for ordinary supported states.
- Enrich `HEAD -> index`, `index -> worktree`, and flattened compatibility diffs.
- Include facet content in budget, truncation, carry-forward, and skip-reason logic.

## Out of scope

- Frontend facet rendering.
- New behavior for unmerged/conflict states.
- New Git routes.

## Acceptance

- `MM` and `AM` produce one path entry with both accurate facets.
- Pure staged, pure unstaged, and untracked paths retain the legacy compact representation.
- Facet content cannot bypass the 256 KiB representation cap or shared 2 MiB threshold.

## Verification

```bash
cd apps/backend
go test ./internal/agentctl/server/process -run 'TestWorkspaceGitStatusPreservesMixedChangeFacets|TestMixedChangeFacetDiffs|TestMixedChangeFacetBudget|TestCarryForwardMixedChangeFacets'
go test ./internal/agentctl/types/streams ./internal/agentctl/server/process
```

## Files likely touched

- `apps/backend/internal/agentctl/types/streams/git.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_status.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_diff.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_mixed_changes_test.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_diff_test.go`

## Dependencies

Task 01 red regression evidence.

## Risks

- Rename numstat paths and compatibility projections can address different old/new names.
- Carrying prior facet content after an enrichment failure must not overwrite a fresh sibling facet.

## Parallelism

`sequential`

## Inputs

- Requirement acceptance criteria `.7`, `.8`, and `.9`.
- Mixed-change section of the workspace Git status design.
- ADR-2026-08-27-mixed-git-change-facets.

## Results

- Added optional `staged_change` and `unstaged_change` wire facets while preserving the flattened
  compatibility fields and compact pure-change representation.
- Parsed supported porcelain index/worktree states independently and enriched mixed paths with
  `HEAD -> index` and `index -> worktree` diffs.
- Shared representation accounting now includes both facets; same-HEAD carry-forward preserves each
  facet independently without bypassing an exhausted budget.
- `go test ./internal/agentctl/types/streams ./internal/agentctl/server/process` passed.
