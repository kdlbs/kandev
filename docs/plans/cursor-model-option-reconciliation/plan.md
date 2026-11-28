---
created: 2026-11-25
status: implemented
requirements:
  - REQ-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001
system_design:
  - ../../specs/platform/system-design/session-config-cross-agent-reconcile.md
legacy_specs: []
---

# Fix plan for model-dependent session options

## Overview

Restore saved options using the running provider's catalog for the active model.
Clean up the saved effective options when the provider sends a complete report.
Tasks 01 and 02 implemented the Cursor path in the draft PR. Tasks 03 and 04 extend
it to any provider that can verify the settings for its active model.
[Issue #5508](https://github.com/kdlbs/kandev/issues/5508) is assigned to
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

The draft PR checks the Cursor agent ID in the v1 adapter, startup layers,
and runtime option persistence. A temporary overlay at `243bdab59e6f` reused
the Cursor regression fixtures with the agent ID `generic-acp`. The adapter
discarded the typed reply's catalog evidence. Cold resume sent the stale
`fast` setting. The work orders turn those fixtures into regressions that
exercise the same code under more than one agent ID.

## Scope

### In scope

- Cursor ACP v1 startup and cold resume through the normal startup layers.
- Complete model confirmation from Cursor setup, replies, and live updates.
- Shared replay and persistence for provider-verified active-model options.
- ACP v1 and v2 typed settings reports. Existing Grok model metadata provides
  evidence for the reasoning options it describes.
- Stale effective options in existing rows and new in-session model switches.
- Startup warnings for rejected settings that the active catalog advertises.

### Out of scope

- Database-wide cleanup and changes to explicit override ownership.
- Context-reset or workspace-rebind changes outside the startup path.
- Startup policy changes for model fallback, permission mode, or provider-restored recovery.

## Technical approach

1. Carry Cursor's complete catalog through the correlated agentctl model
   result and the existing stream. Reuse the startup catalog wait for later
   reports from the same native session. Use only qualified evidence to
   filter saved Cursor options in both startup layers.
2. Reconcile the observed `runtime_config` in the accepted model-event handler.
   Fence writes with the existing owner and generation checks. Keep concurrent
   model-only writes from restoring a stale option map.
3. Let each transport associate the options it can verify with the model,
   native session, and settings generation.
4. Use that report in both startup layers and persistence for every provider.
   Replay supported IDs. Limit removal of absent saved IDs to complete reports.

A cache-only change would still depend on stream timing. A write-only change
would leave old rows and older peers able to reach replay with stale options.
The [system design](../../specs/platform/system-design/session-config-cross-agent-reconcile.md#backend-replay-and-persistence)
defines the evidence and the storage boundary.

| Source | Evidence | Startup and persistence behavior |
| --- | --- | --- |
| ACP v1 or v2 typed setup | Identified model and complete option IDs | Replay from the current setup. Reconcile after startup settles. |
| ACP v1 or v2 typed setter reply | Correlated model and complete option IDs | Replay supported options as soon as the reply arrives. Reconcile via the accepted event. |
| Delayed provider report | Matching native session and a fresh settings generation for the effective model | Use within the startup wait, including a resume that keeps the same model. A later accepted report can reconcile the saved map. |
| Partial provider report, such as Grok's verified reasoning value | Individually verified entries | Replay and save verified entries. Preserve omitted saved keys. |
| Copied display options or an acknowledgment with missing evidence | No verified entries | Defer replay and keep the saved values. |

Task 03 adds provider-neutral evidence to the existing agentctl results and
events. Task 04 replaces the Cursor gates in the consumers. A partial report
may verify individual entries for replay. Missing and partial reports
preserve saved keys they omit. Legacy model acknowledgments with copied option
lists can update the displayed model while Kandev waits for provider evidence.

## Tests

Criterion suffixes refer to `AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001`.

| Criteria | Planned test | Observable result |
| --- | --- | --- |
| .1, .3, .6 | `TestModelResultCarriesVerifiedCatalog` in ACP adapter tests | Typed replies carry the selected model and option IDs. Partial or synthetic snapshots carry no completeness claim. |
| .1, .2, .3, .6, .8 | `TestInitializeAndPromptWithLayersModelScopedReplay` in lifecycle tests | With a delayed model event, a saved `reasoning_effort` reaches the provider. `fast` has zero setter calls and no warning. |
| .1, .2 | Same lifecycle test, missing catalog and rejection cases | An unknown catalog defers unverified writes. A rejected advertised option reaches the existing startup warning. |
| .5, .9 | `TestRuntimeConfigReconcilesVerifiedCatalog` in orchestrator tests | Live and settled events publish the reported controls. SQLite records supported values and drops absent keys only with complete evidence. |
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
- [x] [Task 03. Verify provider option reports](task-03-verify-provider-option-reports.md) (after Task 02)
- [x] [Task 04. Reconcile options for the active model](task-04-reconcile-active-model-options.md) (after Task 03)

## Verification results

- Both diagnostic probes failed at the expected assertions on the baseline.
- The focused regression and compatibility checks passed in nine Go packages.
- The specification catalog, specification lint, and whitespace checks passed.
- The local PR documentation evaluator accepted both work orders for the
  planned backend paths.
- The first full backend run hit package timeouts in process and backend app
  tests and an analytics read deadline. Each affected test passed on its
  focused rerun.
- All eight package suites for provider-neutral replay and persistence passed.
  The lifecycle suite passed after the profile fixture supplied the active
  model's options. The final focused lifecycle checks and Go lint passed.
- The review regressions reproduced lost display provenance, a guessed
  reasoning value treated as evidence, missed late setup options, and a
  partial write that could not be retried. All nine suites below passed after
  remediation. The spec catalog, spec lint, and PR document coverage checks
  passed again.

Run the review verification from `apps/backend`.

```bash
go test -trimpath -tags fts5 -p=2 -count=1 -timeout=20m \
  ./internal/agentctl/acpcompat \
  ./internal/agentctl/server/adapter/transport/acp \
  ./internal/agentctl/server/adapter/transport/acpv2 \
  ./internal/agentctl/server/api \
  ./internal/agentctl/types/streams \
  ./internal/agent/runtime/agentctl \
  ./internal/agent/runtime/lifecycle \
  ./internal/orchestrator \
  ./internal/orchestrator/executor
```

## Risks

- A legacy or incomplete Cursor reply can defer settings for one startup.
  A new provider report gives the next attempt evidence to use them.
- A complete startup catalog can arrive before the session becomes ready.
  Exercise the settled publication so that the option map still reconciles.
- A stale model-only write can restore removed keys. Exercise the actual
  repository interleaving and retry.
- A legacy provider can acknowledge a model change before reporting its
  options. Defer settings that lack evidence for the selected model. Verify
  the existing setup and provider-restored compatibility paths.
