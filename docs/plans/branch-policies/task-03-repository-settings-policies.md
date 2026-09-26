---
id: "03-repository-settings-policies"
title: "Build repository policy settings"
status: complete
wave: 2
depends_on:
  - "01-branch-policy-persistence-api"
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-BRANCH-POLICIES-001
  - REQ-WORKSPACES-BRANCH-POLICIES-002
  - REQ-WORKSPACES-BRANCH-POLICIES-005
acceptance_criteria:
  - AC-WORKSPACES-BRANCH-POLICIES-001.1
  - AC-WORKSPACES-BRANCH-POLICIES-001.2
  - AC-WORKSPACES-BRANCH-POLICIES-001.3
  - AC-WORKSPACES-BRANCH-POLICIES-001.4
  - AC-WORKSPACES-BRANCH-POLICIES-001.6
  - AC-WORKSPACES-BRANCH-POLICIES-002.1
  - AC-WORKSPACES-BRANCH-POLICIES-002.2
  - AC-WORKSPACES-BRANCH-POLICIES-002.3
  - AC-WORKSPACES-BRANCH-POLICIES-002.4
  - AC-WORKSPACES-BRANCH-POLICIES-002.5
  - AC-WORKSPACES-BRANCH-POLICIES-005.1
  - AC-WORKSPACES-BRANCH-POLICIES-005.2
  - AC-WORKSPACES-BRANCH-POLICIES-005.3
system_design: "../../specs/workspaces/system-design/branch-policies.md"
---

# Task 03: Build repository policy settings

## Objective

Add compact, self-documenting policy management to saved repository settings,
including responsive CRUD and a guided Gitflow starter.

## TDD contract

Write frontend tests first for collapsed-by-default behavior, policy count,
immediate confirmed mutations, form validation, Gitflow preview/submission, and
desktop-dialog versus phone-drawer composition.

## Implementation scope

- Add policy transport functions and a repository-keyed store slice hydrated by
  boot state and semantic events.
- Add a collapsed `Branch policies` section to the existing repository editor.
- Add immediate create/edit/delete flows without registering a settings-save
  contributor or adding a page-local Save button.
- Add visible field explanations and focusable info controls. Use tooltip or
  popover on fine pointers and the existing touch drawer behavior on coarse
  pointers.
- Add a desktop dialog and phone full-height drawer with shared form logic, one
  scroll owner, safe-area actions, and 44 CSS pixel touch targets.
- Add the Gitflow starter with production/development selectors, four-policy
  preview, and atomic submission.
- Reuse the searchable, refreshable branch selector for policy and Gitflow
  branch fields, preserving distinct local and remote refs.
- Add all new copy to English, Portuguese, Simplified Chinese, and both
  Traditional Chinese catalogs; run the existing Traditional Chinese generator.

## Implementation acceptance

- Saved repositories expose a collapsed, counted policy section with immediate
  confirmed CRUD and no settings-route save contribution.
- The Gitflow starter previews and submits the exact four-policy mapping from
  the requirements.
- Desktop and phone use equivalent form/help content through native dialog and
  drawer interactions.

## Exclusions

- Policy selection during task creation.
- Policy reordering or a default-policy setting.
- Management on unsaved repository drafts.

## Files likely touched

- `apps/web/app/settings/workspace/workspace-repositories-client.tsx`
- `apps/web/components/settings/repository-card.tsx`
- `apps/web/components/settings/`
- `apps/web/lib/api/domains/`
- `apps/web/lib/store/`
- `apps/web/src/locales/`

## Verification

- `cd apps/web && pnpm exec vitest run app/settings/workspace/workspace-repository-branch-policies.test.tsx lib/api/domains/repository-branch-policies-api.test.ts`
- `cd apps/web && pnpm run typecheck`
- `cd apps/web && pnpm run i18n:check`

Run `cd apps && pnpm install --frozen-lockfile` once first if this worktree has
no dependencies installed.

## Dependencies and parallelism

Depends on Task 01. Sequential by default with Task 02 because both consume the
new policy transport contract; coordinate shared model/type changes if execution
is later parallelized.

## Output contract

Report the RED tests, desktop and phone composition, help interaction parity,
save-coordinator behavior, locale changes, exact verification results, changed
files, and risks. Mark this task and the plan checkbox complete together.

## Results

Implemented repository-keyed policy hydration, REST/WebSocket client methods,
collapsed counted settings, immediate CRUD, Gitflow starter, shared desktop
dialog and phone drawer forms, touch help controls, and five-language copy.
Unsaved repository drafts remain save-first and do not add a settings-save
contributor.

Verification:

- Focused frontend Vitest suite passed: 78 tests in 5 files.
- `pnpm run typecheck` passed.
- `pnpm run lint` passed with zero warnings.
- `pnpm run i18n:check` passed; all required catalogs are complete.
