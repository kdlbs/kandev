---
id: "07-shared-agent-initialization"
title: "Share agent discovery initialization"
status: complete
wave: 7
depends_on:
  - "04-progressive-task-navigation"
plan: "plan.md"
requirements:
  - REQ-UI-SESSION-REFRESH-EFFICIENCY-006
acceptance_criteria:
  - AC-UI-SESSION-REFRESH-EFFICIENCY-006.1
system_design:
  - ../../specs/ui/system-design/session-refresh-efficiency.md
---

# Task 07: Share agent discovery initialization

## Summary

Eliminate simultaneous agent-list reads and repeated publication from settings consumers and route enrichment.

## In scope

- Share listAgentsUntilSettled and its retry chain per store/auth scope, including route enrichment.
- Preserve discovery reconciliation and profile-version invalidation with one queued follow-up. Do not replace a newer profile edit with the old snapshot.
- Use enabled consumers only; release abandoned owners and retain current empty-list/error behavior.

## Out of scope

No backend API, deployment, dependency replacement, or general cache migration.

## Acceptance

Add `three mounts share one discovery sequence`, `route enrichment joins settings read`, and `profile edit queues fresh read` to settings/resource tests. Extend the task-switch browser scenario to count agent reads and inspect the current model/profile.

## Verification

Start with the named failing behavioral tests; record RED/GREEN evidence.
New test paths below are created by this order. Install workspace dependencies
once before the first package check if this checkout lacks them.

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/settings/use-settings-data.test.tsx lib/ssr/session-page-state.test.ts hooks/domains/settings/agent-list-resource.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/task/task-switch-efficiency.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-task-switch-efficiency.spec.ts)
```

## Files likely touched

- `apps/web/hooks/domains/settings/use-settings-data.ts`
- `apps/web/hooks/domains/settings/use-settings-data.test.tsx`
- `apps/web/lib/ssr/session-page-state.ts`
- `apps/web/lib/ssr/session-page-state.test.ts`
- `apps/web/hooks/domains/settings/agent-list-resource.ts (new)`

## Dependencies

Requires 04-progressive-task-navigation. Run sequentially in this package; Task 04 introduces the shared browser scenarios.

## Risks

A response that is empty while discovery settles is not permanently authoritative. Preserve existing bounded retry behavior.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/session-refresh-efficiency.md)
- [System design](../../specs/ui/system-design/session-refresh-efficiency.md)
- [Trace and source evidence](trace-2026-09-29-task-switch.md)

## Results

The store- and authentication-scoped `agent-list-resource` now shares the
settling read and retry chain between settings consumers and route enrichment.
It preserves the profile-version guard and queues a fresh read when a real
profile edit arrives during an outstanding request. Route enrichment without
an app store keeps the existing direct SSR path.

Resource and settings tests pass for three concurrent consumers, route joining,
empty-list retries, and a profile edit during a deferred response. Desktop and
phone task-switch E2Es each observe one outstanding list read while held and
the current model after release. The existing available-agent freshness pass
may trigger its own later follow-up read, so the browser assertion checks the
shared in-flight request rather than imposing a one-request total.

Follow-up code-review remediation passes `enabled` into the resource hook. A
disabled settings consumer uses a no-op subscription and does not create the
agent-list resource. When the last hook disables, profile-version changes do
not start another read. The targeted settings regression covers disabled,
enabled shared-read, and disabled-again states.
