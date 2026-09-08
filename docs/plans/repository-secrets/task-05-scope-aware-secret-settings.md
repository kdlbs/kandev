---
id: "05-scope-aware-secret-settings"
title: "Build scope-aware secret settings"
status: done
wave: 2
depends_on: ["01-scoped-secret-storage"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/repository-secrets.md"
---

# Task 05: Build Scope-Aware Secret Settings

## Acceptance

- General Secrets manages Global secrets only.
- Every workspace has a Secrets route and navigation leaf that manages only that workspace's
  secrets.
- Secret API/cache state is keyed by scope so Workspace data cannot appear in shared profile
  selectors.
- Agent and executor profile selectors show Global secrets only and preserve invalid/missing refs.
- Desktop and phone layouts retain complete CRUD/reveal capability, accessible labels, one scroll
  owner, and no horizontal overflow.
- New user-facing copy is localized in English and pseudo locales.

## Files likely touched

- `apps/web/lib/types/http-secrets.ts`
- `apps/web/lib/api/domains/secrets-api.ts`
- `apps/web/hooks/domains/settings/use-secrets.ts`
- `apps/web/lib/state/slices/settings/`
- `apps/web/components/settings/secrets-settings.tsx`
- `apps/web/src/settings-routes.tsx`
- `apps/web/components/app-sidebar/sections/settings/workspaces-group.tsx`
- Agent/executor profile secret selector components
- `apps/web/src/locales/en/settings.json`
- `apps/web/src/locales/en/workspaces.json`
- Matching pseudo-locale files and focused tests

## Inputs

- Task 01's list/create response and query contract.
- Mobile design contract in the spec and `mobile-parity` reference language.
- Existing `SecretsSettings`, `WorkspacesGroup`, and Settings route bootstrap.

## Dependencies

Task 01.

## TDD sequence

1. Add failing API/hook/state tests for scope-keyed loading and Global-only selectors.
2. Add failing component/route/nav tests for Global and Workspace modes.
3. Implement the reusable scope-configured surface and responsive navigation.
4. Run i18n checks and inspect pseudo-locale behavior for labels, errors, and icon actions.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
cd apps && pnpm run i18n:check && pnpm run i18n:ratchet
```

## Risks

- The existing single secret store slice is not safe for mixed scopes without a keyed redesign or
  isolated data ownership.
- Scope badges need text semantics; color alone is insufficient.
- Do not translate persisted secret names or environment identifiers.

## Output contract

Report route/navigation changes, cache ownership, selector filtering, mobile composition, i18n keys,
files changed, tests run, and residual risks.

## Result

Added reusable Global and Workspace secret settings, scope-keyed loading, workspace navigation and
routes, Global-only shared-profile selectors, missing-reference handling, and localized desktop/mobile
copy with accessible controls. Focused API, hook, component, route, and selector tests passed; web
typecheck, lint, pseudo-locale generation, i18n checks, and the ratchet passed.
