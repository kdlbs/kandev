---
id: "04-reconcile-active-model-options"
title: "Reconcile options for the active model"
status: done
wave: 4
depends_on:
  - "03-verify-provider-option-reports"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001
acceptance_criteria:
  - AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001.1
  - AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001.2
  - AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001.3
  - AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001.6
  - AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001.8
  - AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001.9
system_design:
  - ../../specs/platform/system-design/session-config-cross-agent-reconcile.md
---

# Task 04. Reconcile options for the active model

## Summary

Use the same verified option report in startup and persistence for all
providers. An old report must not authorize a write after a model switch.

## In scope

- Replace the Cursor checks in lifecycle and orchestration with the report
  from Task 03. For startup, use the correlated reply or wait for a newer
  matching report in the agentctl client. A different model, native session,
  or attempt invalidates old evidence. When startup keeps its model, it can
  use the current setup report or wait within the same budget for that report.
- Filter the saved profile and runtime options using the verified IDs.
  Preserve mode confirmation and provider-restored recovery. Keep the
  existing warning for an attempted write that the provider rejects.
- Apply complete reports to the effective option map under the existing
  settings fences and repository comparison. Merge only observed entries
  from partial reports, including when they carry copied display options.
  Keep saved choices and preserve the generation for retry if a sequenced
  report's runtime write fails.

## Out of scope

- Database migration, per-model settings history, context reset, and UI
  changes.

## Acceptance

- Cursor and a second provider fixture replay only verified options for the
  selected model. Supported settings reach the setter. Stale settings cause
  zero setter calls and zero startup warnings.
- A complete report removes absent IDs. Partial reports update verified
  values and preserve omitted keys. A failed write can be retried. Neither
  a copied option list nor a stale or superseded report changes saved values.
- Legacy startup with the current setup report, Pi thinking settings, and
  provider-restored explicit recovery pass their existing tests.

## Regression cases

Parameterize `TestInitializeAndPromptWithLayersCursorModelScopedReplay` and
`TestCursorRuntimeConfigReconcilesVerifiedCatalog` as
`TestInitializeAndPromptWithLayersModelScopedReplay` and
`TestRuntimeConfigReconcilesVerifiedCatalog`. Use the existing agentctl
stream fixture and the real SQLite-backed model-event handler. Cover reply,
delayed report, wrong-model and wrong-session reports, missing evidence, and
a supported rejection. Include the newer different-model report and an
unverified copied list after a complete report. Cover a late same-model setup
report and a synchronous display-only setup snapshot. In the SQLite fixture,
redeliver a partial report after a failed write at the same generation.

Keep the concurrency case in
`TestPersistRuntimeModelMetadataPreservesConcurrentCursorCatalog`. Run it
under the shared write policy. Use `TestApplyRuntimeSessionLayersFiltersUnadvertisedOptions`
and `TestProviderRestoredSessionReportsPreserveSavedSelections` as controls.

## Verification

Run from `apps/backend`.

```bash
go test -trimpath -tags fts5 -p=2 -timeout=20m -count=1 \
  ./internal/agent/runtime/lifecycle \
  ./internal/orchestrator \
  ./internal/orchestrator/executor
```

After both work orders pass, rerun the backend workflow on the updated PR.
Explain the shared behavior in the PR table and retain Cursor as the example.

## Likely files

- `internal/agent/runtime/lifecycle/session.go`, `session_model_options.go`,
  `manager_events.go`, and the lifecycle regression fixtures.
- `internal/orchestrator/event_handlers_model_options.go`,
  `event_handlers_streaming.go`, and the runtime option event tests.
- `internal/orchestrator/executor/executor_runtime_model_metadata_test.go`.

## Risk

An older helper can acknowledge a model switch without a verified settings
report. Startup defers those settings within the existing wait budget. The
saved values remain available when a provider gives Kandev more evidence.

## Results

The temporary startup probe at `243bdab59e6f` and the permanent fixture before
the lifecycle change both sent `fast` for the non-Cursor agent after the model
setter returned the Sonnet catalog.

The existing profile replay fixture now supplies the selected model's options
from the model setter. It still checks the profile option write and leaves the
final option response unconfirmed to verify startup settlement.

The focused replay, SQLite reconciliation, concurrent model write, Pi
thinking, and provider-restored tests passed. The explicit selection
compatibility case also passed after host snapshots kept their additive merge.

The orchestrator and executor package suites passed. The full lifecycle suite
passed on the rerun with the active-model profile fixture. The focused
lifecycle tests and changed-package lint passed after simplifying the
model-switch predicate.

The review checks reproduced a dropped verified value on a display-only event,
a missed late same-model report, and a partial runtime write that could not
be retried at the same generation. The lifecycle, orchestrator, and executor
suites passed after the fixes in the [review verification](plan.md#verification-results).
