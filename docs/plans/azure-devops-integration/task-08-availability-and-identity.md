---
id: "08-availability-and-identity"
title: "Immediate availability and integration identity"
status: done
wave: 5
depends_on: []
plan: "plan.md"
spec: "../../specs/azure-devops-integration/spec.md"
---

# Task 08: Immediate Availability And Integration Identity

## Acceptance

- Successful configure, copy, replace, and delete operations invalidate shared integration availability immediately for every integration.
- The 90-second health poll remains active as recovery and does not create duplicate or stale in-flight updates.
- Configured integrations show an Enabled chip in the expanded workspace settings navigation.
- Azure DevOps uses one repository-local official product-mark component across app and settings navigation.

## Verification

- Focused availability-hook and sidebar component tests.
- Desktop and mobile Playwright coverage for save/delete propagation and the Enabled chip.
- Web typecheck and lint.

## Files Likely Touched

- `apps/web/hooks/domains/integrations/use-integration-availability.ts`
- Integration configuration API modules and settings components.
- `apps/web/components/app-sidebar/sections/integrations-section.tsx`
- `apps/web/components/app-sidebar/sections/settings/workspaces-group.tsx`
- A shared Azure DevOps icon component or asset.

## Output Contract

Report invalidation semantics, mutation coverage, icon source/license, RED/GREEN commands, mobile verification, and update this task plus its plan checkbox.
