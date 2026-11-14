---
created: 2026-11-11
status: implemented
requirements:
  - REQ-AGENTS-BUILTIN-MCP-CALLER-AUTH-001
  - REQ-AGENTS-BUILTIN-MCP-CALLER-AUTH-002
  - REQ-AGENTS-BUILTIN-MCP-CALLER-AUTH-003
system_design:
  - ../../specs/agents/system-design/builtin-mcp-caller-auth.md
legacy_specs: []
---

# Implementation Plan: Built-in MCP caller authentication

## Overview

Each agentctl instance created with enforcement enabled gets its own MCP caller
credential. Kandev delivers it to the agent it launched, and the built-in MCP
endpoint rejects other callers. When enforcement is disabled, agentctl does not
issue a credential and Kandev keeps entries header-less. This stops blind or
stale-port calls, like the 2026-11-02 and 2026-11-06 incidents, from creating
clarifications in another session while preserving existing provider
configuration when the feature is off.

Order:

1. agentctl issues a credential only for an enforced instance and can enforce
   it.
2. agentctl builds credentialed entries when a credential exists and preserves
   header-less entries when it does not.
3. The backend sends the enforcement value through each lifecycle create path
   and delivers credentials on its own delivery paths when enforcement is on.
4. End-to-end and provider compatibility evidence.

Each step is safe to merge alone. The disabled path keeps the previous
header-less behavior. Enforcement stays off unless the backend asks for it.

## Scope

### In scope

- Credential generation for enforced instances, storage, and authenticated
  return to the backend.
- Enforcement middleware on the core and package `/mcp`, `/sse`, and `/message`
  routes.
- Credentialed built-in entries when enforcement is enabled, with header-less
  entries and policy recognition when it is disabled.
- The `builtinMcpCallerAuth` runtime release toggle, on in E2E only.
- Mock-agent support, E2E coverage, provider evidence, and log redaction.

### Out of scope

- Promoting enforcement to the production default. This needs a separate
  change after the evidence in Task 04.
- Rejecting new clarification requests on sessions without an active turn.
- The `atomica-code` 180-second tool-call limit.
- Isolation from processes of the same OS user that copy a credential on
  purpose.
- The backend's external `/mcp` endpoint and the OpenCode plugin host.

## Technical approach

### agentctl credential and enforcement (Task 01)

- `internal/agentctl/server/instance/instance.go`:
  `CreateRequest.BuiltinMCPAuthEnforced` (`builtin_mcp_auth_enforced`).
- `internal/agentctl/server/instance/manager.go`: generate the credential only
  when the create request enables enforcement, before `startInstance` builds
  the handler. Store it, with the enforcement
  value, on `config.InstanceConfig` (`BuiltinMCPCredential`,
  `BuiltinMCPAuthEnforced`).
- `internal/agentctl/server/api/server.go` and `builtin_mcp_auth.go`: the
  control-authenticated instance read `GET /api/v1/mcp/builtin-credential`
  (`types.BuiltinMCPCredentialResponse`). Create, get, and list responses never
  carry the credential.
- `internal/agentctl/server/api/builtin_mcp.go`: split the route set into
  `builtinMCPRoutePaths(cfg)` and the `/health` exemption. Add the
  `builtinMCPCallerAuth` middleware with rate-limited rejection logs. Install it
  in `server.go` next to `instanceAuth`.

### Credentialed entries in agentctl (Task 02)

- `internal/agentctl/server/config/builtin_mcp.go` and `config.go`:
  `BuiltinMCPServers` and `injectKandevMcpServerAtHost` take the credential and
  set `Headers`.
- `internal/agentctl/server/api/agent.go` (`injectKandevMcpServers`,
  `sessionMCPServers`) and `api/builtin_mcp.go` (`builtinSessionMCPServers`):
  copy headers.
- `internal/agentctl/server/process/builtin_mcp_preparation.go`,
  `managed_tool_policy.go`, `manager_permission_policy.go`,
  `builtin_mcp_permission.go`: exact recognition including headers.
- `cmd/mock-agent`: forward ACP `headers` to the mcp-go SSE client with
  `transport.WithHeaders`.
- Redact `Headers` wherever agentctl logs MCP server entries.

### Backend toggle and delivery (Task 03)

- Toggle: `profiles.yaml` (`KANDEV_FEATURES_BUILTIN_MCP_CALLER_AUTH`, prod and
  dev `"false"`, e2e `"true"`), `internal/common/config/config.go`
  (`FeaturesConfig.BuiltinMCPCallerAuth`), and one registration in
  `internal/runtimeflags/registry.go`.
