---
id: "08-shared-integration-health"
title: "Share integration availability probes"
status: complete
wave: 8
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SESSION-REFRESH-EFFICIENCY-006
acceptance_criteria:
  - AC-UI-SESSION-REFRESH-EFFICIENCY-006.2
  - AC-UI-SESSION-REFRESH-EFFICIENCY-006.3
system_design:
  - ../../specs/ui/system-design/session-refresh-efficiency.md
---

# Task 08: Share integration availability probes

## Summary

Coalesce same-scope integration availability reads and polling without changing configured/healthy/disabled semantics.

## In scope

- Introduce explicit provider/workspace/auth identity for shared raw config health. Keep global and workspace requests separate.
- Represent a successful 204 as loaded-unconfigured. Share a timer at the fastest live cadence, invalidate on credential changes, and tear down on last release.
- Migrate availability wrappers together and retain their disabled gating and error behavior.

## Out of scope

No backend API, deployment, dependency replacement, or general cache migration.

## Acceptance

Add `concurrent null results settle once`, `disabled consumers do not probe`, `credential invalidation refreshes`, `last release clears timer`, and `workspace change rejects old health` to availability/resource tests. Browser tests mount both task consumers, change scope, and check integration actions.

## Verification

Start with the named failing behavioral tests; record RED/GREEN evidence.
New test paths below are created by this order. Install workspace dependencies
once before the first package check if this checkout lacks them.

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/integrations/use-integration-availability.test.ts hooks/domains/integrations/integration-health-resource.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/task/task-switch-efficiency.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-task-switch-efficiency.spec.ts)
```

## Files likely touched

- `apps/web/hooks/domains/integrations/use-integration-availability.ts`
- `apps/web/hooks/domains/integrations/use-integration-availability.test.ts`
- `apps/web/hooks/domains/integrations/integration-health-resource.ts (new)`
- `apps/web/hooks/domains/jira/use-jira-availability.ts`
- `apps/web/hooks/domains/linear/use-linear-availability.ts`
- `apps/web/hooks/domains/azure-devops/use-azure-devops-availability.ts`
- `apps/web/hooks/domains/sentry/use-sentry-availability.ts`
- `apps/web/lib/integrations/integration-availability-events.ts`

## Dependencies

No functional dependency on session ETags or the environment polling work. Run sequentially in this package; Task 04 introduces the shared browser scenarios.

## Risks

Do not cache credential failures as permanently unconfigured or join unscoped and workspace-specific requests.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/session-refresh-efficiency.md)
- [System design](../../specs/ui/system-design/session-refresh-efficiency.md)
- [Trace and source evidence](trace-2026-09-29-task-switch.md)

## Results

The shared `integration-health-resource` now keys raw health by backend,
authentication, workspace, and provider. It treats a successful no-config
response as settled, shares the fastest active consumer cadence, refreshes on
credential invalidation, and clears timers after the last consumer releases.
Jira, Linear, Azure DevOps, and Sentry availability readers use the resource;
disabled integrations remain gated.

Focused resource and provider tests pass for concurrent null results, disabled
consumers, invalidation, timer cleanup, and stale workspace responses. Desktop
and phone task-switch E2Es each hold Jira, Linear, and Sentry no-config reads
across navigation and confirm actions remain unavailable after the shared
responses settle.
