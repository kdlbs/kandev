---
id: "09-integrations-automations-system"
title: "Integrations automations system"
status: done
wave: 3
depends_on: ["03-query-options-taxonomy", "04-query-bridge-audit"]
plan: "plan.md"
spec: "../../specs/ui/tanstack-query-server-state.md"
---

# Task 09: Integrations Automations System

## Acceptance

- GitHub, GitLab, Jira, Linear, Slack/Sentry where present, automations, and
  system settings/status data read from TanStack Query.
- Auth health poller cadence remains 90 seconds where currently expected.
- Integration/import/watch flows preserve optimistic or post-mutation cache
  updates and existing toasts.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- apps/web/components/github apps/web/components/gitlab apps/web/components/jira apps/web/components/linear apps/web/components/automations apps/web/components/settings/system apps/web/lib/query`
- `cd apps/web && pnpm e2e:docker tests/github tests/integrations tests/system/status-page.spec.ts tests/system/database-page.spec.ts`
- `cd apps/web && pnpm e2e:docker --project mobile-chrome tests/integrations/mobile-linear-watcher-profile.spec.ts tests/github/mobile-github-sidebar.spec.ts`

## Files Likely Touched

- `apps/web/components/github/**`
- `apps/web/components/gitlab/**`
- `apps/web/components/jira/**`
- `apps/web/components/linear/**`
- `apps/web/components/automations/**`
- `apps/web/components/settings/system/**`
- `apps/web/lib/query/query-options/{github,gitlab,jira,linear,integrations,automations,system}.ts`
- `apps/web/lib/query/bridge/{github,gitlab,jira,linear,integrations,automations}.ts`
- old handlers under `apps/web/lib/ws/handlers/{github,secrets,system-events}.ts`
- relevant state slices under `apps/web/lib/state/slices/*`

## Dependencies

- Tasks 03 and 04.

## Inputs

- Old PR query option/bridge files for integrations and automations.
- Current integration specs and `apps/backend/internal/integrations/AGENTS.md`
  if integration contracts are touched.

## Output Contract

Update this task to `done`, list migrated domains, and identify any integration
state intentionally left local-only.

## Completed

- Added query keys/options and query-backed hooks/components for:
  - GitHub status, workspace PRs, task PRs, task CI options, PR watches,
    review watches, issue watches, action presets, and rate-limit bridge
    updates.
  - GitLab status, stats, workspace MRs, task MRs, review watches, issue
    watches, action presets, and projects.
  - Jira config availability, settings config, issue watches, and app-page
    configured-state checks.
  - Linear config availability, settings config, issue watches, app-page
    configured-state checks, and watcher agent profile flows.
  - Slack and Sentry config availability/settings config; Sentry issue watches.
  - Automations list/runs and mutation cache updates.
  - System health, jobs, metrics, runtime flags, backups, logs, database, disk,
    updates, secrets, and sprites status/instances where this wave touched
    settings/system surfaces.
- Preserved the 90s auth-health poll cadence through the shared integration
  availability hook.
- Kept local-only state local:
  - form drafts and inline test-connection results,
  - per-browser integration enabled toggles backed by localStorage,
  - control-plane mutations/triggers/reset-preview calls,
  - high-volume stream/readiness state that is intentionally not server cache.
- Tightened strict E2E WS accounting after the Docker gate found a reload race:
  the helper now distinguishes a missing browser hook from an installed hook
  with no post-reload frames, and the UI-state reset test waits for the app to
  render after reload.

## Verification Completed

- `cd apps/web && rtk pnpm typecheck` passed.
- `cd apps && rtk pnpm --filter @kandev/web test -- hooks/domains/settings hooks/domains/system hooks/domains/github hooks/domains/gitlab hooks/domains/jira hooks/domains/linear hooks/domains/sentry hooks/domains/slack hooks/domains/integrations components/github components/gitlab components/jira components/linear components/slack components/sentry components/automations components/settings/system lib/query`
  passed 55 files / 487 tests.
- `cd apps && rtk pnpm --filter @kandev/web test -- lib/ws/ws-account-e2e-helper.test.ts lib/ws/ws-account.test.ts lib/ws/client.test.ts e2e/helpers`
  passed 3 files / 15 tests.
- `cd apps/web && rtk pnpm exec eslint --max-warnings 0 app/jira/jira-page-client.tsx app/linear/linear-page-client.tsx`
  passed.
- `cd apps/web && rtk pnpm e2e:docker tests/system/status-page.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 5 desktop Docker tests after the WS-accounting reload fix.
- `cd apps/web && rtk pnpm e2e:docker tests/system/ws-event-accounting.spec.ts tests/integrations/jira-settings.spec.ts tests/integrations/linear-settings.spec.ts tests/integrations/sentry-settings.spec.ts tests/integrations/github-watch-reset.spec.ts tests/integrations/jira-import.spec.ts tests/integrations/linear-import.spec.ts tests/github/pr-list-task-indicator.spec.ts tests/github/github-scope-bar.spec.ts tests/pr/ci-automation-options.spec.ts tests/automations-settings.spec.ts tests/system/status-page.spec.ts tests/system/database-page.spec.ts tests/system/disk-usage.spec.ts tests/system/backups-page.spec.ts tests/system/updates-page.spec.ts tests/system/logs-page.spec.ts`
  passed 60 desktop Docker tests / 1 skipped.
- `cd apps/web && rtk pnpm e2e:docker --project mobile-chrome tests/integrations/mobile-linear-watcher-profile.spec.ts tests/github/mobile-github-sidebar.spec.ts tests/settings/mobile-general-settings.spec.ts tests/mobile-automations-scroll.spec.ts`
  passed 4 mobile Docker tests.
