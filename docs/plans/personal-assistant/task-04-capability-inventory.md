---
id: "04-capability-inventory"
title: "Native capability inventory"
status: done
wave: 3
depends_on: ["01-durable-intake","03-memory-context"]
plan: "plan.md"
requirements:
  - REQ-ORCHESTRATION-ASSISTANT-003
acceptance_criteria:
  - AC-ORCHESTRATION-ASSISTANT-003.1
  - AC-ORCHESTRATION-ASSISTANT-003.2
  - AC-ORCHESTRATION-ASSISTANT-003.3
  - AC-ORCHESTRATION-ASSISTANT-003.4
system_design:
  - ../../specs/orchestration/system-design/personal-assistant.md
---

# Task 04: Native capability inventory

## Inputs

Read the [requirements](../../specs/orchestration/requirements/personal-assistant.md) and [design](../../specs/orchestration/system-design/personal-assistant.md); legacy scenarios S14, S15, and [plan](plan.md), Backend 4 (directory). Read applicable AGENTS.md and implementation skills before editing. The [baseline experiments](experiments.md) are continuation evidence, not completed implementation.

## Acceptance

1. Runtime can list scoped native, integration, plugin and MCP capabilities with schemas, health, effects, generation/revision and bounded pagination.
2. Inventory excludes secret configuration and inaccessible resources; missing/locked/disconnected states are explicit.
3. Conversation plugin applicability is explicit and versioned; current task-only declarations and existing per-tool MCP naming remain compatible.

## Likely files

- apps/backend/internal/orchestration/runtime/capabilities.go, capabilities_test.go (new)
- apps/backend/internal/backendapp/adapters_assistant_capabilities.go (new); adapters_workspace_tasks.go
- apps/backend/internal/plugins/manifest/{manifest,validate,capability}.go and tests
- apps/backend/internal/mcp/plugintools/types.go; profile/profile.go; server registry
- apps/backend/cmd/agentctl/kandev.go (runOrchestrationCLI capability directory command); kandev_orchestration_test.go

## Implementation sequence

Define safe DTOs and adapter interfaces, then compose existing catalogs/health evidence in backendapp. Keep plugin operations individually typed and discoverable through the existing MCP registry; do not build a second invoke-plugin protocol. Test bounded output using configuration containing synthetic secrets and mismatched ownership. Inventory by itself enables no operation.

## Verification

Run each parenthesized command from the repository root. Use the repository Go/Node/pnpm toolchains. Scoped Go tests are intentional: the available make test target runs the entire backend. New test filters must select the named new tests; a no-tests-to-run result does not satisfy acceptance.

```sh
(cd apps/backend && go test -tags fts5 -count=1 ./internal/orchestration/... ./internal/backendapp -run 'TestAssistantCapabilities')
(cd apps/backend && go test -count=1 ./internal/mcp/profile ./internal/mcp/plugintools ./internal/plugins/manifest)
(cd apps/backend && go test -tags fts5 -count=1 ./internal/plugins ./internal/mcp/server -run 'Test.*(AgentTool|PluginTool|Profile|Conversation)')
```

## Dependencies and risks

Dependencies: `01-durable-intake`, `03-memory-context`, and delivery 01 for assistant admission. Execute in the primary session unless the user explicitly authorizes subagents.

Plugin manifests currently describe task surfaces; broadening them by default is a privilege expansion. Health evidence is session-specific and can become stale.

## Output

A secret-free, live directory integrated with Kandev's existing typed tool surfaces.

## Detailed implementation checklist

1. Add a bounded `CapabilityReader` port in Orchestration and typed entry/page DTOs:
   stable identity, kind, display label, parameter schema, effect classification,
   supported surfaces, health/reason, authorized resource scope and generation.
   Inventory composition remains in backendapp; no Office imports or config blobs.
2. Inventory existing profiles/workflows/executors, native task operations,
   integration health, plugin AgentTools and actually attached session MCP tools.
   Treat configured and attached as separate facts. Cap at 100 entries per page
   (default 50) and bind continuation to owner/workspace/filter/generation.
3. Build a response allowlist rather than serializing providers or arbitrary map
   metadata. Include synthetic secret-like fields in every adapter fixture and
   prove they do not appear in response, error or log output.
