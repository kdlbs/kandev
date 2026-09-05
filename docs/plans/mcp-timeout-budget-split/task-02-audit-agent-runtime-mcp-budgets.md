---
id: "02-audit-agent-runtime-mcp-budgets"
title: "Assert no agent runtime couples MCP startup and tool budgets"
status: done
wave: 2
depends_on: ["01-split-claude-mcp-budgets"]
plan: "plan.md"
spec: "../../specs/agents/requirements/mcp-timeout-budgets.md"
design: "../../specs/agents/system-design/mcp-timeout-budgets.md"
requirements:
  - REQ-AGENTS-MCP-TIMEOUT-BUDGETS-001
acceptance_criteria:
  - AC-AGENTS-MCP-TIMEOUT-BUDGETS-001.1
  - AC-AGENTS-MCP-TIMEOUT-BUDGETS-001.2
  - AC-AGENTS-MCP-TIMEOUT-BUDGETS-001.5
---

# Task 02: Assert no agent runtime couples MCP startup and tool budgets

## Outcome

The invariant task 01 establishes for the Claude runtime holds for every agent
runtime in the catalog, enforced by a test that a new agent cannot bypass by
omission.

## In scope

- Audit every `Runtime()` implementation in
  `apps/backend/internal/agent/agents/` for MCP time-budget environment values.
  At the time of writing, `claude_acp.go` is the only one that declares any;
  `hermes_acp.go` declares `HERMES_ACP_SKIP_CONFIGURED_MCP` for a related but
  distinct reason and is not in scope to change.
- Add a catalog-wide test that iterates the registered agents and asserts, for
  each runtime that declares `MCP_TIMEOUT`, that it is at most 60000 and that
  the runtime also declares `MCP_TOOL_TIMEOUT` when it needs a long tool
  budget.
- If the audit finds another runtime with the same coupling, correct it in this
  work order using the values its CLI documents.

## Out of scope

- Introducing MCP budgets for a runtime that declares none today.
- Changing `HERMES_ACP_SKIP_CONFIGURED_MCP`.
- Non-MCP environment defaults.

## Acceptance

- A catalog-level test enumerates agents through the existing catalog helper
  rather than a hardcoded list, so a newly added agent is covered without
  editing the test.
- The test fails if any runtime declares `MCP_TIMEOUT` above 60000.
- The audit result is recorded in this work order's results, including runtimes
  found to declare nothing.

## Verification

- `make -C apps/backend test` (new regression:
  `TestAgentRuntimesBoundMCPStartupBudget` in `internal/agent/agents`)
- `make -C apps/backend lint`

## Files likely touched

- `apps/backend/internal/agent/agents/helpers_catalog_test.go` or a new
  `mcp_budget_test.go` in the same package
- Any agent runtime file the audit finds coupled

## Dependencies

Task 01. This work order generalises the invariant that task 01 establishes.

## Inputs

- `apps/backend/internal/agent/agents/helpers_catalog_test.go` for the existing
  catalog enumeration pattern.
- Task 01's implementation of the per-runtime assertion.

## Output contract

Report the audited runtimes and what each declares, the new test output, any
runtime corrected, and any blocker. Mark this work order `done` and update
`plan.md` in the same conversation.

## Results

Implemented 2026-09-05.

- Audit: `grep -rn "MCP_TIMEOUT|MCP_TOOL_TIMEOUT" apps/backend/internal` (excl.
  generated/docs) returns exactly one non-doc hit, `claude_acp.go`. All other
  22 registered agents (`Registry.LoadDefaults()`) declare neither key.
  `hermes_acp.go` declares only the unrelated `HERMES_ACP_SKIP_CONFIGURED_MCP`
  flag — confirmed out of scope, not coupled to MCP_TIMEOUT semantics.
- Added `TestAgentRuntimesBoundMCPStartupBudget` in
  `internal/agent/registry/registry_test.go`. It iterates
  `Registry.LoadDefaults()` + `reg.List()` (the real, extensible agent
  catalog — `internal/agent/agents/helpers_catalog_test.go` has no such
  enumeration, it holds only three per-agent permission tests, so this test
  lives in `registry` instead per the corrected location in the plan) and
  fails if any runtime's `MCP_TIMEOUT` exceeds 60000. A newly registered
  agent is covered automatically since the test walks `LoadDefaults()`, not a
  hardcoded list.
- Confirmed the test fails for the right reason against the un-fixed
  `claude_acp.go` (temporarily isolated via a tagged `git stash` of just that
  file, restored immediately after): `claude-acp: MCP_TIMEOUT = 7200000, want
  <= 60000`. Passes after task 01's fix.
- No other runtime needed correction.
- `make -C apps/backend test` / `lint`: same result as task 01 (green for
  touched packages; pre-existing unrelated failures confirmed identical to
  merge-base).
