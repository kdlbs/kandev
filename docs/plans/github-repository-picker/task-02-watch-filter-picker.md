---
id: "02-watch-filter-picker"
title: "Watch filter uses the GitHub picker"
status: done
wave: 2
depends_on: ["01-shared-picker-panel"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/github-repository-picker.md"
---

# Task 02: Watch filter uses the GitHub picker

## Outcome

Automation `github_pr` / `github_pr_merged` triggers and GitHub review/issue
watch dialogs list accessible, in-scope GitHub repositories on open and match
a bare repository name.

## Scope

- Regression test first:
  `components/github/repo-filter-selector.test.tsx` — "lists accessible
  repositories on open and matches a bare repository name". It must fail on
  the current `RepoSearchCombobox` (empty list, owner-prefix hint).
- `lib/github/repository-input.ts`: `parseGitHubRepositoryInput` for
  `owner/name` and GitHub repo URLs.
- `components/github/github-repository-multi-select.tsx`: selected chips
  (`owner/*` for org entries), add trigger, popover with
  `RemoteRepositoryPickerPanel`, GitHub-only hook, scope filter, dedupe,
  typed entry with scope rejection message.
- `RepoFilterSelector`: replace `RepoSearchCombobox` and `SelectedFilters`
  with the multi-select; remove `useRepoSearch` and the `searchOrgRepos`
  import. Keep header, toggle, scope fetch, and org badges.
- Locale keys in all seven locales for any new copy (scope-rejection message).

## UI preview

UI-01 in [plan.md](plan.md#ui-01-watch-filter-repositories-automation-trigger-reviewissue-watch):

```text
Organizations  [kdlbs] [acme]
[kdlbs/kandev x] [acme/*  x]  [+ Add repository]
                 | Search repositories or paste a URL   |
                 | (gh) kdlbs/kandev        [check]     |
                 | (gh) acme/api              Private   |
```

## Exclusions

- Repository scope setting (task 03).
- Consumers of `RepoFilterSelector` (props unchanged).

## Requirements and design

- AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.1–.5, .7, .9
- [System design: GitHub multi-select, Typed entry, Watch filter](../../specs/integrations/system-design/github-repository-picker.md#github-multi-select)

## Acceptance

- The regression test passes; scope tests prove `orgs`/`repos` modes hide
  out-of-scope repos and reject out-of-scope typed entries.
- Picking a listed or typed repo emits `{ owner, name }` once; re-picking is a
  no-op; removing a repo or org chip emits the remaining list.
- Saved `RepoFilter[]` values render unchanged; emitted shape is unchanged.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/github/repo-filter-selector.test.tsx lib/github/repository-input.test.ts components/automations/trigger-configs components/github/issue-watch-dialog.test.tsx components/github/review-watch-dialog.test.tsx
cd apps/web && pnpm run typecheck && pnpm run i18n:check
```

## Files likely touched

- `apps/web/components/github/repo-filter-selector.tsx`
- `apps/web/components/github/repo-filter-selector.test.tsx` (new)
- `apps/web/components/github/github-repository-multi-select.tsx` (new)
- `apps/web/lib/github/repository-input.ts` (new)
- `apps/web/lib/github/repository-input.test.ts` (new)
- `apps/web/src/locales/*/github.json`
- `apps/web/lib/github/repo-scope.ts` (new)
- `apps/web/components/automations/trigger-configs/github-pr-config.tsx`
- `apps/web/components/automations/trigger-configs/github-pr-config.test.tsx`
- `apps/web/components/automations/trigger-card.tsx`

## Dependencies

Task 01.

## Results

- Regression test failed first on the old combobox (empty list on open), then passed.
- Additional root cause found: `github-pr-config.tsx` never passed `workspaceId`, so the automation "PR: opened" picker could not search at all. `trigger-card.tsx` now passes it; covered by `github-pr-config.test.tsx`.
- Scope helpers moved to `lib/github/repo-scope.ts`. Locale keys used only by the old combobox removed; four new keys added in all seven locales.
- Focused tests passed: `repo-filter-selector.test.tsx` (7), `repository-input.test.ts` (9), automation trigger configs (203 across `components/automations`).
