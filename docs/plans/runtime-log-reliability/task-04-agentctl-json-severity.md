---
id: "04-agentctl-json-severity"
title: "Preserve agentctl JSON severity"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DIAGNOSTIC-SIGNAL-001
acceptance_criteria:
  - AC-PLATFORM-DIAGNOSTIC-SIGNAL-001.1
  - AC-PLATFORM-DIAGNOSTIC-SIGNAL-001.2
  - AC-PLATFORM-DIAGNOSTIC-SIGNAL-001.3
system_design:
  - ../../specs/platform/system-design/runtime-diagnostic-signal.md
---

# Task 04: Preserve agentctl JSON severity

## Summary

Recognize complete agentctl JSON log records in the existing child parser. Preserve the stream fallback and every existing supported format.

## In scope

- Anchored JSON record parsing with duplicate-field and trailing-content refusal.
- Existing stdout/stderr forwarding and info-threshold observer tests.
- Error-class levels map to parent Error without process termination.

## Out of scope

- Arbitrary JSON parsing, raw ACP data, provider stderr changes, log flattening, and new logging configuration.

## Acceptance

- Trusted JSON INFO/WARN/ERROR records retain severity. WARN and ERROR remain visible at an info threshold.
- Malformed, unknown-level, payload-only, nested-level, and duplicate-field records keep the stream-specific fallback.
- Existing console/slog fixtures pass. Fatal/panic child records do not terminate or panic the parent.

## Verification

Run this block from the repository root after the implementation result exists.
New test names below are required planned regressions, not claims of existing coverage.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/runtime/agentctl/launcher -run 'Test(ChildLogLevel|StripANSI|PipeOutput)' -count=1)
```

## Files likely touched

- `apps/backend/internal/agent/runtime/agentctl/launcher/launcher.go`
- `apps/backend/internal/agent/runtime/agentctl/launcher/launcher_loglevel_test.go`
- `apps/backend/internal/agent/runtime/agentctl/launcher/launcher_json_log_test.go (new)`

## Dependencies

None. Follow the plan's sequential priority order.

## Risks

- A permissive parser can misclassify arbitrary child content. Validate the root record shape and retain original fallback behavior.

## Parallelism

`sequential`

## Inputs

- [Plan](plan.md), especially evidence, contract ownership, and completion rules.
- [Design](../../specs/platform/system-design/runtime-diagnostic-signal.md) and its linked requirements.

- Scoped backend/agentctl instructions for any touched package.
- Existing source and tests listed above. Preserve completed companion-package results.

## Results

Implemented strict parsing for complete agentctl JSON records. The parser validates the canonical timestamp, nonempty caller, string message and level, unique required envelope fields (`level`, `timestamp`, `caller`, and `msg`), recognized level, and absence of trailing content. Repeated additional fields are decoded and ignored, preserving trusted Zap records whose nested `WithFields` calls append duplicate `component` keys. The original line and stream field are forwarded. FATAL, PANIC, and DPANIC remain parent Error entries, so child records cannot terminate or panic the backend. Malformed and unknown JSON keeps the existing stdout DEBUG / stderr WARN fallback.

Tests cover valid JSON INFO/WARN/ERROR at an info threshold, WARN and ERROR emitted by a real Zap JSON encoder with nested duplicate `component` fields, all four duplicate envelope fields remaining rejected, fatal child severity without parent termination, payload and nested level decoys, malformed timestamps and field types, repeated extra fields, trailing JSON/text, and stream fallbacks.

Passed:

- `(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/runtime/agentctl/launcher -run 'Test(ChildLogLevel|StripANSI|PipeOutput)' -count=1)`
- Full affected-package tests passed: `go test -trimpath -tags fts5 ./internal/task/handlers ./internal/task/service ./internal/orchestrator/messagequeue ./internal/agent/runtime/agentctl/launcher -count=1`.
- `make -C apps/backend build` passed.

Existing console and slog parser/relay tests are included in that command.
