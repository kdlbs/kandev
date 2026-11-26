---
id: "02-reconcile-persisted-options"
title: "Reconcile persisted options"
status: done
wave: 2
depends_on:
  - "01-replay-verified-options"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001
acceptance_criteria:
  - AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001.5
  - AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001.9
system_design:
  - ../../specs/platform/system-design/session-config-cross-agent-reconcile.md
---

# Task 02. Reconcile persisted options

## Summary

Reconcile the session's observed runtime options with the complete catalog that
Cursor reports for its active model. Keep a partial report from losing settings.

## In scope

- In `handleSessionModelsEvent`, apply catalog reconciliation only after the
  existing owner, attempt, generation, and startup publication checks.
- In `persistSessionRuntimeConfigOnSession`, remove absent IDs only with
  complete evidence. Retain supported saved values when the report supplies
  no value. Accept reported values including `"false"`.
- Use the existing JSON compare-and-set support for competing writes to the
  runtime metadata key. Reload the raw metadata value and recompute upon a
  conflict. Keep `persistRuntimeModelMetadata` from overwriting a new option
  map with a map from an older row.

## Out of scope

- Profile, explicit override, and original settings mutations.
- Cleanup across unrelated sessions or providers.

## Acceptance

- A complete Sonnet catalog removes `fast` from the stored
  `runtime_config.config_options`. Supported values survive or take the
  provider-reported value. The live event publishes the reported controls.
- Partial, synthetic, stale, and provider-restored reports preserve saved
  intent. Repeated authoritative reports converge on the same runtime map.
- Legacy rows already labeled Sonnet clean up on an accepted complete report.
  A concurrent model-only write and a retry after a failed runtime write
  preserve the latest reconciled options.

## Regression cases

Add `TestRuntimeConfigReconcilesVerifiedCatalog` next to the streaming
event tests. Use `setupTestRepo` and `handleSessionModelsEvent` to read back
the SQLite row and check the published event. The complete-catalog case fails
before the fix because the stored map contains `fast`.

Add `TestPersistRuntimeModelMetadataPreservesConcurrentCursorCatalog` next
to the executor persistence tests. Hold the model-only writer after it reads
the old map. Reconcile the catalog before it can write. Assert that the
model-only writer keeps the current map.

| Lifecycle case | Evidence | Criterion |
| --- | --- | --- |
| Live switch | Complete catalog removes absent keys and publishes its controls | .5, .9 |
| Cold resume | Settled complete report cleans a row whose model is already correct | .9 |
| Partial report | Omitted IDs and explicit selections survive. Matching provider values still update. | .9 |
| Duplicate, stale generation, or replaced execution | Older and synthetic reports cannot restore keys. Replayed reports yield the same map. | .9 |
| Concurrent metadata writes | Losing write retries against current data and preserves absent keys as absent | .9 |
| Provider-restored recovery | Existing end-to-end projection test preserves saved selections and reports effective state | .9 |

## Verification

From the repository root, run the targeted tests and package checks.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -count=1 \
  ./internal/orchestrator ./internal/orchestrator/executor \
  -run 'Test(RuntimeConfigReconcilesVerifiedCatalog|PersistRuntimeModelMetadataPreservesConcurrentCursorCatalog|ProviderRestoredSessionReportsPreserveSavedSelections|SessionSettingsSameSourceFencesEmptyAttemptGeneration|PersistSessionRuntimeConfigUpdatesProviderState|PersistRuntimeModelMetadataStoresRuntimeConfigAndClearsContextWindow)')
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- `apps/backend/internal/orchestrator/event_handlers_model_options.go`
- `apps/backend/internal/orchestrator/event_handlers_model_options_test.go`
- `apps/backend/internal/orchestrator/executor/executor_interaction.go`
- `apps/backend/internal/orchestrator/executor/executor_runtime_model_metadata_test.go`
- `apps/backend/internal/task/repository/session_runtime_config.go`

## Dependencies

[Task 01. Replay verified options](task-01-replay-verified-options.md) supplies
the catalog evidence on the accepted model events.

## Risks

- An old source report can carry a complete-looking option list. Verify its
  owner and generation before mutating persisted state.
- If the runtime write fails, a generation advance can block retry. Verify
  that a later complete report can reconcile the same row.

## Parallelism

`sequential`

## Inputs

- [Cross-agent reconciliation requirements](../../specs/platform/requirements/session-config-cross-agent-reconcile.md)
- [Backend persistence design](../../specs/platform/system-design/session-config-cross-agent-reconcile.md#persistence)
- `TestPersistSessionRuntimeConfigUpdatesProviderState` and
  `TestProviderRestoredSessionReportsPreserveSavedSelections` in orchestrator tests.

## Results

The baseline SQLite probe retained `fast`. The executor regression also
failed when a model-only write restored the old map. The targeted Go command
now passes all reconciliation, retry, and compatibility cases. Specification
catalog and structure checks passed.
