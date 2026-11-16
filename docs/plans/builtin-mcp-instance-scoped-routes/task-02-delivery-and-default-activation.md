---
id: "02-delivery-and-default-activation"
title: "Migrate delivery and activate scoped instances"
status: complete
wave: 2
depends_on:
  - "01-scoped-helper-routes"
plan: "plan.md"
requirements:
  - REQ-AGENTS-MCP-INSTANCE-002
  - REQ-AGENTS-MCP-INSTANCE-003
  - REQ-AGENTS-MCP-INSTANCE-004
acceptance_criteria:
  - AC-AGENTS-MCP-INSTANCE-002.1
  - AC-AGENTS-MCP-INSTANCE-002.2
  - AC-AGENTS-MCP-INSTANCE-002.3
  - AC-AGENTS-MCP-INSTANCE-002.4
  - AC-AGENTS-MCP-INSTANCE-003.1
  - AC-AGENTS-MCP-INSTANCE-003.2
  - AC-AGENTS-MCP-INSTANCE-003.4
  - AC-AGENTS-MCP-INSTANCE-003.5
  - AC-AGENTS-MCP-INSTANCE-004.1
  - AC-AGENTS-MCP-INSTANCE-004.2
  - AC-AGENTS-MCP-INSTANCE-004.3
system_design:
  - ../../specs/agents/system-design/builtin-mcp-instance-scoped-routes.md
---

# Task 02: Migrate delivery and activate scoped instances

## Summary

Carry effective mode from create responses and retained `InstanceInfo` into
each selected client. Migrate native artifacts, then activate scoped routing
on new managed creates.

## In scope

- Propagate effective mode through every fresh/recovered executor client,
  including standalone/worktree, Docker, Remote Docker, Kubernetes, SSH,
  Sprites, and plugin remote paths.
- Preserve full create results in direct SSH/Sprites HTTP paths. Keep mode when
  a retained `InstanceInfo` becomes a synthesized get-or-create response.
- Use `InstanceInfo` as the sole retained-adoption mode source. Kubernetes
  reattachment needs a read through its existing temporary control forward.
  Mode-read errors must not trigger helper recreation.
- Route `agentMCPURL`, package expansion, legacy/bound Cursor composition, and
  native fingerprints through the common builder with an explicit selected client.
- Update `builtinPassthroughEndpoint` to accept only the canonical path for that
  client's mode/ID. Preserve host/port validation and rejection of user info,
  queries, fragments, and malformed endpoints.
- Exercise actual Claude/OpenCode file contents, Codex argument values, Pi
  files, and both Cursor modes. Preserve third-party definitions and ownership.
- Use `placement.client` for local and remote project-file delivery during
  restart. Verify the helper target/read-back URL against that same client.
- Retain the selected SSH snapshot/client lease through preparation. Do not
  reacquire a different client and combine its mode with an old snapshot.
- Set true on every backend fresh/recreation create builder, including SSH's
  direct HTTP path and plugin remote creation. Continue to use the echoed mode.
- Add `TestBuiltinMCPInstanceDelivery`, `TestBuiltinMCPInstanceDefaultCreate`,
  `TestBuiltinMCPInstanceEndpointValidation`, and `TestBuiltinMCPInstanceStagedDelivery`.
  Include `TestBuiltinMCPInstanceRecovery` for retained bare/scoped modes and
  failed `InstanceInfo` reads that do not trigger recreation.
  Test both mode directions with staged/published clients whose IDs match or differ.
- Inventory `MCPServerURL(`, `/mcp`, `/sse`, `/message`, and direct loopback
  formatters. Classify every remaining production occurrence as routing,
  generic transport, external endpoint, or third-party data. No native builder
  can retain independent URL assembly.

## Out of scope

Provider headers, new provider protocols, operator UI, live-instance migration,
package grant changes, a flag, and browser/publication work. Native strategies
keep their existing file formats and argument/quoting contracts.
Do not add helper-ID carriers or metadata, rotate IDs on replacement, change
owner matching, or modify session-association DTOs. Same-execution ID reuse stays valid.

## Acceptance

1. Every provider/channel row in the plan produces the exact canonical URL in
   both modes, for legacy and bound catalogs. Existing approval tests still pass.
2. Every new managed create requests scoping. Retained `InstanceInfo` and old
   peer responses yield the correct mode without native-session calls or
   identity changes. Known scoped errors never trigger bare retries or writes.
3. A staged replacement emits its own ID/mode through local/remote project
   files and SSH preparation. Failure preserves the retained client's accepted
   state and rolls back configuration/evidence through existing owners.