- `internal/agent/runtime/lifecycle`: receive the restart-scoped flag at
  startup and put it on each executor create request. Each create request
  builder passes it to agentctl; `ControlClient.CreateInstance` serializes the
  caller's explicit value. `Client.BuiltinMCPCredential` reads the credential
  through the instance route. A plain 404 identifies an older agentctl, while a
  recycled-port 404 is an error.
- `internal/agent/runtime/lifecycle`: read the credential on demand from the
  execution's agentctl client for each artifact build, so adopted and replaced
  instances need no extra state. Use it in
  `builtin_mcp_passthrough.go`, `builtin_mcp_cursor.go`,
  `cursor_plugin_mcp_reconcile.go`, `project_mcp_files.go`, and
  `passthrough_remote_mcp.go`.
- `cmd/mock-agent/tui_mcp.go`: forward `headers` from `--mcp-config` with
  `transport.WithHTTPHeaders`.

### Provider compatibility matrix

| Provider | Transport | Carrier | Intended behavior | Evidence (Task 04) | If unsupported |
| --- | --- | --- | --- | --- | --- |
| Claude ACP | ACP HTTP | `headers` → `--mcp-config` | Connects with header | ACP debug probe + live session | Keep toggle off |
| Codex ACP | ACP HTTP | `headers` | Connects with header | ACP debug probe | Keep toggle off |
| Codex app-server | Native | `http_headers` | Connects with header | Unit test + probe | Keep toggle off |
| Copilot, Gemini, Auggie, Grok, Amp, Muse, Devin, OpenCode ACP | ACP HTTP/SSE | `headers` | Connects with header | ACP debug probe where installed; otherwise "not verified" | Keep toggle off |
| Cursor ACP and TUI | `.cursor/mcp.json` | `headers` | Connects; re-approval is automatic | `cursor-agent mcp list-tools` probe | Keep toggle off |
| Pi | `.pi/mcp.json` | `headers` | Connects with header | Project-file probe | Keep toggle off |
| Custom ACP (`atomica-code`) | ACP HTTP | `headers` | Connects with header | ACP debug probe on installed 1.0.102 | Keep toggle off; report to agent owner |
| Mock agent | ACP SSE, TUI HTTP | mcp-go headers | Connects with header | E2E | Not applicable |

Shared implementation does not prove provider support. Only rows with recorded
evidence count as supported.

## Tests

| AC | Evidence |
| --- | --- |
| 001.1, 001.2, 001.6, 003.5 | `internal/agentctl/server/api/builtin_mcp_auth_test.go`: `TestBuiltinMCPCallerAuth_RejectsMissingCredential`, `_RejectsForeignCredential`, `_HealthStaysOpen`, `_NoOAuthMetadata` |
| 001.2 | `internal/agentctl/server/instance/manager_mcp_credential_test.go`: `TestCreateInstance_PortReuseIssuesNewCredential` |
| 001.3 | `internal/agentctl/server/api/builtin_mcp_auth_test.go`: `TestBuiltinMCPCallerAuth_OwnCredentialServesToolsAndSSE` |
| 001.4 | `builtin_mcp_auth_test.go`: `TestBuiltinMCPCallerAuth_ControlCredentialRejected`, `TestInstanceAuth_MCPCredentialRejectedOnControlRoutes` |
| 001.5, 003.1 | `manager_mcp_credential_test.go`: `TestCreateInstance_CredentialIsRandomAndScoped`, `TestCreateInstance_DisabledBuiltinAuthKeepsEntriesHeaderless` |
| 002.1 | `config/builtin_mcp_test.go`: `TestBuiltinMCPServers_CarryCredentialHeader`; `api/builtin_mcp_session_test.go`; `acp/mcp_test.go`; `codexappserver/builtin_mcp_test.go`; `lifecycle/builtin_mcp_passthrough_test.go`; `lifecycle/builtin_mcp_cursor_test.go` |
| 002.2 | `acp/mcp_test.go`: `TestLoadAndResumeForwardBuiltinHeaders`; `lifecycle`: `TestAdoptedExecutionReusesInstanceCredential` |
| 002.3 | `process/manager_permission_policy_test.go`, `process/builtin_mcp_permission_test.go`, `process/managed_tool_policy_test.go`: exact-header acceptance and foreign-header rejection |
| 002.4 | Web E2E `e2e/tests/chat/clarification*.spec.ts` with the E2E profile |
| 003.1, 003.2 | `builtin_mcp_auth_test.go`: `TestBuiltinMCPCallerAuth_DisabledPassesThrough`; `runtime/agentctl`: `TestCreateInstanceRequestCarriesExplicitEnforcementFlag`, `TestClientBuiltinMCPCredential`; lifecycle manager and executor request propagation tests |
| 003.3 | `builtin_mcp_auth_test.go`: `TestBuiltinMCPCallerAuth_RejectionLogIsRateLimited` |
| 003.4 | `builtin_mcp_auth_test.go` and lifecycle log tests assert the credential is absent from captured logs and responses |

