---
id: dynamic-provider-options-03
title: Add model resolution hook
status: done
wave: 3
depends_on:
  - dynamic-provider-options-02
plan: docs/plans/dynamic-provider-options/plan.md
spec: docs/specs/agents/requirements/dynamic-provider-options.md
---

# Add model resolution hook

## Scope

Add the frontend API/types and a reusable model-aware resolution hook that
shares requests, replaces option snapshots atomically, reconciles drafts only
after success, and ignores stale responses.

## Acceptance conditions

1. The hook sends the selected model/context to the resolver endpoint and
   exposes loading, resolved options, sanitized error, retry, and refresh
   state.
2. A newer model selection prevents an older response from changing the
   current options; a successful snapshot never merges stale option IDs.
3. Resolver failure preserves the existing draft/persisted values and does not
   create unverified option controls.

## Files

- `apps/web/lib/api/domains/settings-api.ts`
- `apps/web/lib/types/http-agents.ts`
- `apps/web/hooks/domains/settings/use-dynamic-models.ts`
- `apps/web/hooks/domains/settings/use-dynamic-models.test.ts`
- `apps/web/lib/agent-config/` (only if a pure reconciliation helper is
  extracted; add its focused test beside it)

## Dependencies and inputs

- `dynamic-provider-options-02` HTTP contract.
- Existing `ModelConfig`, `ConfigOptionEntry`, `fetchDynamicModels`, and
  settings API error conventions.

## Output contract

Return a provider-neutral hook/helper consumable by both profile and workflow
settings. It must accept a stable agent/model/context identity and make the
complete option snapshot plus reconciliation behavior available without
duplicating fetch logic in either component.

## Checks

```bash
cd apps && pnpm --filter @kandev/web exec vitest run hooks/domains/settings/use-dynamic-models.test.ts --reporter=dot
```

Results: `cd apps && pnpm --filter @kandev/web exec vitest run hooks/domains/settings/use-dynamic-models.test.ts components/settings/profile-model-config.test.ts` — passed.
