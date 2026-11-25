---
created: 2026-11-25
status: implemented
requirements:
  - REQ-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001
system_design:
  - ../../specs/platform/system-design/session-config-cross-agent-reconcile.md
legacy_specs: []
---

# Fix plan for Cursor model option reconciliation

## Overview

Restore saved Cursor options against the active model's confirmed catalog.
Then reconcile the saved effective options whenever Cursor reports a complete
catalog. [Issue #5508](https://github.com/kdlbs/kandev/issues/5508) is assigned to
`carlosflorencio`.

## Confirmed root cause

Startup reads the lifecycle cache after selecting the effective model. The
model setter can return before the selected model's `session_models` event
reaches that cache. The previous model's catalog can admit a saved option
that the active model rejects. `applyRuntimeSessionLayers` sends the option
and adds its ID to the startup failure list.

Separately, `applySessionRuntimeConfigUpdate` merges reported values into
`runtime_config.config_options`. An accepted Sonnet snapshot updates the model
and reasoning effort while the map retains the old `fast` key.

Two temporary Go tests reproduced these paths at `c671d8080883`.
The first held the model event after a stub confirmed Sonnet. The real startup
replay path sent `fast="false"` and returned `[fast]` as failed.
The second delivered a Sonnet catalog through `handleSessionModelsEvent` with
a seeded SQLite session. The stored map still contained `fast="false"`.
Both probes exercised the existing service methods through a temporary Go
overlay. The report supplies the real Cursor log. The original failed
attempt's catalog and a fresh authenticated Cursor reproduction remain unavailable.

## Scope

### In scope

- Cursor ACP v1 startup and cold resume through the normal startup layers.
- Complete model confirmation from Cursor setup, replies, and live updates.
- Stale effective options in existing rows and new in-session model switches.
- Startup warnings for rejected settings that the active catalog advertises.

### Out of scope

- Database-wide cleanup and changes to explicit override ownership.
- Context-reset or workspace-rebind changes outside the startup path.
- Startup policy changes for model fallback, permission mode, or provider-restored recovery.

## Technical approach

1. Carry the provider's complete catalog through the correlated agentctl model
   result and the existing stream. Reuse the startup catalog wait for later
   reports from the same native session. Use only qualified evidence to
   filter saved Cursor options in both startup layers.
2. Reconcile the observed `runtime_config` in the accepted model-event handler.
   Fence writes with the existing owner and generation checks. Keep concurrent
   model-only writes from restoring a stale option map.

A cache-only change would still depend on stream timing. A write-only change
would leave old rows and older peers able to reach replay with stale options.
The [system design](../../specs/platform/system-design/session-config-cross-agent-reconcile.md#backend-replay-and-persistence)
defines the evidence and the storage boundary.

| Source | Evidence | Startup and persistence behavior |
| --- | --- | --- |
| Cursor ACP v1 typed setup for the active native session | Identified model and complete option IDs | Replay when this is the effective setup. Reconcile after startup settles. |
| Cursor ACP v1 typed setter reply | Correlated model and complete option IDs | Replay supported options without waiting for the lifecycle event. Reconcile via the accepted event. |
| Cursor ACP v1 typed update | Matching native session and a fresh settings generation for the selected model | Use within the startup wait. A later accepted report can reconcile the saved map. |
| Cursor acknowledgment, partial projection, or older agentctl | Missing qualified catalog | Defer unverified options. Preserve omitted saved keys. |
| Other agents and transports | Existing replay and settings contracts | Existing cross-agent filtering, explicit recovery, and Pi coverage guard those paths. |

## Tests

Criterion suffixes refer to `AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001`.

| Criteria | Planned test | Observable result |
| --- | --- | --- |
| .1, .3, .6 | `TestCursorModelResultCarriesVerifiedCatalog` in ACP adapter tests | Typed replies carry the selected model and option IDs. Partial or synthetic snapshots carry no completeness claim. |
| .1, .2, .3, .6, .8 | `TestInitializeAndPromptWithLayersCursorModelScopedReplay` in lifecycle tests | With a delayed model event, a saved `reasoning_effort` reaches the provider. `fast` has zero setter calls and no warning. |
| .1, .2 | Same lifecycle test, missing catalog and rejection cases | An unknown catalog defers unverified writes. A rejected advertised option reaches the existing startup warning. |
| .5, .9 | `TestCursorRuntimeConfigReconcilesVerifiedCatalog` in orchestrator tests | Live and settled events publish the reported controls. SQLite records supported values and drops absent keys only with complete evidence. |
| .9 | `TestPersistRuntimeModelMetadataPreservesConcurrentCursorCatalog` in executor tests | A model-only write interleaved with reconciliation preserves the clean map. |

## End-to-end evidence

The lifecycle regression drives `InitializeAndPromptWithLayers` through the
normal agentctl stream with a stub peer. It exercises cold load, saved option
replay, the startup failure event, and readiness on the same native session.
The orchestrator regression drives the real model event handler and reads
the persisted row. Together these cover the provider boundary and the stored
state that a later resume reads. Both checks run without Cursor credentials.

The optional provider smoke uses a disposable instance with the reported
Cursor model pair. Switch from Opus to Sonnet, restart the session, and send a
prompt after it becomes ready. Check the native session identity, outbound
option calls, and transcript for the reported warning.

## Work orders

- [x] [Task 01. Replay verified options](task-01-replay-verified-options.md)
- [x] [Task 02. Reconcile persisted options](task-02-reconcile-persisted-options.md) (after Task 01)

## Verification results

- Both diagnostic probes failed at the expected assertions on the baseline.
- The focused regression and compatibility checks passed in nine Go packages.
- The specification catalog, specification lint, and whitespace checks passed.
- The local PR documentation evaluator accepted both work orders for the
  planned backend paths.
- The first full backend run hit package timeouts in process and backend app
  tests and an analytics read deadline. Each affected test passed on its
  focused rerun.

## Risks

- A legacy or incomplete Cursor reply can defer settings for one startup.
  A new provider report gives the next attempt evidence to use them.
- A complete startup catalog can arrive before the session becomes ready.
  Exercise the settled publication so that the option map still reconciles.
- A stale model-only write can restore removed keys. Exercise the actual
  repository interleaving and retry.
