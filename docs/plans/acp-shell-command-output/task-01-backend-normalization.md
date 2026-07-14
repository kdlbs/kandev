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
cd apps/backend && go test ./internal/agentctl/server/adapter/transport/acp -run 'Test.*Shell|TestNormalizerResult|TestParseShellOutput'
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
- Existing `NormalizeToolResult`, `extractRawOutput`, and `parseShellOutput` patterns in `normalize.go`.
- Captured provider shapes in `acp-debug/codex-shell-output.jsonl`, `acp-debug/codex-shell-streaming-delayed-first.jsonl`, and `acp-debug/opencode-shell-output.jsonl`; Claude and Auggie shapes are pinned in the spec from adapter inspection.

## Output contract

Report the normalization API chosen, provider precedence, truncation behavior, tests run, files changed, blockers, and follow-up risks. Set this task to `done` and update `plan.md` only after targeted tests pass.
