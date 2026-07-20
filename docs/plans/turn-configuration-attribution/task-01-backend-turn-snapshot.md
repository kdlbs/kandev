---
id: "01-backend-turn-snapshot"
title: "Persist the effective configuration on each turn"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/acp-model-configuration-summary.md"
---

# Task 01: Persist the Effective Configuration on Each Turn

## Acceptance

- Creating two turns around a session configuration change persists different immutable snapshots on the two turn rows.
- Each snapshot contains model, mode, ordered selected option IDs/raw values/display names, and the captured provider-default baseline.
- Later prompt-usage metadata updates preserve the captured snapshot while refining model/usage attribution.

## Verification

```bash
rtk go test -run 'TestStartTurn.*Config' ./internal/task/service
rtk go test -run 'TestPersistPromptMetadata.*Config' ./internal/orchestrator
```

Run from `apps/backend`.

## Files

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/service/service_turns.go`
- `apps/backend/internal/task/service/service_turns_test.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming_test.go`

## Inputs

- Spec data model, persistence guarantees, and cross-turn scenario.
- ADR-2026-07-18-turn-configuration-snapshots.
- Existing `SessionRuntimeConfig`, `SessionModelsSnapshot`, `StartTurn`, and `persistPromptMetadataOnTurn` patterns.

## Output Contract

Report the persisted JSON shape, effective-value precedence, failing/passing tests, files changed, and any turn-creation timing gaps.
