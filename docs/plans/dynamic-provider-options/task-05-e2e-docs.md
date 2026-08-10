---
id: dynamic-provider-options-05
title: Verify dynamic option flows
status: done
wave: 5
depends_on:
  - dynamic-provider-options-04
plan: docs/plans/dynamic-provider-options/plan.md
spec: docs/specs/agents/dynamic-provider-options.md
---

# Verify dynamic option flows

## Scope

Make the mock ACP provider expose model-dependent options, update desktop and
mobile Playwright coverage, and keep public workflow documentation aligned.

## Acceptance conditions

1. The mock provider's dependent option appears only after the selected model is
   resolved, and desktop workflow/profile tests save and reload that option.
2. Mobile workflow coverage configures the resolved option through touch,
   retains the existing one-column layout, and reports no horizontal overflow.
3. Public documentation and its validators describe the dynamic capability
   behavior without adding provider-specific UI assumptions.

## Files

- `apps/backend/cmd/mock-agent/main.go`
- `apps/web/e2e/tests/settings/agent-profile-acp.spec.ts`
- `apps/web/e2e/tests/workflow/workflow-settings.spec.ts`
- `apps/web/e2e/tests/workflow/mobile-workflow-settings.spec.ts`
- `apps/web/e2e/pages/workflow-settings-page.ts` (only if the page object needs
  a dynamic-option helper)
- `docs/public/tasks-and-workflows.md`
- `docs/public/agents-and-profiles.md`

## Dependencies and inputs

- `dynamic-provider-options-04` settings integration.
- Existing mock-agent fixture contract and managed Playwright projects.
- Public docs validator and documentation coverage inventory.

## Output contract

End-to-end evidence must exercise a model-dependent provider response rather
than a static capability option. Keep the desktop and mobile specs in their
existing project paths.

## Checks

```bash
cd apps/web && pnpm e2e:run --project chromium tests/settings/agent-profile-acp.spec.ts tests/workflow/workflow-settings.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/workflow/mobile-workflow-settings.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

Results: managed desktop and mobile profile/workflow model-option flows —
passed. `node --test scripts/validate-public-docs.test.mjs` and
`node scripts/validate-public-docs.mjs` — passed.
