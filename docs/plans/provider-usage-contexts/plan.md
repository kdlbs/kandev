---
created: 2026-11-03
status: in-progress
requirements:
  - REQ-PLUGINS-AGENT-TOOLS-001
  - REQ-PLUGINS-AGENT-TOOLS-002
system_design:
  - ../../specs/plugins/system-design/agent-tool-contexts.md
legacy_specs: []
---
# Implementation plan: Provider usage across MCP contexts

## Overview

Expose cached provider percentages to approved automations, phase-2 coordinators,
and external MCP clients. Deliver the host contract before the plugin declares
new surfaces. Host implementation is complete; the expanded provider plugin
declaration is updated but still requires a host at or above 0.99.0 for
packaged evidence.

## Assumptions and scope

The user requested the three contexts and the current tool contract.
Design-time source inspection found that automation has exact saved grants,
coordinator bindings excluded plugins, and external plugin calls required a
task. Implementation preserves those permissions while adding the explicit
instance-read paths. Read the shared instance cache without adding
a scheduler decision, automatic provider switching, UI redesign, or telemetry
refresh. Configuration and unsupported managed-provider adapters are excluded.

The plugin system owns the durable declaration and invocation contract.
Office, coordinator, and integration designs supply adjacent identity and
authorization rules. No material question remains for this proposed design.

## Technical approach

Host worktree: `/root/.kandev/tasks/expand-provider-usag_hzb1fqle/kdlbs-kandev`,
branch `feature/expand-provider-usag-iw4`, based on `ea9a146ff9`.
The original investigation used host revision `b3b52e2b`.
Plugin worktree:
`/root/.kandev/tasks/https-github-com-kdl_60ctgpf8/kdlbs-kandev-plugin-provider-usage`.
Use its current feature branch and preserve PR36's existing UI changes.

1. Extend manifest scope, snapshot transport, surface filters, and trusted
   external dispatch. Preserve old declarations and retained agentctl compatibility.
2. Extend coordinator versioned bindings and action authorization. Keep core
   policy separate and support old bindings without implicit plugin authority.
3. Add plugin surfaces, author documentation, compatibility guidance, and
   packaged disposable-host MCP evidence. Do not migrate SDK imports unless
   the implementation proves a dependency on a new SDK symbol.

| Context | Identity and selection | Result | Evidence |
| --- | --- | --- | --- |
| Kanban / Office | Existing bound execution | Existing tool remains available | Host and plugin regression tests |
| Automation | Bound execution plus saved exact grant | Usage tool only after selection | Approved/denied/stale grant tests |
| Phase-2 coordinator | Current conversation plus v2 exact plugin binding | Bound instance reads | Binding and raw action tests |
| Phase-1 / v1 coordinator | Existing core binding | No implicit plugin grant | Compatibility test |
| External HTTP / SSE | Trusted endpoint and existing auth | Declared instance reads, empty task context | MCP transport integration test |
| Configuration | Existing profile | No usage plugin tool | Negative catalog test |
| Older host / agentctl | Legacy contracts | New contexts unsupported | Compatibility fixture |

## Tests

All named tests below were implemented and executed on 2026-11-03 unless a
row says otherwise.

| Criteria | Planned evidence |
| --- | --- |
| AC-PLUGINS-AGENT-TOOLS-001.5; 002.1, 002.4 | manifest/agent_tools_test.go: TestAgentToolInvocationScope; server/plugin_tools_test.go: TestPluginToolsContextEligibility |
| 002.2, 002.6 | handlers/plugin_tools_contexts_test.go: TestInstancePluginToolTrustedExternalContext and TestInstancePluginToolRejectsForgedIdentity |
| 002.3 | handlers/plugin_tools_automation_grant_test.go: TestProviderUsageRequiresAutomationGrant |
| 002.4, 002.6 | profile/coordinator_policy_test.go: TestCoordinatorPluginBindingCompatibility; handlers/plugin_tools_contexts_test.go: TestCoordinatorPluginToolAuthorization |
| 002.5 | plugin server/agent_tool_test.go: TestProviderUsageInstanceContextsUseCache |
| 002.6 | handlers/plugin_tools_contexts_test.go: TestInstancePluginToolRevokedAfterDiscovery |

