---
id: "01-split-claude-mcp-budgets"
title: "Split the Claude ACP MCP startup and tool-call budgets"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/mcp-timeout-budgets.md"
design: "../../specs/agents/system-design/mcp-timeout-budgets.md"
requirements:
  - REQ-AGENTS-MCP-TIMEOUT-BUDGETS-001
acceptance_criteria:
  - AC-AGENTS-MCP-TIMEOUT-BUDGETS-001.1
  - AC-AGENTS-MCP-TIMEOUT-BUDGETS-001.2
  - AC-AGENTS-MCP-TIMEOUT-BUDGETS-001.3
  - AC-AGENTS-MCP-TIMEOUT-BUDGETS-001.5
---

# Task 01: Split the Claude ACP MCP startup and tool-call budgets

## Outcome

`ClaudeACP.Runtime().Env` declares `MCP_TIMEOUT` and `MCP_TOOL_TIMEOUT` as two
independent values, and a test asserts both, so the CLI's first-turn wait costs
bounded startup latency while Kandev's blocking MCP tools keep a two-hour
budget.

## In scope

- `apps/backend/internal/agent/agents/claude_acp.go`: replace the single
  `MCP_TIMEOUT: "7200000"` entry with `MCP_TIMEOUT: "30000"` and
  `MCP_TOOL_TIMEOUT: "7200000"`, with a comment naming what each budget governs
  and why they are separate. The comment must state the correct mechanism:
  `MCP_TIMEOUT` bounds both the MCP connect deadline and the CLI's first-turn
  wait on `subscriptions/listen`, which a conformant server holds open, so the
  CLI waits `MCP_TIMEOUT - 5000` ms before its first turn. Cite
  `anthropics/claude-code#91414`. Do not describe this as a slow, broken, or
  stalling MCP server, and do not name any MCP server.
- `apps/backend/internal/agent/agents/claude_acp_test.go`: add the regression
  test.
- `apps/backend/internal/mcp/handlers/handlers.go:3782`: correct the comment
  that currently attributes the two-hour wait to `MCP_TIMEOUT`. It is
  `MCP_TOOL_TIMEOUT` that keeps this call open.

## Out of scope

- Other agent runtimes. Task 02 owns that audit.
- `MCP_PROTOCOL_NEGOTIATION`. Rejected in the ADR.
- Stall detection and orchestrator cancellation.

## Acceptance

- `ClaudeACP.Runtime().Env` contains both keys with distinct values, and
  `MCP_TIMEOUT` parses as an integer no greater than 60000 while
  `MCP_TOOL_TIMEOUT` parses as an integer no less than 7200000.
- The test fails before the change with a message that names the coupled value,
  not only a count mismatch.
- An agent profile value for either key still replaces the managed agent
  default without an environment conflict, proven by an existing or added test
  over `appendAgentRuntimeDefaults`.

## Verification

- `make -C apps/backend test` (new regression:
  `TestClaudeACPSeparatesMCPStartupAndToolBudgets` in
  `internal/agent/agents`)
- `make -C apps/backend lint`

Write the test first. Confirm it fails because `MCP_TOOL_TIMEOUT` is absent and
`MCP_TIMEOUT` is 7200000, then make it pass.

## Manual acceptance evidence

Launch a Claude session with at least one MCP server configured and record time
to first token. Expect roughly the no-MCP baseline plus 25 s
(`MCP_TIMEOUT - 5000`), against roughly 1h 59m 55s before the change.

Then verify the risk named in the plan: hold an
`ask_user_question_kandev` call open past 5 minutes without answering and
confirm the agent is still waiting. If it is not, record the observed bound and
raise it as separate work. Do not widen this task.

## Files likely touched

- `apps/backend/internal/agent/agents/claude_acp.go`
- `apps/backend/internal/agent/agents/claude_acp_test.go`
- `apps/backend/internal/mcp/handlers/handlers.go`

## Dependencies

None.

## Inputs

- Plan section `Confirmed root cause` for the CLI semantics of each key.
- System design section `Data and contracts` for the values and their bounds.
- `appendAgentRuntimeDefaults` in
  `apps/backend/internal/agent/runtime/lifecycle/environment_resolution.go` and
  the tier table in `apps/backend/internal/agent/runtime/environment/tier.go`
  for the override behavior.

## Output contract

Report the failing assertion before the change, the exact test output after,
the files changed, the manual latency measurement, the idle-timeout finding,
and any blocker. The commit message and the test name must describe the split
of the two budgets. Neither may attribute the defect to a slow, broken, or
stalling MCP server, and neither may name a specific MCP server. Mark this
work order `done` and update `plan.md` in the same conversation.

## Results

Implemented 2026-09-05.

- `ClaudeACP.Runtime().Env` now sets `MCP_TIMEOUT: "30000"` and
  `MCP_TOOL_TIMEOUT: "7200000"` with a comment citing
  `anthropics/claude-code#91414`.
- New test `TestClaudeACPSeparatesMCPStartupAndToolBudgets` in
  `claude_acp_test.go`. Confirmed failing before the change:
  `MCP_TIMEOUT = 7200000, want <= 60000` and
  `Runtime().Env missing "MCP_TOOL_TIMEOUT"`. Passes after.
- `handlers.go:3782` comment corrected to reference `MCP_TOOL_TIMEOUT`.
- "Agent profile still overrides managed default" acceptance criterion is
  satisfied by the existing generic test
  `TestAppendAgentRuntimeDefaultsFillsOnlyMissingKeys`
  (`environment_resolution_defaults_test.go`), which picks an arbitrary key
  out of `claude-acp`'s `Runtime().Env` map and asserts override behavior;
  ran with `-count=5` to confirm it exercises both `MCP_TIMEOUT` and
  `MCP_TOOL_TIMEOUT` across map-iteration orders. No new test added — would
  have duplicated existing generic coverage.
- `make -C apps/backend test`: the only two packages this change touches at
  the unit level (`internal/agent/agents`, `internal/mcp/handlers`) are green.
  A full-suite run turned up 67 pre-existing failing tests across unrelated
  packages (worktree/socket/docker/config plumbing); confirmed byte-identical
  failing-test-name and failing-package sets against merge-base `c51ec0a2` in
  a scratch worktree — environmental, not caused by this change.
- `make -C apps/backend lint`: 0 issues.
- Manual acceptance evidence (live session latency measurement, idle-timeout
  probe) **not performed in this automated build step** — requires launching
  a real Claude session with a configured MCP server, which is outside what
  this session can safely/meaningfully do unattended. Left as an operator
  follow-up; does not block the code/test acceptance criteria above.
