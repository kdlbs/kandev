---
id: "01-card-navigation"
title: "Integration card navigation"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/requirements/clickable-integration-cards.md"
---

# Task 01: Integration card navigation

## Acceptance

- Native and plugin integration cards expose one accessible full-card link that
  uses their existing global or workspace-scoped `href`.
- The native switch and plugin action remain clickable without triggering card
  navigation.
- The focused component test covers the link route and switch isolation.

## Verification

From the repository root, after the fresh-worktree install if dependencies are
missing:

```bash
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web test -- app/settings/integrations/page.test.tsx
```

## Files likely touched

- `apps/web/components/integrations/integrations-index-page.tsx`
- `apps/web/app/settings/integrations/page.test.tsx`

## Dependencies

None.

## Parallelism

`sequential`.

## Inputs

- `docs/specs/integrations/requirements/clickable-integration-cards.md`, What and Scenarios.
- `docs/plans/clickable-integration-cards/plan.md`, Frontend and Tests.
- Existing `AppLink` behavior in `apps/web/components/routing/app-link.tsx`.

## Output contract

Report the component and unit-test files changed, the focused test command and
result, any accessibility or pointer-event risk, and the updated task and plan
status before handing off to Task 02.

## Results

Updated the shared integration card composition for native and plugin cards.
Each card now exposes one accessible overlay `AppLink`, while the native
switch and plugin action remain above that link as independent controls.

Focused verification passed:

```text
pnpm --filter @kandev/web test -- app/settings/integrations/page.test.tsx
12 tests passed
```

Targeted ESLint and the web TypeScript check also passed. The default Node heap
size was insufficient for the repository-wide typecheck, so it was rerun with
`NODE_OPTIONS=--max-old-space-size=8192`.