## Verification

Run from the repository root, after behavioral RED for each new path.

```bash
(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/lifecycle -run 'TestBuiltinMCPInstance|TestManagerReopensStoppedPluginExecutorByAttachingRetainedEnvironment|TestKubernetesCreateInstanceRehandshakesAfterMainContainerRestart|TestBuiltinPassthrough|TestMaterializeRuntimeProjectMCP|TestRemoteProjectMCP|TestKubernetesRestart|TestProjectMCPDeliveryHandoff|Test.*PassthroughMCP|Test.*Cursor.*MCP|Test.*ComposeEffectiveMCP|Test.*MCP.*Restart' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agentctl/server/api -run '^TestBuiltinMCPInstance' -count=1)
(cd apps/backend && go test -trimpath ./internal/agent/mcpconfig ./internal/agentctl/server/process -count=1)
git ls-files --modified --others --exclude-standard -z -- 'apps/backend/**/*.go' | xargs -0 -r bash scripts/lint-go-changed
git diff --check
```

Use the shared `TestBuiltinMCPInstance` prefix for new carrier/delivery cases.
Record all provider/runtime subtests and the direct-formatter inventory in
Results. A test that asserts only a prefix does not prove an exact endpoint.

## Files likely touched

Under `apps/backend/internal/agent/runtime/lifecycle/`:

- Client construction in `executor_standalone.go`, `executor_standalone_reconnect.go`,
  `container.go`, `executor_docker.go`, `executor_docker_reconnect_owner.go`,
  `executor_remote_docker_recovery.go`, and their affected tests.
- SSH/Sprites creation and recovery in `executor_ssh_operations.go`,
  `executor_ssh_recovery.go`, `executor_sprites_operations.go`,
  `executor_sprites_recovery.go`, and their client constructors.
- `executor_kubernetes_reconnect.go`, `executor_kubernetes_shared_control.go`,
  `executor_plugin.go`, and `executor_plugin_recovery.go`, limited to mode propagation.
- `mcp_host.go`, `builtin_mcp_passthrough.go`, `builtin_mcp_cursor.go`.
- `builtin_mcp_*_test.go` and new `builtin_mcp_instance_delivery_test.go`.
- New `builtin_mcp_instance_recovery_test.go` for retained mode and unchanged ownership.
- `cursor_plugin_mcp_reconcile.go`, `cursor_plugin_mcp.go`, and related tests.
- `manager_passthrough.go`, `passthrough_remote_mcp.go`, and the SSH runner's
  snapshot/client lease owner.
- `manager_project_mcp.go`, `project_mcp_files.go`, `project_mcp_delivery.go`,
  `manager_kubernetes_refresh.go`, and restart/placement tests.
- Create builders in `executor_standalone.go`, `container.go`,
  `executor_docker.go`, `executor_ssh_operations.go`,
  `executor_sprites_operations.go`, and `executor_plugin.go`.

Also inspect `apps/backend/internal/agent/mcpconfig/*strategy*.go` and its tests.
The strategies normally accept URLs unchanged. Change production strategy code
only if exact artifact tests expose an actual assumption about bare paths.

## Dependencies

[Task 01](task-01-scoped-helper-routes.md) supplies the helper's scoped service,
negotiation DTOs/client reader, exact policy comparisons, and common builder.

## Risks

- The current parser rejects scoped core paths even when the server accepts them.
- SSH currently returns only the port, and synthesized responses can lose mode.
- A mode-read failure must not change a healthy helper's ownership or lifecycle.
- Cursor's legacy composer and package composer can diverge without separate tests.
- An acquired execution client can still be old while replacement placement is ready.
- Native files can look correct while read-back evidence certifies another helper.
- Default activation cannot precede writer migration, even inside this order.

## Parallelism

`sequential`

## Inputs

- Design: Delivery and exact-URL policy, Per-instance negotiation, Recovery and identity.
- Existing executor `InstanceInfo` reads and client-construction patterns.
- Existing `builtin_mcp_passthrough_test.go`, `manager_project_mcp_test.go`,
  `project_mcp_files_test.go`, `mcp_restart_client_lease_test.go`,
  `builtin_mcp_permission_test.go`, and passthrough strategy tests.
- [Per-CLI injection ADR](../../decisions/0014-passthrough-mcp-injection-strategies.md)
  and [Pi ownership ADR](../../decisions/2026-10-08-pi-project-mcp-ownership.md).
- Backend/agentctl engineering guidance and `/tdd`.

## Results