Host paths above are relative to `apps/backend/internal/mcp`, except manifest
tests, which are under `apps/backend/internal/plugins`.
Each work order lists complete runnable package commands.

## End-to-end evidence

Add `internal/mcp/server/plugin_contexts_e2e_test.go` with
`TestProviderUsageMCPContexts`. Use a disposable backend and packaged fixture:
initialize, list, and call over external HTTP and SSE; check PAT rejection when
auth is enabled; check automation grants and a real coordinator conversation
binding. Assert tool names, empty args, percentages, freshness, and denial after
revocation. Synthetic telemetry proves routing, not provider-account access.
No rendered UI change or Playwright test is required.

## Work orders

- [x] [01: Instance tools through external MCP](task-01-instance-external-tools.md)
- [x] [02: Bound coordinator plugin reads](task-02-coordinator-bindings.md)
- [ ] [03: Provider declaration and packaged evidence](task-03-provider-adoption.md)

Order: 01 → 02 → 03. No delegation is authorized.

## Verification results

Design checks passed on 2026-11-03 in the original host worktree.
The package was subsequently copied into this task-managed host worktree and
implemented there. Design-time checks:


- `pnpm install --frozen-lockfile` from `apps`: completed; lockfile unchanged.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/list-docs.test.py`: 9 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/list-docs.py validate`: 428 decisions and 1,898 specifications validated.
- Local `.github/scripts/pr-docs.cjs` `validateCoverage` preflight: passed,
  documentation-only changes exempt. A separate cross-reference check confirmed
  all three work orders' requirement IDs, acceptance IDs, design ownership,
  plan membership, and one-to-three acceptance conditions.
- Current manifest input/output schemas validated the synthetic example;
  date-time format checks passed and an unexpected argument was rejected.
- `git diff --check`: passed. All package files remain unstaged and uncommitted.

Implementation checks passed on 2026-11-03 in this worktree. Review
remediation also closed three gaps: coordinator installation/policy authority
is rechecked inside the plugin dispatch lease, aggregate multi-plugin bindings
are capacity-bounded without invalidating core tools, and the external MCP
server subscribes to the lifecycle catalog fanout for live
`tools/list_changed` refreshes.

- `go test -trimpath ./internal/mcp/... ./internal/plugins/...
  ./internal/coordinator/... ./internal/automation/...
  ./internal/orchestrator/... ./internal/agent/runtime/lifecycle/...
  ./internal/agentctl/server/process ./internal/agentctl/types/...
  ./internal/backendapp/...` passed.
- `go test -trimpath -tags fts5 ./...` passed every package except an
  unrelated `internal/task/repository/sqlite` full-suite timeout; that package
  passed alone in 506s.
- `bash scripts/lint-go-changed` reported zero issues; `gofmt -l` is clean
  for changed Go paths.
- `python3 scripts/lint-spec-files.py --all`,
  `python3 scripts/list-docs.py validate`, `node scripts/validate-public-docs.mjs`,
  and `node --test scripts/validate-public-docs.test.mjs` passed.
- The expanded plugin's `go test -trimpath ./...` and
  `go test -trimpath -race ./server` passed. `make package` and packaged
  disposable-host evidence remain pending because the sibling SDK checkout
  predates host 0.99.0; do not produce the archive against that host.

The schema-validated [example](example-output.json) contains synthetic usage,
not measurements from the user's providers.

## Risks

- Existing coordinator conversations require reopen before gaining new grants.
- Host support must ship before installing the expanded plugin declaration.
- External access reveals shared instance usage, not per-user usage.
- Cached or partial data must not be interpreted as a guaranteed current quota.
- The standalone SDK migration in host main is separate from this manifest change.
