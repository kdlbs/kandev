---
id: "01-classify-codex-usage-limit"
title: "Classify codex usage-limit notices"
status: planned
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-CODEX-USAGE-LIMIT-001
acceptance_criteria:
  - AC-AGENTS-CODEX-USAGE-LIMIT-001.1
  - AC-AGENTS-CODEX-USAGE-LIMIT-001.2
  - AC-AGENTS-CODEX-USAGE-LIMIT-001.3
  - AC-AGENTS-CODEX-USAGE-LIMIT-001.4
system_design:
  - ../../specs/agents/system-design/codex-usage-limit-classification.md
---

# Task 01: Classify codex usage-limit notices

## Summary

Recognize the codex usage-limit notice in both apostrophe forms. Carry the
notice through the matching generic ACP prompt error so dynamic routing can
correlate the diagnostic and manual recovery can retain queued work. Parse a
text reset time only when it includes an explicit timezone.

## In scope

- Extend the codex quota rule with the plain usage-limit notice pattern.
- Add a reset-hint text parser and apply it in `Classify` for quota and rate
  classifications without a structured hint. Reject unzoned times and invalid
  calendar, clock, and offset values.
- Capture a Codex quota notice on its active prompt turn and attach it only to
  the matching generic `-32603` / `Internal error` response after draining ACP
  notifications.
- Pass the provider ID into orchestrator diagnostic and terminal error
  classification so the existing code-and-text correlation can authorize
  fallback.
- Normalize the typographic apostrophe in the orchestrator manual-recovery
  check.
- Add focused classification, dynamic-routing, and manual-recovery tests.

## Out of scope

- Changing rules for providers other than codex-acp.
- Changing candidate ordering, circuit behavior, or provider selection.
- Web changes, database migrations, or new provider contracts.

## Acceptance

- A codex-acp notice with a straight or typographic apostrophe classifies as
  `quota_limited` with high confidence and allows fallback.
- A structured reset hint takes precedence. Otherwise, only an explicitly
  zoned valid retry timestamp creates a hint; an unzoned time creates none.
- A Codex quota diagnostic followed by the same prompt's generic ACP error
  yields a typed Codex provider error. Other errors remain unchanged.
- Manual recovery recognizes the typographic-apostrophe notice and retains the
  queued prompt.

## Verification

```bash
gofmt -w <changed Go files>
go test ./internal/agent/runtime/routingerr ./internal/agent/runtime/dynamic ./internal/agentctl/server/adapter/transport/acp ./internal/orchestrator -count=1
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.py validate
git diff --check
```

Run the Go tests from `apps/backend`.

## Files likely touched

- `apps/backend/internal/agent/runtime/routingerr/rules.go`
- `apps/backend/internal/agent/runtime/routingerr/resethint.go`
- `apps/backend/internal/agent/runtime/routingerr/routingerr.go`
- `apps/backend/internal/agent/runtime/routingerr/classify_test.go`
- `apps/backend/internal/agent/runtime/routingerr/resethint_test.go`
- `apps/backend/internal/agent/runtime/dynamic/engine_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_codex_usage_test.go`
- `apps/backend/internal/orchestrator/dynamic_evidence.go`
- `apps/backend/internal/orchestrator/dynamic_evidence_diagnostic_correlation_test.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/task_operations_manual_recovery_test.go`

## Dependencies

None.

## Risks

- A provider time without an explicit timezone must not become a circuit
  deadline.
- A structured reset hint must never be overwritten by text parsing.
- Codex notice text must attach only to a matching generic failure from the
  same prompt turn.

## Parallelism

`sequential`

## Inputs

- `REQ-AGENTS-CODEX-USAGE-LIMIT-001` and all acceptance criteria.
- The codex usage-limit classification system design.

## Results

- Added strict reset-time parsing. Unzoned values do not produce deadlines;
  explicit UTC or numeric offsets are validated and used for conversion.
- Added same-prompt Codex usage-limit projection onto the generic ACP error.
- Passed provider identity through orchestrator diagnostic and terminal
  classification so matching usage-limit evidence can authorize fallback.
- Targeted tests are recorded in the PR fixup summary.
