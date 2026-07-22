---
id: "10-frontend-search-client"
title: "Frontend reference search client"
status: completed
wave: 4
depends_on: ["09-backend-composition"]
plan: "plan.md"
spec: "../../specs/ui/entity-reference-composer.md"
---

# Task 10: Frontend Reference Search Client

## Acceptance

- Typed API client encodes workspace/query/limit/current-task exclusion and preserves normalized provider groups/statuses without closed native-provider unions.
- Hook debounces non-empty queries, aborts or generation-guards stale work, and resets cleanly on workspace/session changes.
- Partial groups remain usable while aggregate failures expose a retryable state without mutating the draft.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- lib/api/domains/mentions-api.test.ts hooks/use-entity-reference-search.test.ts
```

## Files likely touched

- `apps/web/lib/types/entity-reference.ts`
- `apps/web/lib/api/domains/mentions-api.ts`
- `apps/web/lib/api/domains/mentions-api.test.ts`
- `apps/web/hooks/use-entity-reference-search.ts`
- `apps/web/hooks/use-entity-reference-search.test.ts`

## Dependencies

Task 09 endpoint contract.

## Inputs

Spec API/state/failure sections; command-panel abort/debounce search pattern.

## Output contract

Report request lifecycle and types, files changed, exact tests, blockers, risks, and mark task/plan done.
