---
id: "10-agent-routing-regression"
title: "Agent routing regression"
status: complete
wave: 10
depends_on: ["09-compact-mcp-guidance"]
plan: "plan.md"
spec: "../../specs/agents/requirements/agent-rich-output.md"
---

# Task 10: Agent routing regression

## Acceptance

1. Explicit chart, graph, plot, file-preview, KPI-card, and native metric-summary
   requests route directly to `show_rich_output_kandev` when suitable data is
   available.
2. Agent guidance forbids ASCII, SVG, HTML, and alternate-app presentation
   substitutes while retaining Markdown for small textual tables.
3. Task and Office prompts contain exact inline-chart, CSV-chart, and metric-group
   recipes in at most 750 runes; the MCP description remains at most 650 runes.
4. Fresh full-access, auto-approved Luna Worker subtasks pass inline bar, CSV
   time-series, file-preview, metric-summary, and Markdown-restraint scenarios
   in separate worktree executor environments without input requests.

## TDD order

1. Add failing prompt and tool-description assertions for direct routing,
   presentation-substitute rejection, the metric recipe, and existing budgets.
2. Replace weaker routing text with the compact direct-call contract in both
   injected prompts and the MCP tool description.
3. Run the focused tests and complete MCP package, then rebuild only the
   disposable seed instance.
4. Create fresh Luna Worker subtasks under a worktree-backed evaluation parent,
   inspect their canonical transcripts, and verify workspace isolation.

## Results

Completed on 2026-08-19.

- Focused tests failed before the guidance change and passed afterward.
- `go test ./internal/mcp/... -count=1`: passed.
- Task and Office rich-output lines are 737 runes; the tool description is 454
  runes. Existing 750/650-rune limits remain unchanged.
- Inline bar, CSV time series, file preview, and KPI summary each made exactly
  one valid `show_rich_output_kandev` call. The small comparison made none and
  returned a Markdown table.
- The CSV task created `reports/luna-latency.csv` and no HTML or SVG substitute.
  All five trials had zero permission or clarification requests.
- Every task used executor `exec-worktree` and received a distinct workspace.
  The isolated seed remained on ports `55081`/`55082`; the main instance on
  `9998` remained healthy and unchanged.
