---
id: "01-uuid-fallback"
title: "UUID fallback audit"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/secure-context-browser-fallbacks/spec.md"
---

# Task 01: UUID fallback audit

Route every browser-side client-only UUID through the guarded generator and
prove the reported insecure-origin workflow path.

## Acceptance

- Add Workflow, Add Step, and layout profile creation succeed when
  `crypto.randomUUID` is absent and produce unique UUID-shaped client IDs.
- Existing secure-context UUID behavior remains unchanged.
- No unguarded `crypto.randomUUID()` calls remain in application source under
  `apps/web`.

## Files likely touched

- `apps/web/app/settings/workspace/use-workflow-creation.ts`
- `apps/web/app/settings/workspace/use-workflow-creation.test.ts`
- `apps/web/components/settings/workflow-card-actions.ts`
- `apps/web/components/settings/workflow-card-actions.test.ts`
- `apps/web/lib/layout/layout-profiles.ts`
- `apps/web/lib/layout/layout-profiles.test.ts`

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run app/settings/workspace/use-workflow-creation.test.ts components/settings/workflow-card-actions.test.ts lib/layout/layout-profiles.test.ts lib/utils.test.ts
cd apps/web && pnpm run typecheck
rtk rg -n --glob '!**/*.test.*' --glob '!**/e2e/**' 'crypto\.randomUUID\s*\(' apps/web
```

## Risks

The existing fallback uses `Math.random` and is suitable only for temporary
client identities. Do not route security-sensitive identifiers through it.

## Results

- RED: the three new insecure-context tests failed with
  `TypeError: crypto.randomUUID is not a function` at each original call site.
- GREEN: `cd apps && pnpm --filter @kandev/web test -- --run app/settings/workspace/use-workflow-creation.test.ts components/settings/workflow-card-actions.test.ts lib/layout/layout-profiles.test.ts`
  passed (3 files, 49 tests).
- `cd apps/web && pnpm run typecheck` passed.
- Static audit leaves only the guarded call in `apps/web/lib/utils.ts`.
