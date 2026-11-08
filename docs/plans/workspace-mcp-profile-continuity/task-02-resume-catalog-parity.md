---
id: "02-resume-catalog-parity"
title: "Prove resumed tool and guidance parity"
status: done
wave: 2
depends_on:
  - 00-restart-reproduction
plan: "plan.md"
requirements:
  - REQ-AGENTS-MCP-PROFILE-CONTINUITY-001
acceptance_criteria:
  - AC-AGENTS-MCP-PROFILE-CONTINUITY-001.1
  - AC-AGENTS-MCP-PROFILE-CONTINUITY-001.2
  - AC-AGENTS-MCP-PROFILE-CONTINUITY-001.3
  - AC-AGENTS-MCP-PROFILE-CONTINUITY-001.4
  - AC-AGENTS-MCP-PROFILE-CONTINUITY-001.5
  - AC-AGENTS-MCP-PROFILE-CONTINUITY-001.6
system_design:
  - ../../specs/agents/system-design/mcp-profile-continuity.md
---

# Task 02: Prove resumed tool and guidance parity

## Summary

Protect the existing correction identified by Task 00.
Exercise normal profile resolution and workspace-helper promotion through local MCP transport.
No new profile endpoint or production behavior is planned.

## In scope

- Add `TestResumeWorkspaceHelperMCPToolInventory` and `TestResumeWorkspaceHelperMCPGuidance` in focused backend composition coverage.
- Resolve resume requests through the production task/session resolver, built-in policy reader, and backend adapter.
  Begin with a workspace-only helper created before resume.
  Let the existing lifecycle binding operation deliver its profile before provider configuration/start.
- Initialize a real local MCP client and call `tools/list`.
  Compare eligible canonical names with a fresh/direct-start control.
  Do not install the expected profile directly in the helper or force a nil manifest.
- Cover the pending title owner, a non-owner session, a settled title, and canvas enabled/disabled contexts.
  Reuse production guidance functions to assert optional-tool agreement.
- Reuse existing resolver tests for question/provider policy and existing promotion/adoption tests for binding order and retained authority.
  Add only the missing composition assertions.
- Use a temporary local mutation that omits pre-start binding to show the principal regression detects the historical loss.
  Restore the production source and record the exact failure assertion before final verification.
- Retain current failure semantics and conversation identity.
  Extend existing binding failure coverage only if the composition test exposes a gap.

## Out of scope

- No new endpoint, startup guard, fallback, UI, schema, or package grant.
- No real Claude credentials, provider discovery-cache assertion, or live data changes.
- No migration, automatic restart, or profile rewrite of a retained process.
- No unrelated test cleanup or broad verification audit.

## Acceptance

1. Production resume assembly binds the complete profile before a reused helper's process starts.
   Actual `tools/list` includes all seven canvas tools and the title tool for the eligible owner.
2. Non-owner and settled title sessions omit the title tool; disabled canvas contexts omit canvas tools.
   Generated optional-tool guidance matches the delivered profile.
3. The negative mutation causes a missing-tool assertion.
   The restored source passes the composition and existing targeted binding/failure/adoption tests.
4. No new endpoint or unconditional capability grant is introduced.
   Existing recovery preserves the task session and saved native conversation identity.

## Verification

Record the mutation failure, restore the source, and run from the repository root:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/backendapp -run '^TestResumeWorkspaceHelperMCP(ToolInventory|Guidance)$' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/orchestrator/executor ./internal/orchestrator ./internal/sysprompt -run 'MCPProfile|MCPMode|Canvas|TaskTitle|Builtin' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/agentctl/server/api ./internal/agentctl/server/process ./internal/mcp/server -run 'Builtin|BindInitialProfile' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run `bash scripts/lint-go-changed` with explicit changed Go file paths.
Run every new or changed test outside these patterns as well.
Record the expanded lint command, command results, matrix cases, and mutation failure in this work order.
Both named composition tests must appear in the first command's output before the work order is done.
Run the PR-documentation preflight with the complete changed-path and document map.

## Files likely touched

- `apps/backend/internal/backendapp/restart_mcp_catalog_test.go` (new composition coverage)
- Existing backend boot, profile resolver, MCP protocol, and lifecycle test fixtures, only as needed
- The requirement/design pair and this plan package for coverage and result updates

