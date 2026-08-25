---
id: "02-frontend-selector-gate"
title: "Frontend: don't require persisted-only unadvertised keys in selector gate"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/session-config-cross-agent-reconcile.md"
parallelism: sequential
---

# Task 02: Frontend model-selector render gate

Make the chat model/mode selector render for a resumed session even when the
session's persisted metadata records option keys the current agent does not
advertise, so `configHydrated` no longer stays `false` forever.

## Context / root cause

`hasCompleteDynamicConfig`
(`apps/web/components/task/model-selector.tsx:109-133`) requires every key from
`requiredConfigKeys` (lines 90-107), which includes keys read from
`session.metadata.runtime_config(_overrides).config_options`. When the current
agent never advertises a persisted key (e.g. `effort`, `thinking`), the
`required.every(...)` check never passes, `configHydrated=false`, and the render
gate at line 533 hides the selector permanently. Observed logs:
`requiredKeys=[context, reasoning, effort, fast, mode, model, thinking]`,
`rawConfigOptionIds=[mode, model, context, reasoning, fast]`, `willHide=true`.

## Steps (TDD)

1. Set `status: in_progress`.
2. Write/extend the regression test FIRST and confirm it fails: a session with a
   persisted-only, unadvertised key plus a settled agent catalog currently gives
   `hasCompleteDynamicConfig === false`.
3. Change the gate so a required key that is sourced ONLY from persisted
   `runtime_config` / `runtime_config_overrides` (i.e. not from the agent profile
   snapshot / matched profile) is treated as satisfied when the agent catalog is
   settled (`sessionModelsData.configOptionsSettled === true` and/or non-empty
   `configOptions`) and does not advertise that key. Preserve the existing
   `AGENT_CONFIG_KEY` legacy exception and the `MODEL_CONFIG_KEY` flat-model-list
   exception (lines 124-131). Keys required by the profile snapshot/matched
   profile remain blocking.
   - Suggested shape: split `requiredConfigKeys` conceptually into
     profile-required keys (snapshot + matched profile) and persisted-only keys,
     then in `hasCompleteDynamicConfig` require all profile-required keys as
     today but only require a persisted-only key when the catalog is unsettled.
     Keep the change minimal and within this file.
4. Run the targeted test; confirm it passes. Run typecheck and lint.
5. Reconcile files-touched, set `status: done`, update `plan.md` checkbox and
   `## Verification Results`.

## Acceptance

- Given a settled agent catalog advertising `[mode, model, context, reasoning,
  fast]` and persisted-only keys `effort`, `thinking`, then
  `hasCompleteDynamicConfig` returns `true` (selector renders).
- Given the agent profile snapshot requires a key the agent has NOT advertised
  and the catalog is settled, `hasCompleteDynamicConfig` still returns `false`.
- Given an unsettled catalog, behavior is unchanged from today.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- components/task/model-selector.test.ts
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
```

If the test file is `.test.tsx` or lives elsewhere, substitute the correct path.

## Files likely touched

- `apps/web/components/task/model-selector.tsx`
- The existing model-selector unit test
  (`apps/web/components/task/model-selector.test.ts(x)`); create beside the
  component if none exists, following repo frontend test conventions.

## Dependencies

None.

## Parallelism

`parallel-safe` with task-01 (disjoint TS vs Go files). Default sequential.

## Inputs

- Spec: "What" (selector rendering bullet), "Desired behavior" (Frontend),
  second and third scenarios.
- Plan: Frontend section.
- Existing gate/exception logic: `model-selector.tsx:56-133`.

## Output contract

Summary, files changed, exact test/typecheck/lint commands + results, blockers,
risks, and task/plan status update in the same conversation.

## Results

Done.

- Change (`model-selector.tsx`): split `requiredConfigKeys` into two helpers,
  `profileRequiredConfigKeys` (snapshot + matched profile) and
  `persistedRuntimeConfigKeys` (runtime_config/_overrides). `hasCompleteDynamicConfig`
  now treats a required key as satisfied when it is a persisted-runtime-only key
  the current agent does not advertise once the catalog is settled
  (`isPersistedOnlyStaleKey`). The `AGENT_CONFIG_KEY` is explicitly excluded from
  the new exception so the existing legacy-agent semantics keep owning that key;
  the flat-model-list and legacy-agent exceptions are otherwise unchanged.
- Test (TDD): added `describe("cross-agent persisted config reconciliation")` in
  `model-selector.test.ts` (3 cases). Red first: the persisted-only-unadvertised
  settled-catalog case returned `false` (`expected false to be true`). Green
  after fix. Extracted `modelConfigOption` / `selectOption` / `catalogEntry`
  helpers to keep the block under the 100-line function limit.
- Commands:
  - `pnpm vitest run components/task/model-selector.test.ts` → 26 passed
  - `pnpm run typecheck` → clean
  - `pnpm --filter @kandev/web lint` → 0 warnings
  - `pnpm run i18n:ratchet` → clean
- External side-effect boundaries: None (pure function unit test).
