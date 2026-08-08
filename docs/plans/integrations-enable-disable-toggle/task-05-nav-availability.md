---
id: "05-nav-availability"
title: "Decouple left-panel nav visibility from the enabled toggle"
status: done
wave: 4
depends_on: ["01-enabled-hooks", "04-hide-disabled-setting"]
plan: "plan.md"
spec: "../../specs/integrations/enable-disable-toggle.md"
---

# Task 05: Decouple left-panel nav visibility from the enabled toggle

- **Acceptance:**
  1. `useNavAvailability()` returns `true` for a configured integration that
     is disabled when `hideDisabled` is `false` (the default), and `false`
     for the same integration when `hideDisabled` is `true`.
  2. `useNavAvailability()` returns `false` for an unconfigured integration
     regardless of `enabled`/`hideDisabled`.
  3. `apps/web/hooks/use-nav-availability.test.ts` and
     `apps/web/components/integrations/integrations-menu.test.ts` pass with
     updated mocks, and cover both new cases from criteria 1-2 in a
     table-driven test over the five nav-gated keys.
- **Verification:** `cd apps && pnpm --filter @kandev/web test -- hooks/use-nav-availability components/integrations/integrations-menu`
- **Files likely touched:**
  - `apps/web/hooks/use-nav-availability.ts`
  - `apps/web/hooks/use-nav-availability.test.ts`
  - `apps/web/components/integrations/integrations-menu.test.ts`
- **Dependencies:** `01-enabled-hooks`, `04-hide-disabled-setting`.
- **Parallelism:** sequential.
- **Inputs:** spec Scenarios, plan Task 05 derivation formula,
  `use-jira-availability.ts`'s/`use-linear-availability.ts`'s existing
  `useJiraAuthed`/`useLinearAuthed` exports.
- **Output contract:** summary, files changed, exact test command run and
  its output, blockers/risks, task/plan status update.

## Results

- `useNavAvailability` now computes, for each of the five nav-gated keys,
  `configured && (!hideDisabled || enabled)`:
  - `configured` reuses the exact pre-existing signals unchanged for
    azure-devops (`useAzureDevOpsAvailable`), github
    (`getGitHubIntegrationStatus(...).ready`), and gitlab
    (`useGitLabAvailable`) — none of these three had an "enabled" toggle
    before this feature, so their "configured" signal already meant exactly
    what this task needs.
  - `configured` for jira/linear switched from `useJiraAvailable`/
    `useLinearAvailable` (the old combined enabled+authed signal) to the
    already-existing, enabled-agnostic `useJiraAuthed`/`useLinearAuthed`.
  - `enabled` comes from the five `useXEnabled()` hooks (three new from Task
    01, two pre-existing: `useJiraEnabled`/`useLinearEnabled`).
  - `hideDisabled` comes from Task 04's `useHideDisabledIntegrationsInNav()`.
  - Confirmed `useJiraAvailable`/`useLinearAvailable` themselves were **not**
    touched — every other consumer (import bars, Kanban external-link
    buttons, task-top-bar buttons, `workspaces-group.tsx`'s settings tree)
    keeps importing and using them exactly as before; only this file's
    imports changed.
- Rewrote `use-nav-availability.test.ts`: renamed the Jira/Linear mocks to
  `jiraAuthed`/`linearAuthed` (matching the new import), added a
  controllable `useGitHubStatus` mock (previously hardcoded to
  unconfigured) so GitHub's `configured` axis is also testable, added
  enabled-hook mocks for all five keys (default `true`) and a `hideDisabled`
  mock (default `false`), and replaced the single Jira-only decoupling test
  with a `describe.each` table over all five `AvailabilityKey`s × 4 cases
  each (unconfigured-hides-regardless, disabled-stays-visible-by-default,
  disabled-hidden-when-hideDisabled-on, enabled-stays-visible-when-hideDisabled-on).
- Updated `integrations-menu.test.ts`: renamed its Jira/Linear mocks the
  same way (`useJiraAvailable`→`useJiraAuthed`, param names
  `jiraAvailable`/`linearAvailable`→`jiraConfigured`/`linearConfigured`),
  and added fixed `enabled: true` / `hideDisabled: false` stub mocks for the
  five new hooks `useNavAvailability` now transitively calls — this suite
  never exercises the decoupling itself (that's `use-nav-availability.test.ts`'s
  job), so every existing assertion keeps passing unchanged.
- Commands:
  - `cd apps && pnpm --filter @kandev/web test -- hooks/use-nav-availability components/integrations/integrations-menu` → `Test Files 2 passed (2)`, `Tests 20 passed (20)` (26 after the later table-driven expansion — see below).
  - `cd apps/web && pnpm run typecheck` → one error caught and fixed (a leftover incorrect `GitHubStatus` stub shape in the rewritten test file — corrected to match the real type's fields), then clean.
  - Follow-up (post-review): expanded the single Jira-only decoupling test
    into the five-key `describe.each` table described above. Rerun:
    `Test Files 1 passed (1)`, `Tests 26 passed (26)` for
    `hooks/use-nav-availability` alone.
- Files changed: the three files listed under "Files likely touched".
- Blockers/risks: none. Explicitly verified no other `useJiraAvailable`/
  `useLinearAvailable` consumer was touched (see spec Out-of-scope).
