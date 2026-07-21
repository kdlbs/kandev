---
id: "09-azure-presets"
title: "Azure presets and saved views"
status: done
wave: 5
depends_on: []
plan: "plan.md"
spec: "../../specs/azure-devops-integration/spec.md"
---

# Task 09: Azure Presets And Saved Views

## Acceptance

- Work items and pull requests expose useful named presets in the primary toolbar.
- Workspace-scoped custom views persist through backend restart and reject malformed records.
- Project/repository scope and editable provider query remain visible without a permanent WIQL textarea.
- Raw WIQL is available under Advanced and all controls remain reachable on mobile.

## Verification

- Go persistence/controller tests for saved views.
- Component tests for preset-to-query mapping, saved-view lifecycle, and Advanced WIQL.
- Desktop and mobile Azure browse Playwright coverage.

## Files Likely Touched

- `apps/backend/internal/azuredevops/` config/store/controller files.
- `apps/web/app/azure-devops/azure-devops-page-client.tsx`
- `apps/web/components/azure-devops/`
- `apps/web/hooks/domains/azure-devops/`

## Output Contract

Report preset definitions, persistence contract, RED/GREEN commands, responsive verification, and update this task plus its plan checkbox.
