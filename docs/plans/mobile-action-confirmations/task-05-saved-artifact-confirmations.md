---
id: "05-saved-artifact-confirmations"
title: "Phone saved artifact deletion confirmations"
status: done
wave: 2
depends_on:
  - "01-shared-mobile-surfaces"
plan: "plan.md"
requirements:
  - REQ-UI-MOBILE-CONFIRMATION-001
  - REQ-UI-MOBILE-CONFIRMATION-002
  - REQ-UI-MOBILE-CONFIRMATION-003
acceptance_criteria:
  - AC-UI-MOBILE-CONFIRMATION-001.1
  - AC-UI-MOBILE-CONFIRMATION-001.2
  - AC-UI-MOBILE-CONFIRMATION-001.3
  - AC-UI-MOBILE-CONFIRMATION-001.4
  - AC-UI-MOBILE-CONFIRMATION-001.5
  - AC-UI-MOBILE-CONFIRMATION-002.1
  - AC-UI-MOBILE-CONFIRMATION-002.2
  - AC-UI-MOBILE-CONFIRMATION-002.3
  - AC-UI-MOBILE-CONFIRMATION-002.6
  - AC-UI-MOBILE-CONFIRMATION-003.1
  - AC-UI-MOBILE-CONFIRMATION-003.2
  - AC-UI-MOBILE-CONFIRMATION-003.5
  - AC-UI-MOBILE-CONFIRMATION-003.6
system_design:
  - ../../specs/ui/system-design/mobile-action-confirmations.md
  - ../../specs/ui/system-design/saved-task-view-deletion-confirmation.md
---

# Task 05: Phone Saved Artifact Deletion Confirmations

## Summary

Adopt the mobile confirmation contract for reusable views, filters, prompts,
layouts and profiles. Their existing stores, eligibility and delete callbacks
stay authoritative. Migrate the shared saved-view adapter before its hosts.

## In scope

- Update the saved-task-view shell and its sidebar/Threads/integration owners.
  Use a step in current filter/editor drawers, preserving unsaved form state.
- Update prompt, layout-profile, action-preset, saved-layout and agent-profile
  confirmation adapters, keeping the same deleted target and visible context.
- Cover exact target IDs, built-in/last-view guards, pending/default-marker
  isolation, current rollback, persistence, focus, and phone geometry.

## Out of scope

Saved-view/profile models, filtering, last-view policies, provider data,
persistence queues, plugin APIs, and non-phone editor composition.

## Acceptance

1. Every manifest entry in this group uses the shared phone surface; Cancel
   restores its current editor/list state without a delete or row selection.
2. Confirmation invokes the same target-specific delete once and retains
   protected-item and current rollback/persistence behavior.
3. Every distinct mobile host and the shared dropdown handoff is exercised;
   desktop saved-view popovers retain their existing boundaries.

## Verification

Read `components/threads/AGENTS.md`, then use `/tdd` and `/e2e`. Add focused
adapter cases to existing tests, plus new prompt/layout-profile tests if those
adapters have no direct coverage. The listed broader component files already
cover the remaining owners.

```bash
cd apps/web
pnpm exec vitest run components/confirmation/saved-task-view-delete-confirmation.test.tsx components/task/sidebar-filter/sidebar-filter-popover.test.tsx components/threads/threads-view-controls.test.tsx components/integrations/presets-scope-bar-base.test.tsx components/github/my-github/presets-sidebar.test.tsx components/gitlab/my-gitlab/presets-sidebar.test.tsx components/jira/my-jira/list-toolbar.test.tsx components/task/layout-preset-selector.test.tsx components/settings/agent-profile-delete-dialog.test.tsx components/settings/prompt-delete-confirmation.test.tsx components/settings/layouts/layout-profile-delete-confirmation.test.tsx
pnpm e2e:run --project mobile-chrome tests/task/mobile-sidebar-views.spec.ts tests/task/mobile-threads-view.spec.ts tests/github/mobile-github-sidebar.spec.ts tests/gitlab/mobile-gitlab-issue-milestone-filter.spec.ts tests/integrations/mobile-azure-devops.spec.ts tests/integrations/mobile-jira-saved-view.spec.ts
pnpm e2e:run --project mobile-chrome tests/settings/mobile-prompts-settings.spec.ts tests/settings/mobile-layout-profiles.spec.ts tests/settings/mobile-agent-profile-delete.spec.ts
pnpm e2e:run --project chromium tests/task/sidebar-filter.spec.ts tests/task/threads-view.spec.ts tests/github/github-scope-bar.spec.ts tests/settings/agent-profile-delete.spec.ts
pnpm run typecheck
pnpm run i18n:check
pnpm run i18n:ratchet
```

Extend the listed specs with missing delete/Cancel outcomes rather than treating
an unrelated layout assertion as coverage. Test long translated target names
and unsaved filter controls through the real hosted surface.

## Files likely touched

- Both 05 inventory rows in `plan.md`, including the shared confirmation hook where target-lifetime integration requires it.
- Existing filter/editor drawer owners and their component tests, including new prompt/layout-profile adapter tests.
- Listed E2E files and relevant locale catalogs; no saved-view persistence modules.

## Dependencies

01 supplies the shared host and renderer.

## Risks

Delete controls must not activate a view or its default marker. Closing a menu
can destroy its candidate unless state lives at a stable owner. Hidden editors
must retain unsaved values without writing them to persistence.

## Parallelism

`sequential`

## Inputs

Mobile design plus the linked saved-view requirement/design; current shell,
saved-view hooks, host components/tests, and completed saved-view deletion plan.

## Results

Implemented the shared saved-view adapter, sidebar/Threads and GitHub/GitLab
modal hosts, transient Jira/integration/layout-menu handoff, and prompt,
layout-profile and agent-profile adapters. Original controls stay mounted on
phones. Cancel restores focus and drafts; existing persistence, eligibility,
default-view rules and full conflict alerts remain unchanged.

Observed behavioral RED tests before adoption. All 80 focused component tests
passed across 13 files, including desktop/coarse-pointer regressions. Ten
mobile browser flows passed across the sidebar, Threads, GitHub, GitLab, Jira,
Azure DevOps, prompts, layouts, and both agent-profile entry points. Successful
deletions include persisted-state checks where supported. Inspected GitHub's
same-sheet confirmation step and the standalone profile sheet screenshots.
Changed production files pass ESLint without warnings; typecheck passed.

Seven desktop saved-artifact regressions passed in the final compatibility
run: GitHub saved query persistence, sidebar deletion and last-view protection,
Threads deletion, and three agent-profile deletion/conflict cases. Combined
affected-component and static-check results are recorded in the manifest.
