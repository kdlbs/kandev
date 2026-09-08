---
id: "02-task-chrome-status-controls"
title: "Repair task chrome status and controls"
status: complete
wave: 2
depends_on: ["01-refresh-status-credentials"]
plan: "plan.md"
spec: "../../specs/kubernetes-executor/spec.md"
---

# Task 02: Repair task chrome status and controls

## Acceptance

- Sidebar and Kanban Kubernetes hovers use the exact executor/task/session Pod
  status source and refetch on reopen, so a prior Unauthorized result can clear.
- The task Pod disclosure keeps its useful summary but replaces the prominent
  footer with compact accessible Refresh and Profile settings icons.
- Refresh visibly joins the in-flight read, and the settings icon opens the
  exact executor profile root on desktop and touch layouts.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- components/task/remote-cloud-tooltip.test.tsx components/task/executor-settings-button.test.tsx hooks/domains/session/use-task-environment.test.tsx lib/sidebar/apply-view.test.ts && pnpm --filter @kandev/web lint && cd web && pnpm run typecheck && cd ../.. && git diff --check
```

## Files likely touched

- `apps/web/components/task/remote-cloud-tooltip.tsx`
- `apps/web/components/task/remote-cloud-tooltip.test.tsx`
- `apps/web/components/task/task-item.tsx`
- `apps/web/components/task/task-switcher-types.ts`
- `apps/web/components/task/task-switcher-row.tsx`
- `apps/web/components/task/task-session-sidebar-item.ts`
- `apps/web/components/kanban-card-content.tsx`
- `apps/web/components/task/executor-environment-disclosure.tsx`
- `apps/web/components/task/executor-environment-info.tsx`
- `apps/web/components/task/executor-settings-button.tsx`
- their focused tests
- `apps/web/lib/sidebar/apply-view.ts`
- `apps/web/lib/settings/executor-settings-routes.ts`
- affected locale catalogs under `apps/web/src/locales/`

## Dependencies

Task 01.

## Parallelism

`sequential`. These files are already part of the uncommitted first UX repair.

## Inputs

- Spec: sidebar/Kanban consistency, compact task controls, profile-root routing,
  refresh single-flight, and mobile parity scenarios.
- Plan: Sidebar/Kanban and Compact task disclosure sections.
- Existing patterns: `getKubernetesTaskSession`,
  `executorProfileSettingsPath`, `useTaskEnvironment.refresh`, and
  `getExecutorStatusIcon`.

## Output contract

Report RED/GREEN evidence, responsive hit-target behavior, route destinations,
files changed, blockers/risks, and synchronize this task plus `plan.md`.

## Results

RED showed Kubernetes list/card hovers still dispatched the generic WebSocket
status request, cached the first response across reopen, and dropped the
executor identity in sidebar projection. The task disclosure also kept the
two-button footer and linked to the executor connection route. GREEN now uses
the exact Kubernetes session endpoint, refetches per reopen while deduplicating
only an in-flight request, discards obsolete-scope responses, and retains the
WebSocket path for other remote executors.

The Pod summary now owns quiet Refresh and Profile settings icons in its header.
They have 40 px desktop and 44 px touch targets, translated labels/tooltips,
`aria-busy` plus a spinning refresh glyph, and the settings link resolves to
the exact profile root (with the executor route only as a malformed legacy
fallback). The desktop popover is reduced from 380 px to 348 px.

Verification:

- Focused task-chrome tests passed: 3 files, 27 tests.
- `hooks/domains/session/use-task-environment.test.tsx` and the environment
  summary tests passed as part of the focused runs.
- `pnpm run typecheck` passed.
- Focused ESLint for all changed task/sidebar/Kanban files passed with no
  findings.
