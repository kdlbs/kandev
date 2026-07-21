---
id: "01-backend-normalization"
title: "Backend shell output normalization"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/acp-shell-command-output.md"
---

# Task 01: Backend Shell Output Normalization

## Acceptance

- The normalized shell contract distinguishes explicit exit `0`, nonzero exit, and unknown exit, and bounds each output field to 256 KiB with valid UTF-8 tail retention.
- Table-driven tests cover final Codex, Claude, OpenCode, and Auggie shapes, including precedence and malformed/absent exits.
- Append, cumulative replace, final replace, final-without-output, and truncation behavior cannot duplicate or silently discard retained output.

## Verification

```bash
make -C apps/backend fmt
(cd apps/backend && go test ./internal/agentctl/server/adapter/transport/acp -run 'Test.*Shell|TestNormalizerResult')
```

## Files likely touched

- `apps/backend/internal/agentctl/types/streams/tool_payload.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/normalize.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/normalize_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/shell_output.go` (new, if needed)
- `apps/backend/internal/agentctl/server/adapter/transport/acp/shell_output_test.go` (new, if needed)

## Dependencies

None.

## Inputs

- Spec sections `What`, `Data model`, and `Failure modes`.
- Existing `NormalizeToolResult` and `extractRawOutput` patterns in `normalize.go`, plus the checked-in provider cases in `shell_output_test.go`.
- Provider mappings and captured-shape summaries pinned in the feature spec; raw ACP captures were investigation artifacts, not repository fixtures.

## Output contract

Report the normalization API chosen, provider precedence, truncation behavior, tests run, files changed, blockers, and follow-up risks. Set this task to `done` and update `plan.md` only after targeted tests pass.

## Completion Report

- Added `NormalizeShellToolUpdate` and final-result normalization with independent stdout/stderr presence and truncation state, explicit-field precedence, optional exit codes, and 256 KiB UTF-8 tail retention.
- Covered Codex, Claude, OpenCode, and Auggie final shapes, malformed/absent exits, precedence, live append/replace behavior, partial stream merges, and truncation.
- Changed `shell_output.go`, `shell_output_test.go`, `normalize.go`, `normalize_test.go`, and the normalized stream payload type.
- Targeted ACP adapter tests, full backend tests, formatting, and backend lint passed. No blockers remain; new provider-specific shapes require new fixtures.
