---
id: "06-workflow-session-options-toggle"
title: "Refine the workflow session-options toggle and family choices"
status: done
wave: 5
depends_on: ["04-editor-and-carry-analysis"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/workflow-session-settings.md"
---

# Task 06: Refine the workflow session-options toggle and family choices

## Acceptance

- The step header shows an `Override original session options` checkbox and an info hover with the Sol-to-Luna example beside the fixed profile selector.
- The conditional editor is hidden when unchecked and appears below WIP controls when checked; fixed profile selection disables the checkbox.
- Conditional family choices include only families represented by available agent profiles, while persisted unavailable rules remain readable.
- Desktop and mobile workflow editor coverage verifies the toggle, visibility, touch sizing, and no horizontal overflow.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/settings/workflow-session-config-editor.test.tsx
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm run i18n:ratchet
```

## Files likely touched

- `apps/web/components/settings/workflow-pipeline-editor-panels.tsx`
- `apps/web/components/settings/workflow-session-config-editor.tsx`
- `apps/web/components/settings/workflow-session-config-shared.ts`
- `apps/web/components/settings/workflow-session-config-editor.test.tsx`
- `apps/web/e2e/tests/workflow/workflow-settings.spec.ts`
- `apps/web/e2e/tests/workflow/mobile-workflow-settings.spec.ts`
- `apps/web/src/locales/en/settings.json`
- `apps/web/src/locales/pseudo/settings.json`

## Dependencies

Task 04. Sequential.

## Inputs

The approved workflow-session-settings spec, the existing editor/carry-analysis
implementation, and the mobile workflow settings scenario.

## Results

- `cd apps && pnpm --filter @kandev/web test -- --run components/settings/workflow-session-config-editor.test.tsx` — 3 tests passed.
- `cd apps && pnpm --filter @kandev/web test -- --run lib/workflows components/settings` — 69 files and 360 tests passed.
- `cd apps/web && pnpm run typecheck` — passed.
- `cd apps && pnpm --filter @kandev/web lint` — passed with zero warnings.
- `cd apps/web && pnpm run i18n:check` — passed with zero orphan keys and pseudo-locale in sync.
- `cd apps/web && pnpm run i18n:ratchet` — passed; all added and modified files clean.
- `cd apps && make build-web` — passed.
- `cd apps/web && pnpm e2e:run --project chromium tests/workflow/workflow-settings.spec.ts` — 14 tests passed.
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/workflow/mobile-workflow-settings.spec.ts` — 3 tests passed.
- `node --test scripts/validate-public-docs.test.mjs` — 58 tests passed.
- `node scripts/validate-public-docs.mjs` — 41 published pages validated.
- `git diff --check` — passed.
- PR #2137 conflict fixup — merged `origin/main` at `332353f647cf8ae157db893f529dfc4cb3516ba2`; resolved `apps/backend/internal/task/models/models.go` by retaining both metadata-key groups; merge commit `72bd08c0d116d513e1c63ba274ec94c17c49bbb1`.
- Post-push PR snapshot — mergeable, zero failed checks, 15 queued/pending checks, and zero unresolved review threads.
