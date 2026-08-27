---
id: "04-frontend-plugin-registry"
title: "Frontend plugin registry contracts"
status: completed
wave: 1
depends_on: ["01-design-package"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/bitbucket-plugin.md"
---

# Task 04: Frontend plugin registry contracts

## Intent

Add generic revocable action, repository-provider, task-action, review-provider,
integration-settings, Drawer, and responsive contracts to the frontend plugin host.
Do not rewrite task-create or review screens in this task.

## Owned paths

- `apps/web/lib/plugins/types.ts`
- `apps/web/lib/plugins/registry.ts`
- `apps/web/lib/plugins/host-api.ts`
- `apps/web/lib/plugins/host.ts`
- `apps/web/lib/plugins/active-plugin.ts`
- `apps/web/src/boot-payload.ts`
- `apps/web/lib/plugins/registry.test.ts`
- `apps/web/lib/plugins/host-api.test.ts`
- `apps/web/lib/plugins/host.test.ts`
- `apps/web/lib/plugins/active-plugin.test.ts`
- `apps/web/src/boot-payload.test.ts`
- Built-in registration adapters directly required by those registry files.
- Native settings integration index, route resolver, workspace navigation, icons, and tests.

## Dependencies

Task 01.

## Acceptance

1. `host.api.invokeAction(...)` targets declared authenticated actions; repository,
   task-action, and review registrations are owner-scoped and revoke on disable.
2. Duplicate provider ownership fails deterministically and unload aborts in-flight
   work; review items use external-store sources rather than plugin React hooks.
3. `ActivePlugin.repositoryProviderIds?: string[]` parses JSON
   `repositoryProviderIds`; loader records present manifest declarations before
   `initialize`, uses them as repository/review provider ownership allowlist, and
   preserves absent-field compatibility for older payloads.
4. Host exports Drawer family and supported responsive breakpoint behavior needed for
   native mobile plugin UI.
5. `registerIntegrationSettings({ id, label, description, icon?, Component })`
   contributes lifecycle-safe native global/workspace settings pages, index cards, and
   workspace navigation without provider-specific host rendering.

## Verification

```sh
cd apps/web && pnpm test -- lib/plugins/registry.test.ts lib/plugins/host-api.test.ts lib/plugins/host.test.ts lib/plugins/active-plugin.test.ts src/boot-payload.test.ts
cd apps/web && pnpm run typecheck
```

## Risks

Registry contracts become public plugin API. Avoid closed provider unions and avoid
making a Bitbucket registration special.
