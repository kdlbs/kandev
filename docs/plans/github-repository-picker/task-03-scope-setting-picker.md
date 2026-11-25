---
id: "03-scope-setting-picker"
title: "Repository scope uses the GitHub picker"
status: done
wave: 3
depends_on: ["02-watch-filter-picker"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.6
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.9
system_design:
  - ../../specs/integrations/system-design/github-repository-picker.md
---

# Task 03: Repository scope uses the GitHub picker

## Outcome

The GitHub workspace "Repository scope" setting selects repositories in
`repos` mode with the same picker instead of a comma-separated text field.

## Scope

- Draft repositories as `RepoFilter[]` in `useGitHubRepoScopeDraft`; compare
  canonical `owner/name` lists for baseline, dirty, discard, and the
  stale-response merge.
- Render `GitHubRepositoryMultiSelect` with `repositorySource="scope-editor"`
  in place of the repos `Input`; disabled unless mode is `repos`.
- Add `GET /api/v1/github/workspace-settings/repositories` for scope-editor
  discovery. Keep the ordinary `GET /api/v1/github/repos` scope-filtered; both
  routes retain personal and automation visibility checks.
- Remove `parseRepoFilters`, `invalidRepos`, and the invalid-entry message and
  `invalidReason`; remove the now-unused locale key from all seven locales
  only if no other caller uses it.

## UI preview

UI-02 in [plan.md](plan.md#ui-02-repository-scope-setting-repos-mode):

```text
Mode [Selected repositories v]   Organizations [kdlbs, example-org      ]
                                 Repositories
                                 [kdlbs/kandev x] [example/api x] [+ Add repository]
```

## Exclusions

- Organizations field and mode selector.

## Requirements and design

- AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.6, .9
- [System design: Repository scope setting](../../specs/integrations/system-design/github-repository-picker.md#repository-scope-setting)

## Acceptance

- New `github-repo-scope-section.test.tsx`: saved repos render as chips;
  adding/removing marks the section dirty; save sends `repo_scope_repos` with
  the selected `{ owner, name }` list; discard restores the baseline.
- The picker loads from the scope-editor route, so it can add a repository that
  is outside the saved scope.
- The scope-editor service still excludes repositories that either the
  personal or automation identity cannot access.
- The ordinary repository-list route continues to apply the saved scope.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/github/github-repo-scope-section.test.tsx
cd apps/web && pnpm run typecheck && pnpm run i18n:check
cd apps/backend && go test ./internal/github/
cd apps && pnpm --filter @kandev/web test -- lib/api/domains/github-api.test.ts hooks/domains/integrations/use-remote-repositories-github.test.tsx
```

## Files likely touched

- `apps/web/components/github/github-repo-scope-section.tsx`
- `apps/web/components/github/github-repo-scope-section.test.tsx` (new)
- `apps/backend/internal/github/service_accessible_repos.go`
- `apps/backend/internal/github/service_routing_test.go`
- `apps/backend/internal/github/controller.go`
- `apps/backend/internal/github/controller_test.go`
- `apps/web/lib/api/domains/github-api.ts`
- `apps/web/lib/api/domains/github-api.test.ts`
- `apps/web/hooks/domains/integrations/use-remote-repositories.ts`
- `apps/web/hooks/domains/integrations/use-remote-repositories-github.test.tsx`

## Dependencies

Task 02.

## Contributor implementation results

- Repository scope draft is `RepoFilter[]`; `repos` mode renders `GitHubRepositoryMultiSelect` without a client-side scope filter. The comma-separated validation and its locale key are removed.
- Focused tests passed: `github-repo-scope-section.test.tsx` (3).

## Maintainer fixup results

- Added `ListScopeEditorReposForWorkspace` and its dedicated settings endpoint.
  The method omits the saved-scope predicate but retains the personal and
  automation repository intersection. `ListAccessibleReposForWorkspace` keeps
  its existing scope enforcement.
- The settings picker selects the dedicated source. Its UI test starts with a
  restricted saved scope, adds a repository returned by scope-editor discovery,
  and saves both entries.
- Red/green regression: Go
  `TestScopeEditorReposIgnoreSavedScopeAndKeepAutomationVisibility` and
  `TestHandleListScopeEditorRepos_OK`; web API, hook, and scope-section suites.
- Verification passed: `go test ./internal/github/` and the focused three-file
  Vitest run (41 tests).
