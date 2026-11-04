---
status: current
system: agents
created: 2026-11-04
requirements:
  - REQ-AGENTS-MCP-PROFILE-CONTINUITY-001
owners:
  - kandev
---

# MCP tool profile continuity system design

## Application boundary

The restart investigation reproduced the missing canvas and title tools on v0.98.0.
The same sequences pass on baseline `5d6adcbca18` through the existing built-in package binding path.
This design records that path and the regression coverage it needs.
It adds no endpoint or new startup rejection rule.
See the [investigation](../../../plans/workspace-mcp-profile-continuity/investigation.md) for the process evidence and its limits.

## Ownership and contract

Agents owns the complete `mcpprofile.Context` sent to agentctl.
The context carries the surface, capabilities, providers, and optional built-in package binding.
The backend resolves this authority from trusted task and session state.
MCP tool arguments never supply it.

The [typed-profile decision](../../../decisions/2026-08-08-mcp-tool-profiles.md),
[built-in execution binding](builtin-mcp-management.md#execution-binding-and-compatibility), and
[resume identity](agent-resume-runtime-recovery.md#session-identity) contracts remain authoritative.

| Requirement | Design sections |
| --- | --- |
| REQ-AGENTS-MCP-PROFILE-CONTINUITY-001 | Profile source, Workspace promotion, Recovery and compatibility, Verification |

## Profile source

`Executor.resolveTaskSessionMCPProfile` resolves task capabilities.
`PrepareTaskSessionMCPProfile` adds the package binding through the production policy reader.
`applyResumeMCPSettings` places the complete value on `LaunchAgentRequest.McpProfile`.
The backend adapter carries that value into `lifecycle.LaunchRequest`.

Startup guidance and runtime registration must use this same resolved value.
A base `McpMode` cannot represent every optional capability.
Title ownership, canvas availability, question routing, and provider authorization retain their existing checks.
An absent title tool is correct for a settled title or a non-owner session.

## Workspace promotion

Workspace access can call `GetOrEnsureExecution` before an agent resume request arrives.
This creates a workspace-only helper using the task's base mode.
It has no launched provider process and does not yet have the later request's complete task profile.

`Manager.promoteWorkspaceExecution` calls `prepareBuiltinWorkspaceExecution` before configuring the agent command.
For an unused helper, that method sends the complete package-bearing profile through the existing authenticated
`PUT /api/v1/mcp/builtin-binding` route.
It preserves the existing owner validation, persistence marker, exact acknowledgement, and execution identity checks.

`process.Manager.PrepareBuiltinProfile` serializes initial binding against process configuration and startup through `startMu`.
`Server.BindInitialProfile` applies the complete profile before the provider process receives its MCP attachment.
The accepted binding includes optional canvas, title, and question capabilities.
The package manifest alone is not the capability resolver.

`PrepareBuiltinWorkspace` covers prepared-workspace starts that skip a second lifecycle launch.
The executor's existing-workspace path uses that operation before agent configuration.
Both entry paths must retain this ordering:

1. Resolve the full task/session profile.
2. Bind and confirm it on the unused helper.
3. Configure and start the provider process.

In v0.98.0, workspace promotion forwarded providers but omitted the full profile.
The basic helper catalog therefore survived into the resumed process.
A direct resume that created its own execution received the complete profile and succeeded.
Commit `a1013e89fc4`, from PR #4762, added the package binding path used by the successful current reproductions.

## Recovery and compatibility

A retained execution keeps its accepted profile and package binding.
Backend adoption, reconnect, and a later prompt do not grant permission to replace that binding.
The current promotion code verifies a retained binding through its existing path.
Retained legacy executions retain their existing compatibility behavior.

Existing binding errors must prevent a new process from starting with a partial catalog.
Recovery preserves the task session and saved provider conversation identity.
No new error response, fallback, migration, automatic restart, or package grant is proposed here.

The manifest-free promotion omission remains visible in a manually assembled lifecycle fixture.
Normal current task starts receive a package-bearing profile from the production policy reader.
That fixture alone does not justify a second profile endpoint.
A failure on current main with a correct helper catalog instead requires investigation of provider attachment or discovery.

## Verification

Permanent regression coverage enters the production resume assembly and workspace promotion path.
The tests live in `apps/backend/internal/backendapp/restart_mcp_catalog_test.go`.
`TestResumeWorkspaceHelperMCPToolInventory` starts with a helper created before resume.
It does not inject the expected profile directly into that helper's MCP server.
It initializes a real local MCP transport and compares canonical `tools/list` membership with a fresh start.

The inventory test covers a pending title's owner, a non-owner session, a settled title, and canvas enabled/disabled contexts.
`TestResumeWorkspaceHelperMCPGuidance` captures the prompt assembled by the production message-add path for the same four contexts.
It asserts canvas and title sections follow the effective profile, and every canvas or title tool name those sections advertise exists in the promoted helper's real `tools/list`.
It does not require every available tool to be advertised.
The same test retains package-selection agreement against the delivered built-in catalog.
Tool counts are evidence for one fixture, not a release-independent assertion.

The principal test detects removal of the existing pre-start binding call.
A temporary local mutation established that sensitivity, and the source was restored before the final checks.
A second mutation forced canvas and title guidance on regardless of profile and failed the eligibility cases.
Existing failure and retained-binding tests cover the current acknowledgement and adoption contracts.

The completed diagnostic used real native backend and agentctl processes with the mock ACP provider.
It preserved task/session and native conversation identity across backend restarts.
It covered message resume, workflow reuse, both workspace/resume orderings, and retained-process adoption on current main.
It did not run real Claude ToolSearch or establish the effect of Claude's `/model` command.

## Observability and delivery

Use existing launch and binding logs, stored attachment inventory, and safe process identity for diagnosis.
No new metric, poller, schema migration, or profile cache is required.

See the [implementation package](../../../plans/workspace-mcp-profile-continuity/plan.md) and its completed regression work order.
