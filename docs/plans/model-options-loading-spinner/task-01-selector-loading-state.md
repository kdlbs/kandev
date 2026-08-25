---
id: "01-selector-loading-state"
title: "Show selector loading state"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/dynamic-provider-options.md"
---

# Task 01: Show Selector Loading State

## Acceptance conditions

1. A model-aware profile selector stays open after a model selection starts
   option resolution.
2. The area below the model list shows a localized spinner and hides stale
   option controls while resolution is pending.
3. The selected model row replaces its check icon with a spinner while
   resolution is pending and restores the check icon afterward.
4. Resolved controls replace the spinner without changing the existing status,
   error, or retry state below the profile field.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web exec vitest run components/model-config-selector.test.tsx components/settings/profile-form-fields.test.tsx --reporter=dot && cd web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet
```

## Files likely touched

- `apps/web/components/model-config-selector.tsx`
- `apps/web/components/model-config-selector.test.tsx`
- `apps/web/components/settings/profile-model-fields.tsx`
- `apps/web/components/settings/profile-form-fields.tsx`
- `apps/web/components/settings/profile-form-fields.test.tsx`

## Dependencies

None.

## Parallelism

Sequential. This task changes the shared selector contract and the profile
caller that uses it.

## Inputs

- The `What`, `Agent profile shows model-option progress`, and `Mobile
settings` sections in the feature spec.
- The `Frontend` and `Tests` sections in `plan.md`.
- `ModelConfigResolutionStatus` for the existing loading copy and status
  behavior.
- `useProfileModelCapabilities` for the authoritative `configIsLoading` state.

## Output contract

Report the selector props, loading-row behavior, changed files, tests, risks,
and blockers. Update this task and `plan.md` with the exact results.

## Risks

- The selector must decide to stay open in the same event that changes the
  model. It cannot wait for the next loading-state render.
- The loading row must not expose controls from the prior model snapshot.

## Results

Implemented the shared selector loading row and profile-form wiring. The
selector now stays open for dynamic profile models, hides stale dependent
controls during resolution, shows a spinner in the selected model row, and
restores both the check icon and resolved controls afterward. Existing callers
retain their previous close behavior by default.

The exact verification command passed:

- Focused Vitest: 2 files, 32 tests passed.
- `pnpm run typecheck` passed.
- `pnpm run i18n:check` passed. The command reported only existing orphan
  catalog entries.
- `pnpm run i18n:ratchet` passed with no new violations.

Review fixup added regression coverage for the self-contained loading close
guard, the single accessible status announcement, and stale-option clearing
after rejected or failed resolution responses. The targeted suite now passes
46 tests.
