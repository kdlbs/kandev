---
id: "02-own-page-sliders"
title: "Own-page enable/disable slider for Azure DevOps, GitHub, GitLab"
status: done
wave: 2
depends_on: ["01-enabled-hooks"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/enable-disable-toggle.md"
---

# Task 02: Own-page enable/disable slider for Azure DevOps, GitHub, GitLab

- **Acceptance:**
  1. `/settings/integrations/azure-devops`, `/settings/integrations/github`
     and `/settings/integrations/gitlab` each render a slider in their top
     connection section's header (via `<SettingsSection action={...}>`),
     reflecting and persisting that integration's `useXEnabled()` state,
     visually and behaviorally identical to Jira's existing
     `<JiraEnabledControl />` (draft + shared save bar, `data-settings-dirty`
     wiring via `useDraftedIntegrationEnabled`).
  2. A component test per new control confirms it renders
     `<DraftedIntegrationEnabledControl>` wired to the right hook (or, if
     judged low-value per the plan's Tests section, the wiring is instead
     proven by Task 06's E2E "own-page slider" assertion — state which choice
     was made in Results).
- **Verification:** `cd apps && pnpm --filter @kandev/web test -- azure-devops-settings gitlab-settings linear-settings sentry-settings slack-settings`
- **Files likely touched:**
  - `apps/web/components/azure-devops/azure-devops-enabled-control.tsx` (new)
  - `apps/web/components/azure-devops/azure-devops-settings.tsx` (edit)
  - `apps/web/components/github/github-enabled-control.tsx` (new)
  - `apps/web/components/github/github-settings.tsx` (edit)
  - `apps/web/components/gitlab/gitlab-enabled-control.tsx` (new)
  - `apps/web/components/gitlab/gitlab-settings.tsx` (edit)
- **Dependencies:** `01-enabled-hooks`.
- **Parallelism:** `parallel-safe` with `03-index-page-sliders` (disjoint
  files: this task never touches `apps/web/app/settings/integrations/page.tsx`
  or the e2e layout helper).
- **Inputs:**
  - Spec: "What" (own-page slider requirement), "API surface".
  - Plan: "Task 02" section, exact component/wiring shapes.
  - Pattern to mirror exactly: `apps/web/components/jira/jira-enabled-control.tsx`
    and its usage in `apps/web/components/jira/jira-settings.tsx:576`
    (`action={<JiraEnabledControl />}`).
- **Output contract:** summary, files changed, exact test command(s) run and
  their output, blockers/risks, and update this task's `status` to `done`
  plus the corresponding checkbox in `plan.md`.

## Results

- Added `AzureDevOpsEnabledControl`, `GitHubEnabledControl`,
  `GitLabEnabledControl` — one-line wrappers mirroring
  `jira-enabled-control.tsx`, each rendering
  `<DraftedIntegrationEnabledControl>` wired to its Task 01 hook.
- Wired each into its integration's top `<SettingsSection action={...}>`:
  `AzureDevOpsConnectionSection`, `GitHubConnectionSection`,
  `GitLabConnectionCard` (GitLab's `action` already held a "Refresh" button;
  combined both into one `<div className="flex items-center gap-2">`).
- Also extracted the pre-existing, module-local, unexported `EnabledPill`
  components out of `linear-settings.tsx`, `sentry-settings.tsx`, and
  `slack-settings.tsx` into standalone exported files
  (`linear-enabled-control.tsx`, `sentry-enabled-control.tsx`,
  `slack-enabled-control.tsx`), matching the Jira/Azure/GitHub/GitLab
  file-per-control convention — required so Task 03's index page can import
  and reuse all seven controls instead of duplicating their logic inline.
  Not in the original plan text but necessary to satisfy Task 03's design
  ("one component per slug, each calling exactly one hook unconditionally")
  without new duplicate `EnabledPill`-shaped components.
- Fixed a latent gap surfaced by adding the new `action`: the existing
  `azure-devops-settings.test.tsx` rendered `AzureDevOpsConnectionSection`
  directly (no `SettingsSaveProvider` ancestor), which the new
  `AzureDevOpsEnabledControl` → `useDraftedIntegrationEnabled` →
  `useSettingsSaveContributor` now requires. Wrapped the four affected
  `render(...)` calls in `<SettingsSaveProvider>`.
- Commands:
  - `cd apps && pnpm --filter @kandev/web test -- azure-devops-settings github-settings gitlab-settings` → `Test Files 2 passed (2)`, `Tests 13 passed (13)` (no `github-settings.test.tsx` exists).
  - `cd apps && pnpm --filter @kandev/web test -- linear-settings sentry-settings slack-settings` → `Test Files 2 passed (2)`, `Tests 6 passed (6)`, plus `sentry-settings.test` run explicitly → `Test Files 1 passed (1)`, `Tests 4 passed (4)`.
  - `cd apps/web && pnpm run typecheck` → clean.
- Files changed: the six files listed above under "Files likely touched",
  plus `apps/web/components/linear/linear-enabled-control.tsx`,
  `apps/web/components/linear/linear-settings.tsx`,
  `apps/web/components/sentry/sentry-enabled-control.tsx`,
  `apps/web/components/sentry/sentry-settings.tsx`,
  `apps/web/components/slack/slack-enabled-control.tsx`,
  `apps/web/components/slack/slack-settings.tsx`,
  `apps/web/components/azure-devops/azure-devops-settings.test.tsx`.
- Acceptance criterion 2 decision: no thin per-control wrapper test was
  added — no other integration's `*-enabled-control.tsx` has one either
  (checked `jira-enabled-control.tsx`; no sibling test exists), and the
  wiring is proven end-to-end by Task 06's `integrations-index-enabled-toggle.spec.ts`
  (own-page slider reflects the index-page toggle after Save).
- Blockers/risks: none.
