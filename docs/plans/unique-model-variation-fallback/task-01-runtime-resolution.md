---
id: "01-runtime-resolution"
title: "Resolve one executor variation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-001
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-002
acceptance_criteria:
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.1
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.2
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.3
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.4
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.5
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.6
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.8
system_design:
  - ../../specs/agents/system-design/no-silent-model-fallback-01.md
  - ../../specs/agents/system-design/no-silent-model-fallback-02.md
---

# Task 01: Resolve One Executor Variation

## Summary

Extend the single lifecycle model policy with safe unique-variation inference.
Expose a distinct decision without changing explicit fallback semantics.

## In scope

- Add the pure backend candidate matcher.
- Apply the defined four-step precedence order.
- Extend decision, logging, and warning metadata behavior.
- Cover initial launch, context reset, and workspace rebind.

## Out of scope

- Host advisory UI and translations.
- Provider-specific variation parsing or ranking.
- Profile persistence changes.

## Acceptance

- One valid distinct variation is the only inferred model-selection call.
- Exact and advertised explicit fallback models remain higher priority; zero or
  multiple variations cause no inference.
- Automatic-fallback profiles keep the legacy no-selection behavior when the
  requested model is absent.
- The warning records the unique outcome and effective model without setting
  the explicit fallback field or compatibility fallback event.

## Verification

```bash
(
  cd apps/backend
  go test -tags fts5 -run 'TestApplyStartModelPolicy|TestInitializeAndPrompt.*Model|TestReapplySessionModel|TestWorkspaceRebind.*Model|Test.*ModelSelectionWarning' ./internal/agent/runtime/lifecycle
  go test -tags fts5 ./internal/agent/runtime/lifecycle
)
git diff --check
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/start_model.go`
- `apps/backend/internal/agent/runtime/lifecycle/start_model_executor_authority_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/session.go`
- `apps/backend/internal/agent/runtime/lifecycle/session_model_selection_warning_test.go`
- `apps/backend/internal/agentctl/types/streams/agent.go`

## Dependencies

None.

## Risks

- The Cursor-specific configuration parser rejects labels such as `1m`. This
  matcher must remain a separate model-ID helper.
- A special inferred-model apply path can weaken existing error handling.

## Parallelism

`sequential`

## Inputs

- Requirement 002 and its matching examples.
- The unique-variation ADR.
- Existing `applyStartModelPolicy` and warning publication tests.

## Results

Implemented the executor-authoritative unique-variation policy in
`start_model.go`, including explicit-fallback precedence, opaque and
case-sensitive matching, the legacy automatic-fallback path, and distinct
warning metadata without a legacy fallback event. Initial launch, context
reset, and workspace rebind all use the same policy. Focused model-policy
tests passed (29), and the full lifecycle suite passed (2,364).
