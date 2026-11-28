---
id: "01-replay-verified-options"
title: "Replay verified options"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001
acceptance_criteria:
  - AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001.1
  - AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001.2
  - AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001.3
  - AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001.6
  - AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001.8
system_design:
  - ../../specs/platform/system-design/session-config-cross-agent-reconcile.md
---

# Task 01. Replay verified options

## Summary

After startup selects a model, use provider evidence for that model to replay
saved Cursor options. This protects existing rows before they receive the
persistence repair in Task 02.

## In scope

- In the Cursor ACP v1 adapter, mark complete, losslessly normalized catalogs
  with an identified active model. Carry the evidence in the optional
  `streams.ConfigurationResult` from the model setter and in
  `streams.SessionModelState` from setup. Use the existing `session_models`
  path for live and settled reports.
- In `InitializeAndPromptWithLayers`, capture the correlated result from
  `applyStartModelPolicyToEffectiveModel`. Keep that result local to startup
  and use the policy's existing model outcome. If startup waits for an update,
  match the native session, selected model, and post-switch generation in the
  agentctl client cache.
- Pass the verified catalog to the saved profile and runtime option layers.
  Use the existing restorable-option predicate and startup warning path.
  Publish evidence with the actual settled snapshot for Task 02.

## Out of scope

- Effective option map writes and database backfill.
- New provider option types or changes to the public selector event.

## Acceptance

- A saved Opus `fast="false"` produces zero option setter calls and zero
  warning entries when the confirmed Sonnet catalog omits it.
  A saved `reasoning_effort` value still reaches the setter.
- A missing, unqualified, or pre-switch report defers unverified writes.
  The same-model catalog from the current setup qualifies when no switch occurs.
- If a supported option fails, the existing startup warning includes its ID.
  The restored session retains its native identity and can become ready.

## Regression cases

Add `TestModelResultCarriesVerifiedCatalog` alongside existing adapter
configuration tests. Drive a typed model reply, an acknowledgment without a
catalog, and a projection that drops an option ID. Confirm that only the
complete reply carries model-bound evidence.

Add `TestInitializeAndPromptWithLayersModelScopedReplay` beside the
runtime layer tests. Seed the old catalog and withhold publication of the newer
event. Return the provider's new catalog in the correlated result. Before the
fix, the test observes a `fast` call and a startup failure entry. Exercise both
the complete result and the bounded notification path using controlled delivery.

| Lifecycle case | Evidence | Criterion |
| --- | --- | --- |
| Cold resume with a legacy row | One native load, no stale setter, supported saved value applied, session ready | .1, .2, .3 |
| Default model from current setup | Saved options filtered with that setup's catalog | .1, .3 |
| Partial, late, or mismatched report | No unverified setter or warning. A complete current report can qualify within the wait budget. | .1, .2, .6 |
| Advertised value rejected | Failure event names just the rejected option | .2 |
| Existing cross-agent, startup, and recovery paths | Existing lifecycle tests for supported replay, startup publication, and provider-restored settings | .6, .8 |

## Verification

From the repository root, run the focused backend checks.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -count=1 \
  ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/api \
  ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle \
  -run 'Test(ModelResultCarriesVerifiedCatalog|InitializeAndPromptWithLayersModelScopedReplay|ApplyRuntimeSessionLayersFiltersUnadvertisedOptions|SessionModelsPublishOnlySettledStartupState|InitializeAndPromptWithLayers_ProviderRestoredOmitsSavedStartupSettings|InitializeAndPrompt_AppliesProfileConfigOptions|SetConfigOptionWithResultUsesItsACPResponse|SetConfigOptionWithPolicyCarriesHostPolicy)')
git diff --check
```

## Files likely touched

- `apps/backend/internal/agentctl/types/streams/configuration_result.go`
- `apps/backend/internal/agentctl/types/streams/agent.go`
- `apps/backend/internal/agentctl/types/streams/session_settings_policy.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_updates.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/model_catalog.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/model_catalog_test.go`
- `apps/backend/internal/agent/runtime/agentctl/agent.go`
- `apps/backend/internal/agent/runtime/agentctl/client.go`
- `apps/backend/internal/agent/runtime/lifecycle/session.go`
- `apps/backend/internal/agent/runtime/lifecycle/types.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_events.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/agent/runtime/lifecycle/session_model_options.go`
- `apps/backend/internal/agent/runtime/lifecycle/session_model_options_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/session_runtime_layers_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/session_test.go`

## Dependencies

None.

## Risks

- The lifecycle cache also feeds startup publication. Keep the provider-default
  baseline separate from the catalog that verifies the selected model.
- An empty reply or a filled-in model label can arrive before the provider's
  full report. Use the existing bounded wait and defer unverified writes.

## Parallelism

`sequential`

## Inputs

- [Cross-agent reconciliation requirements](../../specs/platform/requirements/session-config-cross-agent-reconcile.md)
- [Provider catalog evidence and startup replay](../../specs/platform/system-design/session-config-cross-agent-reconcile.md#provider-catalog-evidence)
- `TestApplyRuntimeSessionLayersFiltersUnadvertisedOptions` and
  `TestSessionModelsPublishOnlySettledStartupState` in lifecycle tests.

## Results

The new adapter test failed on the baseline because the model setter omitted
the verified catalog. The new cold-resume test reproduced the stale setter.
Both now pass. The focused Go command above passed, including the existing
cross-agent and provider-restored replay tests. The API package compiled.
