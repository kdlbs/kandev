---
id: "03-scope-setting-picker"
title: "Repository scope uses the GitHub picker"
status: in_progress
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

- Repository scope draft is `RepoFilter[]`; `repos` mode renders `GitHubRepositoryMultiSelect` without a client-side scope filter. The comma-separated validation and its locale key are removed.
- Focused tests passed: `github-repo-scope-section.test.tsx` (3).

## Open review blocker

The backend still filters the repository list by the saved scope. The mocked
picker test does not cover that boundary. Acceptance requires an authorized
discovery source and a regression that starts with a restricted saved scope,
then selects and saves an additional accessible repository. The ordinary
repository endpoint must retain its existing scope enforcement.