## E2E tests

| Flow | AC | File | Project |
| --- | --- | --- | --- |
| Ask, answer, and resume through the mock agent under enforcement | 002.4, 001.3 | `apps/web/e2e/tests/chat/clarification.spec.ts` | `chromium` |
| Lifecycle and turn shadow with enforcement | 002.4 | `apps/web/e2e/tests/chat/clarification-lifecycle-turn-shadow.spec.ts` | `chromium` |

## Work orders

- [x] [Task 01: agentctl credential and enforcement](task-01-agentctl-credential-enforcement.md)
- [x] [Task 02: Credentialed built-in entries in agentctl](task-02-agentctl-credentialed-entries.md)
- [x] [Task 03: Backend toggle and delivery](task-03-backend-toggle-delivery.md)
- [x] [Task 04: E2E and provider compatibility evidence](task-04-e2e-provider-evidence.md)

## Verification results

2026-11-12:

- Task 01: `go test -tags fts5 ./internal/agentctl/server/api/... ./internal/agentctl/server/instance/...` passed.
- Tasks 02 and 03: `go test -tags fts5` over the config, api, instance, process,
  acp, codexappserver, mock-agent, runtimeflags, common/config, profiles,
  runtime/agentctl, lifecycle, mcpconfig, and backendapp packages passed.
  `bash scripts/lint-go-changed <changed Go files>` reported 0 issues. The web
  feature-flag contract test passed.
- Task 04: the three clarification E2E specs passed in the `chromium` project
  under the E2E profile, where enforcement is on (36 passed). Provider evidence
  is in [compatibility-evidence.md](compatibility-evidence.md): Claude ACP
  0.106.0, OpenCode 1.19.5, and `atomica-code` 1.0.105 send the header. Other
  providers are not verified.

Design changes made during implementation, reflected in the system design:

- The backend reads the credential on demand through the control-authenticated
  instance route `GET /api/v1/mcp/builtin-credential`. Create, get, and list
  responses and the passthrough and project-file snapshots do not carry it.
- The lifecycle manager receives the restart-required toggle at startup and
  explicitly passes it through every executor create request. The control
  client serializes that request value without process-global state.
- `acpdbg mcp-probe` now advertises a random header and reports
  `header_observed` and `header_missing`.

2026-11-14, PR #5294 review remediation:

- Credential generation is gated by the create request's enforcement value;
  disabled instances and their generated entries remain header-less.
- The backend sends the restart-scoped setting through the lifecycle manager
  and every executor create-request builder, without process-global state.
- The backend treats a recycled-port 404 as a credential-read error while
  preserving plain 404 compatibility with older agentctl versions.
- Focused instance, process-policy, runtime/agentctl, and lifecycle tests passed.
  `bash scripts/lint-go-changed <changed Go files>` reported 0 issues.
- `python3 scripts/list-docs.py validate` and
  `python3 scripts/lint-spec-files.py --all` passed.

## Risks

- A provider may drop ACP `headers`. That provider loses Kandev tools under
  enforcement. Task 04 records this, and the toggle stays off in production.
  With the toggle disabled, the instance gets no credential and entries remain
  header-less.
- The credential appears in argv and in workspace files (`.cursor/mcp.json`,
  `.pi/mcp.json`). It expires with the instance but can be committed by
  accident.
- Cursor fingerprint approval changes for each new enforced instance. When the
  toggle is disabled, Kandev preserves the previous header-less fingerprint.
- Policy recognition with exact headers can disable automatic approval if any
  construction path misses the header. Task 02 tests cover every builder.
- Mixed versions degrade to unauthenticated behavior, so a stale remote
  agentctl does not enforce. A `recycled_port: true` 404 must fail instead of
  being treated as an older agentctl.

## Resolved questions

- Rollout: the user accepted the release toggle, off in production and dev and
  on in E2E, with promotion as a separate decision after provider evidence.
- Workspace files: the user accepted writing the short-lived credential to
  `.cursor/mcp.json` and `.pi/mcp.json`.
- Per-agent exceptions: not needed now. Add one only if a provider in use fails
  the probe.
