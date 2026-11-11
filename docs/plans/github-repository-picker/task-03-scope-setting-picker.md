---
id: "03-scope-setting-picker"
title: "Repository scope uses the GitHub picker"
status: done
wave: 3
depends_on: ["02-watch-filter-picker"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/github-repository-picker.md"
---

# Task 03: Repository scope uses the GitHub picker

## Outcome

The GitHub workspace "Repository scope" setting selects repositories in
`repos` mode with the same picker instead of a comma-separated text field.

## Scope

- Draft repositories as `RepoFilter[]` in `useGitHubRepoScopeDraft`; compare
  canonical `owner/name` lists for baseline, dirty, discard, and the
  stale-response merge.
- Render `GitHubRepositoryMultiSelect` (no `scope`) in place of the repos
  `Input`; disabled unless mode is `repos`.
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
- The picker is not filtered by the saved scope.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/github/github-repo-scope-section.test.tsx
cd apps/web && pnpm run typecheck && pnpm run i18n:check
```

## Files likely touched

- `apps/web/components/github/github-repo-scope-section.tsx`
- `apps/web/components/github/github-repo-scope-section.test.tsx` (new)
- `apps/web/src/locales/*/github.json`

## Dependencies

Task 02.

## Results

- Repository scope draft is `RepoFilter[]`; `repos` mode renders `GitHubRepositoryMultiSelect` without scope filtering. The comma-separated validation and its locale key are removed.
- Focused tests passed: `github-repo-scope-section.test.tsx` (3).
