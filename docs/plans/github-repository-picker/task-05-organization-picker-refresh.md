---
id: "05-organization-picker-refresh"
title: "Organization picker and list refresh"
status: in_progress
wave: 5
depends_on: ["04-e2e"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.5
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.10
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.11
system_design:
  - ../../specs/integrations/system-design/github-repository-picker.md
---

# Task 05: Organization picker and list refresh

## Outcome

The repository scope's organizations field and the watch filter's organization
badges become one chip-based organization picker, and both pickers gain a
refresh control that bypasses the server's repository-list cache.

Maps to AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.5, .10, .11.

## Scope

- Backend: `ListAccessibleReposForWorkspace(..., refresh bool)` and
  `listAccessibleReposWithClient(..., refresh bool)` skip the cache read when
  `refresh` is set and store the fresh list; `httpListAccessibleRepos` maps
  `refresh=true`. Test: `service_accessible_repos_test.go`
  "RefreshBypassesAndReplacesCache".
- `fetchAccessibleRepos({ refresh })` sends `refresh=true`; the built-in GitHub
  source in `use-remote-repositories.ts` sends it for the first successful load
  after each `refresh()`.
- New `components/picker-controls.tsx`: `isPlainEnter`, `PickerSearchRow`
  (search input plus optional refresh button), `AlreadyAddedMarker`. The
  repository panel adopts it and shows refresh when `accessible.refresh`
  exists.
- New `components/github/selection-chip.tsx` shared by both multi-selects.
- New `hooks/domains/github/use-github-orgs.ts` (`orgs`, `loading`, `error`,
  `refresh`) and `components/github/github-organization-multi-select.tsx`.
- New `parseGitHubLogin` in `lib/github/repository-input.ts`.
- `repo-filter-selector.tsx`: organization picker replaces `OrgBadges`.
- `github-repo-scope-section.tsx`: organizations draft is `string[]` rendered
  by the organization picker.
- Locales: `addOrganization`, `removeOrganization`, `searchOrganizations…`,
  `organizationOutsideWorkspaceScope`, `pressEnterToAddOrganization`,
  `noOrganizationsFound`, `couldNotLoadOrganizations`, refresh labels; remove
  keys left unused.
- E2E: desktop scope spec adds an organization through the picker; automation
  spec covers organization selection and refresh revealing a newly seeded
  repository; phone spec checks the refresh and organization controls.

## ASCII preview

```text
Mode [Organizations v]   Organizations
                         [kdlbs x] [acme x] [+ Add organization]
                                     +-------------------------------+
                                     | Search organizations    [(R)] |
                                     | octocat (you)          [chk]  |
                                     | kdlbs                  [chk]  |
                                     | oss-tools                     |
                                     +-------------------------------+
```

Watch filter: the `Organizations` row shows the same chips and add control in
place of the toggle badges; the repository row below is unchanged except for
the refresh button in its picker.

## Acceptance

- Failing-first tests: the Go refresh test; `github-organization-multi-select.test.tsx`
  (lists, filters, typed entry, scope rejection, dedupe, refresh);
  `repo-filter-selector.test.tsx` org picker case; `github-repo-scope-section.test.tsx`
  org save case; `remote-repository-picker.test.tsx` refresh case;
  `use-remote-repositories.test.tsx` refresh-bypass case.

## Commands

```bash
make -C apps/backend test PKG=./internal/github/...   # or: cd apps/backend && go test ./internal/github/
cd apps && pnpm --filter @kandev/web test -- components/github components/remote-repository-picker.test.tsx hooks/domains/integrations/use-remote-repositories.test.tsx hooks/domains/github lib/github lib/api/domains/github-api.test.ts
cd apps/web && pnpm run typecheck && pnpm run i18n:check
cd apps/web && pnpm e2e:run -- tests/automations-github-repository-picker.spec.ts tests/mobile-automations-github-repository-picker.spec.ts tests/integrations/github-workspace-settings.spec.ts tests/automations-pr-merged-trigger.spec.ts tests/task/create-task-remote-repo.spec.ts
make fmt && make typecheck test lint
```

## Results

- Failing first, then passing: Go `TestListAccessibleRepos_RefreshBypassesAndReplacesCache`;
  `use-remote-repositories` cache-bypass case; `fetchAccessibleRepos` refresh
  URL case; `RemoteRepositoryPickerPanel` refresh cases; `parseGitHubLogin`;
  `useGitHubOrgs`; `GitHubOrganizationMultiSelect` (9 cases); watch-filter
  organization cases; scope-section organization cases.
- Affected unit suites: 160 files, 1485 tests pass. `go test ./internal/github/`
  passes; `scripts/lint-go-changed` reports 0 issues.
- E2E: desktop picker, GitHub settings, PR-merged, and task-create remote-repo
  specs pass (21 + 25 runs, chromium); phone spec passes (2 tests).
- `automations-pr-merged-trigger.spec.ts` "workspaceId is passed to the orgs
  request" now turns "All repositories" off first: organizations are fetched
  only when the organization picker mounts.
- PR screenshots showed both picker popovers flush against the phone's right
  edge once a chip shifted the trigger. Both now pass `collisionPadding={8}`;
  the phone spec asserts an 8px margin after reopening (failed at 393px of a
  393px viewport before the fix).
- Review fixups: Enter stays inside both pickers and arrow keys move focus
  (unit tests plus a review-watch dialog E2E with Update enabled; the old code
  did not leak Enter in that dialog either, so the E2E guards the contract);
  typed Enterprise URLs use the workspace `githubHost`; submit errors show
  while GitHub is unavailable except in task creation; loading rows announce a
  translated status. Hook GitHub-specific tests moved to
  `use-remote-repositories-github.test.tsx` to stay under the file-size limit.
- Further review fixups: typed URLs must match the workspace host; GitHub-only
  pickers ignore GitLab/Azure DevOps status loading; both pickers load their
  lists only when opened, so the PR-merged E2E now opens the organization
  picker before asserting the orgs request.
- Organization rows show logins only; the backend's prepended account login is
  listed first without a separate "you" label.

## Maintainer review correction

- Repository URLs require a known workspace host. Exact `owner/name` entry
  remains available while status loads or fails. Unit coverage includes the
  unknown-host parser and the picker transition to a known Enterprise host.
- Inputs and chip actions use the shared control-sizing helper. Repository
  and organization rows retain touch targets at coarse-pointer tablet widths.
- Desktop E2E coverage measures 28px controls. Mobile E2E coverage also resizes
  the configured touch device to a tablet width and measures both pickers.
- The prior review session did not run validation. This fixup run verified the
  focused repository picker, API, and settings suites; fresh tablet E2E
  evidence remains pending on the current PR head.
