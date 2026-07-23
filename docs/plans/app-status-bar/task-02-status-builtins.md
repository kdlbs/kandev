---
id: app-status-bar-02
title: Status built-ins and metrics gating
status: done
wave: 1
depends_on: []
plan: docs/plans/app-status-bar/plan.md
---

# Status built-ins and metrics gating

## Inputs

[Spec: What](../../specs/ui/app-status-bar.md#what); current `TopbarMetrics`; canonical connection types; existing system-metrics subscription hook.

## Files

- `apps/web/components/app-status-bar/connection-status-item.tsx`
- `apps/web/components/app-status-bar/connection-status-item.test.tsx`
- `apps/web/components/system-metrics/status-surface-metrics.tsx`
- `apps/web/components/system-metrics/status-surface-metrics.test.tsx`
- `apps/web/components/system-metrics/topbar-metrics.tsx`
- `apps/web/components/system-metrics/topbar-metrics.test.tsx`

## Acceptance

1. Connection item maps every canonical connection state and error to semantic visible, tooltip, and screen-reader output.
2. Status metrics preserve current source selection, labels, formatting, thresholds, and details for compact/full bar and drawer presentations.
3. Subscription is false when preference is disabled; on phone it is false until drawer opens. Primitives create no independent duplicate subscription.

## Verification

```sh
cd apps && pnpm --filter @kandev/web test -- components/app-status-bar/connection-status-item.test.tsx components/system-metrics
```

## Output contract

Report changed files, preference/presentation subscription matrix, tests, blockers, and task status.
