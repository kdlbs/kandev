---
id: "03-provider-adoption"
title: "Provider declaration and packaged evidence"
status: in-progress
wave: 3
depends_on:
  - 01-instance-external-tools
  - 02-coordinator-bindings
plan: plan.md
requirements:
  - REQ-PLUGINS-AGENT-TOOLS-002
acceptance_criteria:
  - AC-PLUGINS-AGENT-TOOLS-002.1
  - AC-PLUGINS-AGENT-TOOLS-002.3
  - AC-PLUGINS-AGENT-TOOLS-002.5
  - AC-PLUGINS-AGENT-TOOLS-002.6
system_design:
  - ../../specs/plugins/system-design/agent-tool-contexts.md
---
# Task 03: Provider declaration and packaged evidence

## Summary

Declare the usage tool in the requested contexts and document the host
dependency. Prove packaged MCP access without polling providers on invocation.

## In scope

Plugin manifest instance scope and five surfaces; current contract examples;
automation grant regression; disposable host with the packaged plugin and
synthetic telemetry.

## Out of scope

Telemetry refresh, automatic provider routing, AI-credit schema expansion,
SDK migration without a demonstrated need, and PR36 UI changes.

## Acceptance

- Kanban, Office, approved automation, bound phase-2 coordinator, and external
  MCP can call the same tool with `{}`; unapproved contexts fail.
- Calls use the shared cache and preserve percentages, freshness, empty external
  workspace identity, output schema, and no provider/credential I/O.
- The package works on the first supporting host version, which is documented;
  retain existing branch changes and do not release against an unsupported host.

## Verification

Run plugin commands from the plugin worktree listed in the plan:

```bash
go test -trimpath ./...
go test -trimpath -race ./server
make package
git diff --check
```

Run host commands from the host root:

```bash
(cd apps/backend && go test -trimpath ./internal/mcp/... -run 'TestProviderUsageMCPContexts|TestProviderUsageRequiresAutomationGrant' -count=1)
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.py validate
git diff --check
```

Implement `TestProviderUsageInstanceContextsUseCache` in plugin
`server/agent_tool_test.go`.
Inspect initialize/list/call success and denied calls over HTTP and SSE on the
disposable host. Record commands, host version, artifact, transport, and results.
Keep real provider access explicitly unverified when using synthetic telemetry.

## Files likely touched

Plugin: `manifest.yaml`, `README.md`, `server/agent_tool_test.go`.
Host: `apps/backend/internal/mcp/server/plugin_contexts_e2e_test.go`,
`apps/backend/internal/mcp/handlers/plugin_tools_automation_test.go`,
and package fixture files.

## Dependencies

Tasks 01 and 02; a supporting host build. Use the existing SDK contract unless
a new required symbol makes a compatibility-pin update necessary.

## Risks

Plugin upgrades may invalidate existing automation grants.
Do not overwrite the sibling SDK checkout used by PR36.

## Parallelism

sequential

## Inputs

Current plugin tool declaration, cache projection tests, packaging workflow,
and the host context integration fixture.

## Results

Implemented on 2026-11-03.

- The provider plugin declares `invocation_scope: instance` on Kanban, Office,
  automation, coordinator, and external surfaces and sets
  `min_kandev_version: "0.99.0"`.
- `TestProviderUsageInstanceContextsUseCache` covers all five surfaces, empty
  external workspace identity, unchanged percentages and freshness, and no
  provider I/O.
- Host automation coverage: `TestProviderUsageRequiresAutomationGrant`.
- `go test -trimpath ./...` passed in the plugin worktree. `make package` and
  packaged disposable-host evidence remain pending until the sibling SDK
  checkout includes host 0.99.0; the archive must not be produced against the
  older host.
