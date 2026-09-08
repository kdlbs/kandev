---
id: "08-real-agent-acp-delivery"
title: "Real-agent ACP delivery"
status: complete
wave: 8
depends_on: ["07-chart-axes-legends"]
plan: "plan.md"
spec: "../../specs/agents/requirements/agent-rich-output.md"
---

# Task 08: Real-agent ACP delivery

## Acceptance

1. A captured Codex ACP MCP frame with `kind: execute` and
   `_meta.is_mcp_tool_call: true` persists as a generic tool call named for the
   actual MCP tool, not as an empty shell execution.
2. Initial arguments and the completed MCP `CallToolResult` survive live
   delivery and replay in the provider-neutral normalized payload.
3. The frontend tolerates both the corrected normalized payload and historic
   Codex wrappers without rendering pending, failed, or malformed data.
4. Task and Office guidance provides an exact compact inline chart recipe,
   preserves the CSV recipe, and states that Kandev owns axes and legends.
5. A fresh isolated Luna profile uses full-access mode and automatic approval;
   real-agent trials create valid inline bar, CSV/time-series, and restrained
   Markdown results without permission prompts.
6. Existing desktop and mobile rich-output flows remain behaviorally equal and
   pass their focused production-build E2E coverage. This repair changes
   transport normalization and guidance, not mobile composition.

## Root causes

- Codex ACP wraps MCP calls in the standard ACP `execute` category and carries
  `{server, tool, arguments}` in `rawInput`. The adapter normalized solely from
  the category, losing the actual MCP identity and treating the presentation as
  shell execution. Its completion wrapper also nested the MCP result beneath
  `rawOutput.result`.
- The rich-output schema was strict, but the injected agent prompt did not show
  a canonical inline chart object. Luna selected the correct tool yet invented
  `data`, `categories`, and axis fields until the prompt supplied the exact
  `labels` plus `series[].values` recipe.
- Earlier isolated sessions inherited a read-only Luna profile. Enabling
  automatic approval later could not rewrite already-running session policy.

## TDD order

1. Add a backend regression using the captured initial and completed Codex ACP
   envelopes; observe shell classification and lost result as RED.
2. Add frontend parser regressions for the real nested `raw_input.arguments`
   and `rawOutput.result.structuredContent` shapes; observe RED.
3. Add prompt/schema contract assertions for the exact inline chart recipe and
   host-owned axes/legend guidance; observe RED.
4. Implement Codex-dialect MCP recognition and result unwrapping, frontend
   compatibility parsing, and synchronized compact authoring guidance.
5. Rebuild only the isolated test environment, configure a new full-access,
   auto-approved Luna profile, and run fresh real-agent scenarios.
6. Run focused Go/Vitest checks plus existing desktop and mobile rich-output
   E2E. Record commands, counts, agent outcomes, and isolated-instance details.

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_codex.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_tools.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/codex_mcp_tool_test.go`
- `apps/backend/internal/mcp/server/rich_output.go`
- `apps/backend/internal/mcp/server/rich_output_test.go`
- `apps/backend/internal/mcp/server/sysprompt_sync_test.go`
- `apps/backend/config/prompts/kandev-context.md`
- `apps/backend/config/prompts/office-context.md`
- `apps/web/components/task/chat/messages/kandev/parse.ts`
- `apps/web/components/task/chat/messages/kandev/parse.test.ts`
- `apps/web/components/task/chat/messages/kandev/kandev-tool-message.test.tsx`
- `docs/specs/agents/requirements/agent-rich-output.md`
- `docs/plans/agent-rich-output/plan.md`

## Risks

- The recognizer must require Codex's explicit MCP marker and a complete
  envelope so ordinary `execute` calls remain shell cards.
- Incremental completion updates must not merge the provider wrapper back into
  the canonical arguments.
- Result unwrapping must preserve standard MCP text, structured content, and
  error information without trusting arbitrary nested objects as rich output.
- Prompt recipes must be exact enough to use while remaining compact enough
  not to burden every task turn.
- Isolated profile changes affect only newly created sessions. Evidence must
  come from fresh tasks after the profile update.

## Results

Completed on 2026-08-16.

- Codex's explicitly marked initial MCP `execute` frame now establishes
  trusted, in-memory MCP provenance. The terminal frame can omit the marker,
  as observed in the live ACP capture, without merging `rawInput` back into
  canonical arguments or retaining the outer `{error, result}` wrapper.
- Historic Codex wrappers remain readable in the frontend while new messages
  persist the provider-neutral tool name, arguments, and `CallToolResult`.
- Task and Office guidance plus MCP schema examples include complete inline and
  CSV chart recipes. The focused tool description routes explicit requests;
  the prompts include the required block summary and host-owned axes/legends.
- The isolated profile `Luna Worker (full access)` used model
  `gpt-5.6-luna`, mode `agent-full-access`, and `auto_approve: true`.
- Real Luna trials produced: one valid inline bar chart on its only rich-output
  attempt; one Markdown table with no rich-output call; and one CSV line chart
  on its only rich-output attempt after the recipe repair. The final CSV trial
  had zero failed calls, zero input requests, and zero permission messages.
- The final CSV tool message contained no `raw_input` field and no outer Codex
  result envelope. Browser verification found one visible line chart, five
  x-axis ticks, five y-axis ticks, one legend, and zero console errors before
  and after reload.
- `go test ./internal/agentctl/... ./internal/mcp/...`: passed.
- Focused rich-output/settings Vitest run: 10 files and 86 tests passed.
- `pnpm run typecheck` and `pnpm run i18n:check`: passed; existing real-locale
  parity findings remained advisory and pseudo stayed synchronized.
- Desktop Chromium rich-output E2E: 2 tests passed. Mobile Chrome rich-output
  E2E: 1 test passed.
