---
id: "05-dynamic-composer-reference-sources"
title: "Dynamic composer reference sources"
status: completed
wave: 2
depends_on: ["01-design-package", "03-protocol-manifest-actions"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/bitbucket-plugin.md"
---

# Task 05: Dynamic composer reference sources

## Intent

Bridge active manifest-declared plugin reference sources into the existing backend
mentions registry, preserving host canonicalization and live authorization at search
and submission.

## Owned paths

- `apps/backend/internal/mentions/service.go`
- `apps/backend/internal/mentions/types.go`
- `apps/backend/internal/mentions/service_test.go`
- `apps/backend/internal/backendapp/mentions.go`
- `apps/backend/internal/backendapp/mentions_test.go`
- `apps/backend/internal/plugins/mentions.go`
- Focused plugin bridge tests under `apps/backend/internal/plugins/`.
- `apps/web/hooks/use-entity-reference-search.test.ts`
- `apps/web/components/task/chat/use-entity-reference-composer.test.ts`
- `apps/web/components/task/chat/entity-reference-menu.test.tsx`

## Dependencies

Tasks 01 and 03.

## Acceptance

1. Plugins dynamically register/unregister only manifest-owned `(source, provider,
   kind)` descriptors; source collisions, disabled plugins, timeouts, and cancellation
   have deterministic behavior.
2. Host, not plugin, constructs canonical references. Search and message submission
   call live plugin authorization with verified workspace and `search`/`submission`
   purpose.
3. Generic composer UI renders plugin pull-request groups/icons/chips without a
   Bitbucket frontend branch.

## Verification

```sh
cd apps/backend && go test ./internal/mentions ./internal/backendapp ./internal/plugins
cd apps/web && pnpm test -- hooks/use-entity-reference-search.test.ts components/task/chat/use-entity-reference-composer.test.ts components/task/chat/entity-reference-menu.test.tsx
cd apps/web && pnpm run typecheck
```

## Risks

Search results are untrusted presentation data. Submission reauthorization is required
to prevent stale, tampered, cross-workspace, or disabled-plugin references entering
message metadata.
