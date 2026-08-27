---
id: "06-repository-binding-settings"
title: "Add repository binding settings"
status: done
wave: 3
depends_on: ["02-repository-secret-bindings", "05-scope-aware-secret-settings"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/repository-secrets.md"
---

# Task 06: Add Repository Binding Settings

## Acceptance

- Repository drafts, dirty tracking, save actions, and reloads preserve `secret_bindings`.
- The editor selects Global plus same-workspace secrets, shows scope, and never reveals values.
- Missing/deleted refs stay visible and repairable.
- Duplicate, invalid, and reserved keys block save with localized feedback.
- Explicit empty bindings clear; omitted transport fields do not accidentally clear.
- Phone rows stack with touch-sized actions, a single scroll owner, and no horizontal overflow.

## Files likely touched

- `apps/web/lib/types/http.ts`
- Workspace/repository API and server-action modules
- `apps/web/app/settings/workspace/workspace-repositories-client.tsx`
- `apps/web/app/settings/workspace/workspace-repositories-dirty.ts`
- `apps/web/components/settings/repository-card.tsx`
- New focused repository secret binding component/helper files
- English and pseudo `workspaces.json` / `settings.json`
- Repository card, dirty helper, validation, and client tests

## Inputs

- Task 02 repository DTO and replacement contract.
- Task 05 scope-aware secret loader and selector primitives.
- Existing repository manual-save coordinator and responsive repository card.

## Dependencies

Tasks 02 and 05.

## TDD sequence

1. Add RED tests for clone/dirty/merge and create/update/clear payloads.
2. Add RED component tests for scope options, missing refs, validation, and phone stacking semantics.
3. Implement the editor section and reuse scope-aware list data.
4. Exercise repository save failure and reload paths; run i18n checks.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
cd apps && pnpm run i18n:check && pnpm run i18n:ratchet
```

## Risks

- Temporary/new repository drafts need stable binding row IDs for React only; those IDs must not
  become backend binding identity.
- Missing selections must remain displayed rather than being filtered out by the available-options
  list.
- Preserve manual-save dirty registration across both repository fields and binding rows.

## Output contract

Report UI data flow, manual-save semantics, missing-ref behavior, mobile layout, i18n coverage, files
changed, tests run, and residual risks.

## Result

Added repository binding drafts, dirty/save/clone payload support, scope-labeled selectors, broken
reference repair, localized validation, and a stacked touch-sized mobile editor. Focused dirty,
action, component, hook, API, and route tests passed. Chromium and mobile-chrome E2E confirmed save,
reload, scope filtering, no plaintext rendering, touch sizing, and no horizontal overflow.
