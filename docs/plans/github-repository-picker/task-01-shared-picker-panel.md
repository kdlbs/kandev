---
id: "01-shared-picker-panel"
title: "Shared remote repository picker panel"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.1
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.2
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.7
system_design:
  - ../../specs/integrations/system-design/github-repository-picker.md
---

# Task 01: Shared remote repository picker panel

## Outcome

The task-creation remote picker's searchable list becomes a reusable
`RemoteRepositoryPickerPanel`, and `useRemoteRepositories` can be restricted to
GitHub. Task creation looks and behaves exactly as before.

## Scope

- Extract `RemoteRepoPopoverContent`, `PickerList`, `RepoOption`,
  `AlreadyAddedMarker`, `ConnectProvidersBanner`, and provider tabs from
  `task-create-dialog-remote-repo-chip.tsx` into
  `components/remote-repository-picker.tsx` with the props named in the
  system design (`accessible`, `onPick`, `onSubmitText`, `isTextSubmittable`,
  `isRepoAllowed?`, `isRepoSelected`, `placeholder`).
- `RemoteRepoPill` renders the panel with its current URL-paste handling.
- Add `options.providers` to `useRemoteRepositories`; omitted means current
  behavior.

## Exclusions

- No GitHub watch or scope changes (tasks 02–03).
- No copy change in task creation.

## Requirements and design

- AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.1, .2, .7
- [System design: Shared picker panel, Provider restriction](../../specs/integrations/system-design/github-repository-picker.md#shared-picker-panel)

## Acceptance

- Panel unit tests: lists repos without input, filters by bare-name substring,
  hides `isRepoAllowed === false` repos, marks selected repos, calls
  `onSubmitText` on Enter and shows a returned error.
- `useRemoteRepositories(ws, { providers: ["github"] })` requests only the
  GitHub list (no GitLab, Azure DevOps, or plugin loads); the default call is
  unchanged.
- Existing `task-create-dialog-remote-repo-chip*.test.tsx` and
  `task-create-dialog-remote-repo-chips*.test.tsx` pass unchanged.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/remote-repository-picker.test.tsx components/task-create-dialog-remote-repo-chip hooks/domains/integrations/use-remote-repositories.test.tsx
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/components/remote-repository-picker.tsx` (new)
- `apps/web/components/remote-repository-picker.test.tsx` (new)
- `apps/web/components/task-create-dialog-remote-repo-chip.tsx`
- `apps/web/hooks/domains/integrations/use-remote-repositories.ts`
- `apps/web/hooks/domains/integrations/use-remote-repositories.test.tsx`

## Dependencies

None.

## Results

- `RemoteRepositoryPickerPanel` extracted; task creation renders it with its URL-paste handler. Typed submission returns `accepted`, `ignored`, or `rejected` with a message.
- `useRemoteRepositories(ws, { providers })` restricts built-in and plugin loads.
- Focused tests passed: picker panel (5), hook (18 including provider restriction), remote chip suites unchanged.
