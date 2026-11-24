---
id: "02-safe-child-diagnostics"
title: "Retain safe child diagnostic context"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DIAGNOSTIC-SIGNAL-001
  - REQ-PLATFORM-DIAGNOSTIC-SIGNAL-002
  - REQ-PLATFORM-DIAGNOSTIC-SIGNAL-003
acceptance_criteria:
  - AC-PLATFORM-DIAGNOSTIC-SIGNAL-001.1
  - AC-PLATFORM-DIAGNOSTIC-SIGNAL-001.2
  - AC-PLATFORM-DIAGNOSTIC-SIGNAL-001.3
  - AC-PLATFORM-DIAGNOSTIC-SIGNAL-001.4
  - AC-PLATFORM-DIAGNOSTIC-SIGNAL-001.5
  - AC-PLATFORM-DIAGNOSTIC-SIGNAL-002.5
  - AC-PLATFORM-DIAGNOSTIC-SIGNAL-003.1
  - AC-PLATFORM-DIAGNOSTIC-SIGNAL-003.2
  - AC-PLATFORM-DIAGNOSTIC-SIGNAL-003.3
  - AC-PLATFORM-DIAGNOSTIC-SIGNAL-003.4
system_design:
  - ../../specs/platform/system-design/runtime-diagnostic-signal.md
---

# Task 02: Retain safe child diagnostic context

## Summary

Preserve typed safe context in recognized agentctl JSON while retaining the existing privacy and stream-draining boundary.
Classify healthy delivery transitions at info and preserve actionable unhealthy warnings.

## In scope

- Implement the exact key/type/value allowlist in the design, including component duplicates, UUIDs, classification enums, exit codes, and pressure integers.
- Keep parent launcher context separate from reconstructed child JSON.
- Omit invalid optional context without raw-line fallback. Retain malformed mandatory-envelope behavior and existing console/slog support.
- Update envelope-only tests to accept safe fields while retaining their sensitive-field assertions.
- Add safe filesystem/submodule producer classification through the common filesystem diagnostic boundary.
- Emit info only for healthy pressure transitions. Preserve all state, journal, prompt-admission, and cancellation behavior.

## Out of scope

- Arbitrary text fields, nested objects, raw errors, paths, provider stderr, secrets, global threshold changes, or sampling.
- Pressure-limit changes, delivery recovery behavior, new logging queues, or remote/adopted child transport changes.

## Acceptance

- Real child producer JSON forwarded through `pipeOutput` retains safe context at the declared severity, with no sensitive context in message or parent fields.
- Invalid values, repeated optional keys, oversized fields, numeric overflow, fake UUIDs, and sensitive unknown keys cannot escape filtering or hide warnings.
- Real journal transitions log initial/recovered healthy state at info and pressure/guarded states at warning. Existing drain and delivery controls pass.

## Regression evidence

Add `TestPipeOutputPreservesSafeDiagnosticContext` and `TestPipeOutputRejectsUnsafeOptionalContext` in a focused launcher test file.
The safe pressure-field assertion must fail on the recorded base because the reconstructed record drops every optional field.
Add `TestDeliveryPressureHealthyTransitionSeverity` with a real journal and logger observer.
Add `TestWorkspaceDiagnosticSafeClassification` and a submodule failure counterpart at their real producer boundaries.
Use the real Zap JSON encoder and matching field shapes in launcher-owned forwarding tests.
Do not import backend runtime code into standalone agentctl to share test helpers.
The healthy-transition test must fail before the severity correction.

## Verification

Run the new regressions first and record their expected failures.
After implementation, run this block from the repository root:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/runtime/agentctl/launcher -run 'ChildLog|PipeOutput|ChildEnv|EnvironmentWithOverrides' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/common/fsdiagnostics ./internal/agentctl/server/process -run 'Diagnostic|Filesystem|Submodule|DeliveryPressure|FullJournal|LatePressure|DeliveryFailure' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agent/runtime/agentctl/launcher/launcher.go apps/backend/internal/agent/runtime/agentctl/launcher/child_log_context.go apps/backend/internal/common/fsdiagnostics/context.go apps/backend/internal/agentctl/server/process/workspace_diagnostics.go apps/backend/internal/agentctl/server/process/manager_submodules.go apps/backend/internal/agentctl/server/process/delivery_pressure.go
git diff --check
```

## Files likely touched

- `apps/backend/internal/agent/runtime/agentctl/launcher/launcher.go`
- `apps/backend/internal/agent/runtime/agentctl/launcher/child_log_context.go` (new)
- `apps/backend/internal/agent/runtime/agentctl/launcher/launcher_json_log_test.go`
- `apps/backend/internal/agent/runtime/agentctl/launcher/child_log_context_test.go` (new)
- `apps/backend/internal/common/fsdiagnostics/context.go` and sibling classification tests
- `apps/backend/internal/agentctl/server/process/workspace_diagnostics.go` and sibling diagnostic tests
- `apps/backend/internal/agentctl/server/process/manager_submodules.go` and sibling submodule tests
- `apps/backend/internal/agentctl/server/process/delivery_pressure.go`
- `apps/backend/internal/agentctl/server/process/delivery_pressure_logging_test.go` (new)

## Dependencies

None. Execute after Task 01 in manifest order.

## Risks

Existing tests intentionally exclude all child fields, so changing their expected JSON alone cannot prove the new privacy boundary.
Producer and relay negative controls must remain independent.
Canonical UUID validation deliberately omits unsupported identifier shapes.

## Parallelism

`sequential`

## Inputs

- [Plan](plan.md) and [requirements](../../specs/platform/requirements/runtime-diagnostic-signal.md).
- The design's safe context allowlist and child pipe-draining contract.
- `launcher_json_log_test.go`, `launcher_output_test.go`, and `delivery_pressure_test.go`.
- Existing common filesystem diagnostic context and submodule producer.

## Results

- Added the typed child-log context allowlist, bounded scalar validation, and fixed record reconstruction. Optional integer fields reject JSON null instead of forwarding it.
- Added null-integer parser and forwarder regressions for `exit_code`, `retained_bytes`, and `limit_bytes`.
- Preserved parent launcher context, malformed-envelope fallback, duplicate-component handling, and stream draining.
- Classified filesystem failures with the closed operation and error-class vocabulary; submodule discovery now emits safe operation/error-class/exit evidence.
- Healthy delivery-pressure transitions log at info; pressure and guarded transitions remain warnings.
- Added the four planned launcher, process, filesystem, and journal regressions.
- Verification passed: focused race tests and lint.