No production source changes are planned.
If a new test exposes a current defect, identify its boundary and revise the scope before selecting a repair.

## Dependencies

Task 00 is done and identifies the existing package binding path.
Task 01 is cancelled and is not an implementation dependency.

## Risks

- A directly injected profile bypasses the production handoff under test.
- Raw counts differ between monolithic and package catalogs.
- A missing settled/non-owner title tool is correct behavior.
- A composition test does not replace real Claude discovery evidence.

## Parallelism

`sequential`

## Inputs

- [Plan](plan.md) and [native-process evidence](investigation.md).
- [Requirements](../../specs/agents/requirements/mcp-profile-continuity.md).
- [System design](../../specs/agents/system-design/mcp-profile-continuity.md).
- Existing wiring, resolver, promotion, preparation, retained-binding, and startup-order tests listed in the investigation.

## Results

Done. `apps/backend/internal/backendapp/restart_mcp_catalog_test.go` adds the two
composition tests; no production source changed.

### Coverage

- `TestResumeWorkspaceHelperMCPToolInventory` resolves the profile through the
  production `executor.PrepareTaskSessionMCPProfile` and settings policy reader,
  then registers a workspace-only agentctl helper with the base task profile.
  The helper starts without canvas or title tools.
- Promotion runs through `lifecycleAdapter.LaunchAgent` and
  `promoteWorkspaceExecution`; the test initializes a real local MCP client and
  lists the promoted inventory.
- Matrix: pending title owner (title plus all seven canvas tools), non-owner
  session (no title), settled title (no title), and canvas disabled (no canvas).
- The promoted inventory matches a direct-start control built from the same
  resolved profile.
- After promotion, the test re-resolves the profile and confirms title and
  canvas eligibility. It parses the persisted binding and confirms the execution,
  session, and saved native conversation identities are unchanged.
- `TestResumeWorkspaceHelperMCPGuidance` captures the prompt assembled by the
  production message-add path (`RegisterMessageRoutes` to `wsAddMessage`) for the
  same four eligibility contexts. The message handler derives title and canvas
  decisions from task state and the production-resolved profile, and injects the
  prepared package manifest. The test asserts the canvas and title sections
  appear only when eligible, and that every canvas or title tool name advertised
  by those sections exists in the promoted helper's real `tools/list`. It does
  not require every available tool to be advertised. Package-selection checks
  remain for the default and task-enabled configuration package sets.
- AC 001.5 remains covered by the existing lifecycle promotion test: an
  unsupported helper returns an error before any agent command exists. The
  composition test exercises the matching exact acknowledgement on success.
- AC 001.6 remains covered by the existing
  `TestBuiltinPromotionVerifiesRetainedBindingWithoutReplacingIt` and the Task 00
  retained-process control. This work order did not change retained bindings.

### Mutation control

Removing the `m.prepareBuiltinWorkspaceExecution(...)` call from
`promoteWorkspaceExecution` made
`TestResumeWorkspaceHelperMCPToolInventory/pending_title_owner` fail at the
direct-start inventory comparison.

A second temporary mutation forced the canvas and title prompt sections on
regardless of the resolved profile. `TestResumeWorkspaceHelperMCPGuidance` failed
for the non-owner session, settled title, and canvas-disabled cases with the
expected eligibility assertions.

Both production changes were restored before the final checks.

### Verification

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/backendapp -run '^TestResumeWorkspaceHelperMCP(ToolInventory|Guidance)$' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/orchestrator/executor ./internal/orchestrator ./internal/sysprompt -run 'MCPProfile|MCPMode|Canvas|TaskTitle|Builtin' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/agentctl/server/api ./internal/agentctl/server/process ./internal/mcp/server -run 'Builtin|BindInitialProfile' -count=1)
(cd apps/backend && go build -trimpath -tags fts5 ./...)
bash scripts/lint-go-changed apps/backend/internal/backendapp/restart_mcp_catalog_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

All commands passed. The composition command passed four inventory subtests and
five guidance subtests in about 40 seconds under `-race`. The two targeted
package groups passed for all eight packages. `go build` returned no output.
`lint-go-changed` reported `0 issues`. Catalog validation reported 431 decisions
and 1914 specifications; specification lint passed; `git diff --check` was clean.