4. Reuse the existing MCP `SurfaceConversation`; add an explicit compatible
   conversation surface to plugin manifests/validation/registration. Old manifests
   retain their existing task/Office scope. Do not broaden every tool or add a
   generic plugin invocation endpoint. Preserve stable per-operation tool names.
5. Read current attachment evidence and health without starting provider calls.
   Invalidate directory generations on profile/plugin/integration changes and
   reauthorize on every page. Revoked entries disappear or show unavailable as
   appropriate; discovery never grants execution rights.
6. Expose human/runtime scoped routes and CLI output using the same DTOs and
   authorization. Add API docs with generic examples and mark authority support
   separately until task 05 proves an enforcement path.

## Detailed evidence map

| Criterion | Planned test | Required edge cases |
| --- | --- | --- |
| AC-ORCHESTRATION-ASSISTANT-003.1 | `TestAssistantCapabilitiesPaginationAndHealth` | Empty, >100, bad cursor, generation change, configured versus attached |
| AC-ORCHESTRATION-ASSISTANT-003.2 | `TestAssistantCapabilitiesScopeAndRedaction` | Wrong owner/workspace, config/env canaries, backend errors |
| AC-ORCHESTRATION-ASSISTANT-003.3 | Manifest/profile conversation-surface tests | Legacy manifest unchanged, explicit opt-in only, tool name compatibility |
| AC-ORCHESTRATION-ASSISTANT-003.4 | `TestAssistantCapabilitiesRevocation` | Disconnect/disable, stale session attachment, page two after revocation |

Use runtime `capabilities_test.go`, backendapp adapter tests and existing manifest,
plugintools and MCP profile tests. Add CLI handler coverage; no external account
or plugin installation is required for deterministic tests.

## Scope boundaries and delivery

Task 03 supplies scoped credential health. Invocation enforcement belongs to 05;
this work order must not advertise a write-capable executor as inspect-safe.
No plugin source is copied. If the manifest surface is externally versioned,
update compatibility tests and docs in the same commit. Record response sizes
and selected test counts along with the normal hook receipt.

## Parallelism

`sequential`

## Results

Implemented the bounded native capability port, owner/runtime routes and CLI.
The directory composes native operations, scoped profiles and workflows, executor
profiles, stored health for six integrations, explicit conversation plugin tools,
configured profile MCP servers and current-session attachment evidence. Cursors
bind owner/workspace/binding/filter/session/generation; defaults are 50, capped
at 100. Changed catalogs return 409. Stored metadata and structural schemas are
allowlisted; provider configuration, secrets, errors and transcripts are omitted.
Read-only hints grant no authority. The real executor record, current profile and
connection state must agree before attachment is reported.

Observed red/green: missing API routes returned 404; the CLI rejected the new
command; manifest and MCP validation rejected explicit conversation opt-in; an
attachment test exposed reliance on the legacy session execution field. All are
fixed. Conversation tools require manifest API version 2, use the existing
per-operation names and registry, and leave old task declarations unchanged.

Verification on the private v0.94.0 workbench:

- The exact work-order capability filter passed **7 tests** (two runtime, five
  backend adapters). Adjacent GitHub metadata and CLI tests also passed. The
  nine-test combined filter passed with the race detector before the final
  extra last-page assertion, which passed in the exact work-order rerun.
- The profile, plugintools and manifest packages passed in full. The work-order
  plugin/server filter passed with the race detector, including the two new
  versioned conversation-surface tests and existing registry behavior.
- Full orchestration and CLI packages passed with the race detector, including
  feature-off route checks. Scoped Go lint reported **zero issues**.
- Synthetic 100-entry directory measured **29,692 bytes**. Schema depth, property
  and byte bounds, 205-profile pagination, foreign scope, changing generations,
  disabled profiles, disconnects and secret-bearing metadata were exercised.
- Public-doc tests passed (61 tests, 48 pages); specification lint and diff checks
  passed. No schema migration or live provider request was needed.

Evidence logs use the `assistant-capabilities-*` prefix in the local implementation
record. Task 05 must still enforce invocation authority; directory presence and
historical health are not execution grants.
