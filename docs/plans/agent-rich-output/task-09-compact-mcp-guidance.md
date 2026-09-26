---
id: "09-compact-mcp-guidance"
title: "Compact MCP guidance"
status: complete
wave: 9
depends_on: ["08-real-agent-acp-delivery"]
plan: "plan.md"
spec: "../../specs/agents/requirements/agent-rich-output.md"
---

# Task 09: Compact MCP guidance

## Acceptance

1. The rich-output tool description remains discoverable but is at most 650
   runes and does not duplicate full inline or CSV payloads.
2. Task and Office guidance each retain mandatory explicit-request routing and
   exactly one proven inline recipe and CSV recipe in at most 750 runes.
3. JSON Schema examples continue to expose complete inline and CSV calls for
   clients that surface them.
4. The shared MCP metadata budget and focused prompt tests prevent description,
   prompt-budget, and recipe-placement drift.

## TDD order

1. Add failing description and prompt budget/placement assertions.
2. Reduce the tool description to routing and host-ownership guidance.
3. Deduplicate both prompts while preserving the exact Luna-proven recipes.
4. Run the focused MCP server tests, then the complete MCP server package.

## Results

Completed on 2026-08-17.

- The MCP description is 439 runes, down from 845, under the 650-rune budget.
- `show_rich_output_kandev` is registered in the shared core-tool description
  budget introduced by PR #2716.
- Task and Office rich-output lines are each 740 runes, down from 1,205 and
  1,198, under the 750-rune budget.
- Both prompts still say agents must use rich output for explicit suitable
  requests, retain one exact inline and CSV block, and explain host ownership.
- Schema examples remain unchanged.
- `go test ./internal/mcp/... -count=1`: passed.