Completed. Lifecycle clients now carry the effective `ScopedMCPRoutes` mode
from accepted create responses or authenticated retained `InstanceInfo`. New
managed create builders request scoped routes, and all native delivery paths
use the selected helper client for their canonical endpoint. This includes
ACP/package expansion, exact passthrough validation, Claude/OpenCode files,
Codex arguments, Pi project files, both Cursor composers/fingerprints, staged
project delivery, and SSH snapshots. Kubernetes retained adoption reads mode
through its existing control forward. Failed mode reads do not trigger helper
recreation. Staged preparation rolls back prior configuration/evidence on
failure and retains its selected client. Existing helper identity allocation,
same-execution reuse, external routes, and session-association carriers remain
unchanged.

RED evidence: focused `TestBuiltinMCPInstance` delivery regressions exposed
rollback-state and no-port behavior that could not be validated by endpoint
prefix checks alone. GREEN evidence: exact URLs, both route modes, old-peer
fallback, retained recovery, endpoint rejection, placement-client selection,
SSH snapshot mode, and rollback assertions pass. Existing Cursor lifecycle
fixtures were updated to provide the legacy helper client expected by the
new selected-client contract.

Verification passed on 2026-11-16:

- `(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/lifecycle -run 'TestBuiltinMCPInstance|TestManagerReopensStoppedPluginExecutorByAttachingRetainedEnvironment|TestKubernetesCreateInstanceRehandshakesAfterMainContainerRestart|TestBuiltinPassthrough|TestMaterializeRuntimeProjectMCP|TestRemoteProjectMCP|TestKubernetesRestart|TestProjectMCPDeliveryHandoff|Test.*PassthroughMCP|Test.*Cursor.*MCP|Test.*ComposeEffectiveMCP|Test.*MCP.*Restart' -count=1)`
- `(cd apps/backend && go test -trimpath -race ./internal/agentctl/server/api -run '^TestBuiltinMCPInstance' -count=1)`
- `(cd apps/backend && go test -trimpath ./internal/agent/mcpconfig ./internal/agentctl/server/process -count=1)`
- Task 01 helper package matrix, including `internal/common/mcpurl`, agentctl API/config/instance/process, MCP server, and runtime agentctl: passed.
- `make -C apps/backend build`: passed for backend, agentctl, mock-agent, and debug binaries. Darwin codesign warnings were non-fatal.
- Changed-Go `scripts/lint-go-changed`: passed, 0 issues.
- `git diff --check`: passed.

Review remediation also passed: retained plugin attachment reads InstanceInfo
with the selected runtime identity and request credential, including repeated
reopen from a DB-shaped row with no transient token. Kubernetes reconnect uses
recorded main-container restart evidence before reading retained mode, restores
the nonce handshake for an absent helper, and still fails closed on 500 or
malformed InstanceInfo. Stale native MCP requests remain 404/no-dispatch while
their warning log is capped at five per minute per server.

Review follow-up passed on 2026-11-16. Plugin startup recovery now reads
InstanceInfo with the inventory's selected runtime identity. SSH resume
discovers an omitted control port from the bounded session log before its
authenticated InstanceInfo read and retains the returned route mode. Kubernetes
client construction requires an explicit route-mode value, and profile
preparation materializes package URLs for the selected helper identity.

- `(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/lifecycle -run '^(TestPluginExecutorRestartRecovery|TestSSHResumeReadsMissingControlPortFromExactSessionLog|TestSSHRecoveryReadsLegacyControlPortFromExactSessionLog|TestKubernetesRefreshRemoteInstanceReattachesDeadLocalClientWithoutHandshake|TestKubernetesCreateInstanceRehandshakesAfterMainContainerRestart|TestManagerReopensStoppedPluginExecutorByAttachingRetainedEnvironment)$' -count=1)`: passed.
- `(cd apps/backend && go test -trimpath -race ./internal/agentctl/server/process -run '^(TestBuiltinPreparationUsesScopedHelperInstanceURLs|TestBuiltinPreparationAllowsUnusedHelperWithDefaultCommand)$' -count=1)`: passed.
- `(cd apps/backend && go test -trimpath -race -timeout=15m ./internal/agent/runtime/lifecycle ./internal/agentctl/server/process)`: passed.
- `make -C apps/backend build`: passed for backend, agentctl, mock-agent, and debug binaries.
- `git diff --name-only -z -- '*.go' | xargs -0 -r bash scripts/lint-go-changed`: passed, 0 issues.
- `git diff --check`: passed.

The remaining installed-provider question/approval release gates are recorded
under Task 03 and do not change this backend delivery order's completion.
