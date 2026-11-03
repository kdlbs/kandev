---
id: "06-selector-integration"
title: "Integrate configuration ownership"
status: completed
wave: 6
depends_on:
  - "05-shared-catalog-picker"
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-001
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-001.2
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-001.3
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.3
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003.2
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004.1
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004.2
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004.3
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004.4
system_design:
  - ../../specs/agents/system-design/grouped-model-catalog.md
---

# Task 06: Integrate configuration ownership

## Summary

Connect the shared picker to profile drafts, workflow overrides, and live sessions.
Keep existing configuration authority, confirmation, persistence, restore, and turn attribution intact.

## In scope

- Full context propagation and model-aware option resolution for each existing consumer.
- Complete draft Apply, current Save behavior, and confirmed live mutation sequencing.
- Stale response rejection, busy-turn rules, partial failure, and retry from confirmed state.
- Catalog-aware summaries, current snapshot hydration, reset/resume, and historical attribution tests.

## Out of scope

- A new atomic ACP transaction, another state store, rewriting profile defaults from a live change, or conversation reset during Apply.

## Acceptance

1. Profile/workflow Apply changes only the current draft, and existing Save persists exact advertised model and option values.
2. Live Apply reports actual provider confirmations and partial failure without changing conversation identity or overwriting a newer mutation.
3. Reload, restore, baseline summaries, and historical turns retain their existing authority with both grouped and older flat data.

## ASCII UI preview

Use [UI-GROUP-02/03/04](plan.md#ascii-ui-preview).

```text
Picker:  < Models                   Fusion
         Lead / Sidekick / returned options
         [ Use configuration ]

Profile/workflow: Apply updates draft -> existing Save
Live session:    Apply confirms model -> confirms options
Partial failure: Model changed. An option was rejected.
                 [ Retry from confirmed state ]
```

The same completion action has the owning surface's existing authority.
Phone composition uses the single-sheet view from Task 05, including internal Back and focus return.

## TDD

Add `apply-model-configuration.test.ts` with fake confirmed model/option callbacks before wiring consumers.
Cover success, model rejection, later option rejection, cancellation, rapid replacement, and session/execution changes during application.
Require the partial result to describe actual accepted state rather than restore an optimistic previous draft.
Do not expose a completed configuration until all requested confirmations succeed.

For profile and workflow hooks, test complete exact context, no mutation before Apply, successful draft Apply, existing Save, and retained values on failure.
Options arrive as complete snapshots. Do not merge a late previous model's choices into the current one.
Include a dimension change followed by a different model selection before the first response completes.

For live state, reuse confirmed configuration and request-generation handling.
Extend `session-model-hydration.test.ts` for optional catalog data and older flat snapshots.
Add `TestGroupedCatalogRestore` and `TestComposedConfigurationPartialApply` at existing orchestrator test seams.
Retain settled-startup filtering, explicit overrides, write-once baseline, original effective configuration, and reset/resume guards.
Prove that historical turns retain captured labels/options after the live selection or catalog changes.

Update compact task summaries from verified pairing metadata without interpreting model strings.
Keep the existing changed-from-baseline contract and settings all-values contract.
Keep mode in its own session permission control, including the Devin permission companion.

## Verification

Run from the repository root.

```bash
(cd apps/web && pnpm exec vitest run lib/apply-model-configuration.test.ts lib/state/slices/session-runtime/session-model-hydration.test.ts components/model-config-selector.test.tsx components/settings/profile-model-config.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/task/service -run 'Test.*(GroupedCatalog|ComposedConfiguration|SessionModel|ConfigBaseline|Turn.*Config)' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Add changed hook/store test files to the Vitest command, and record exact ESLint/Go changed-file lint commands from the final diff.
Task 07 owns complete browser proof for every consuming surface.

## Files likely touched

- New `apps/web/lib/apply-model-configuration.ts` and its tests
- `apps/web/components/task/model-selector.tsx`
- `apps/web/hooks/domains/settings/use-profile-model-options.ts`
- `apps/web/components/settings/profile-model-config.ts`
- `apps/web/components/settings/use-session-config-model-options.ts`
- Current workflow override form and its existing resolution/save tests
- Existing session runtime actions, config-option WebSocket handler, and `session-model-hydration.ts`
- Existing orchestrator selector snapshot/restoration/turn tests, with focused new test files where needed
- Existing model-trigger summary helpers and tests

## Dependencies

Task 05 and both Devin permission work orders.
Re-read the current authoritative mutation and restoration code before changing the integration.

## Risks

- Model selection and option selection can partially succeed.
- An asynchronous provider update can belong to a superseded session/execution or configuration generation.
- Current catalog labels cannot retroactively change captured historical attribution.

## Parallelism

`sequential`

## Inputs

- [Resolution and application design](../../specs/agents/system-design/grouped-model-catalog.md#resolution-and-application)
- [Dynamic provider requirements](../../specs/agents/requirements/dynamic-provider-options.md)
- [Existing summary/attribution contract](../../specs/ui/requirements/acp-model-configuration-summary.md)
- Existing selector, profile/workflow resolution, runtime confirmation, reset/resume, and turn snapshot tests.

## Results

Completed 2026-11-03.

- `lib/apply-model-configuration.ts` applies the model first, then each option in deterministic order, reports each accepted mutation to the owning surface, and returns only the state the provider actually confirmed. It never reports a completed configuration for a partial, superseded, or cancelled application, and a retry can resume from an already confirmed model. `apply-model-configuration.test.ts` covers success, model rejection, later option failure, confirmed callbacks, retry resumption, replacement fencing, and pre-start cancellation.
- Every production consumer is wired to the completed exact model plus its real provider option values:
  - Profile drafts resolve candidate options through the settings probe and Apply merges the model and option values into the draft; the existing Save persists them.
  - Workflow override rules receive the same completed model plus option values on the rule draft.
  - Live sessions resolve candidate options without mutation, then apply the model first and each option in confirmation order through `setSessionConfigOption`/`setSessionModel`, fenced by the per-session request id and the confirmed execution id. A later option rejection keeps the confirmed model, shows a partial-application toast, and retries from confirmed state without re-applying the model; a superseded request confirms nothing.
- Live sessions reuse the existing mutation path, confirmation projection, and error handling; conversation identity and newer-mutation fencing are unchanged.
- `session-model-hydration.test.ts` plus `session-model-catalog-hydration.test.ts` cover optional catalog data and older flat snapshots. `lifecycle/session_models_catalog_test.go` proves `TestGroupedCatalogRestore` and `TestComposedConfigurationPartialApply` through the real JSON rehydration path and that legacy flat snapshots stay flat.
- Commands: `pnpm exec vitest run lib/apply-model-configuration.test.ts lib/state/slices/session-runtime/session-model-hydration.test.ts lib/state/slices/session-runtime/session-model-catalog-hydration.test.ts components/settings/profile-form-fields.test.tsx components/task/model-selector-consecutive-switch.test.tsx` passed. `go test -trimpath ./internal/agent/runtime/lifecycle -run 'TestGroupedCatalog|TestComposedConfigurationPartialApply' -count=1` passed.
