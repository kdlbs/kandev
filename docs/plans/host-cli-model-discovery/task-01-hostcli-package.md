---
id: "01-hostcli-package"
title: "Host CLI metadata and process adapters"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-HOST-CLI-001
  - REQ-AGENTS-HOST-CLI-002
  - REQ-AGENTS-HOST-CLI-003
acceptance_criteria:
  - AC-AGENTS-HOST-CLI-001.2
  - AC-AGENTS-HOST-CLI-001.4
  - AC-AGENTS-HOST-CLI-002.1
  - AC-AGENTS-HOST-CLI-002.5
  - AC-AGENTS-HOST-CLI-003.3
system_design:
  - ../../specs/agents/system-design/host-cli-model-discovery.md
---

# Task 01: Host CLI metadata and process adapters

## Summary

Create the `internal/agent/hostcli` package that describes a vendor CLI,
parses its version, and lists Codex models through the app-server protocol.
Declare the `HostCLIAgent` capability on Claude, Codex, and the mock agent.
Nothing is wired into the controller yet.

Installing or updating a vendor CLI stays the existing agent install action
(see the plan's "Install and upgrade ownership" section); this package adds
no update mechanism, so it carries no `InstallMethod`, `UpdateCommand`, or
failure classifier.

## In scope

- `Spec`, `ModelSource`, `Model`.
- `ParseVersion`, `DetectVersion` with timeout.
- `ListCodexModels` JSON-RPC client over an injectable `Runner`.
- `agents.HostCLIAgent` and the three implementations; mock-agent `--version`
  and `app-server` behaviors.

## Out of scope

- Discovery registry, controller caches, HTTP routes, frontend, any CLI
  update or install mechanism.

## Acceptance

- Unit tests cover version parsing of real `claude --version` and
  `codex --version` output, a timeout, and a fake app-server exchange.
- `agents.NewClaudeACP().HostCLI()` and `agents.NewCodexACP().HostCLI()`
  return the compiled specs and the mock agent's spec points at its binary.

## Verification

```bash
(cd apps/backend && go test ./internal/agent/hostcli/... ./internal/agent/agents/... ./cmd/mock-agent/...)
(cd apps/backend && gofmt -l internal/agent/hostcli internal/agent/agents cmd/mock-agent)
```

## Files likely touched

- `apps/backend/internal/agent/hostcli/*.go`
- `apps/backend/internal/agent/agents/agent.go`
- `apps/backend/internal/agent/agents/claude_acp.go`
- `apps/backend/internal/agent/agents/codex_acp.go`
- `apps/backend/internal/agent/agents/mock.go`
- `apps/backend/cmd/mock-agent/main.go`
- `apps/backend/cmd/mock-agent/host_cli.go`

## Dependencies

None.

## Risks

- The Codex app-server response shape can change between CLI versions; the
  client reads only `id`, `displayName`, `description`, `isDefault`, `hidden`,
  and the reasoning-effort fields.

## Parallelism

`sequential`

## Inputs

- System design: Host CLI specification, Version detection, Model discovery,
  Update preview and job, Failure and recovery.
- Existing `agents.ManagedNPMRuntimeSpec` and `install_job.go` runner shapes.

## Results

Implemented as scoped: `hostcli.Spec`/`ModelSource`/`Model`, `ParseVersion`,
`DetectVersion`, and `ListCodexModels` in
`apps/backend/internal/agent/hostcli/`; `agents.HostCLIAgent` plus
`ClaudeACP.HostCLI()`, `CodexACP.HostCLI()`, `MockAgent.HostCLI()`; mock-agent
`--version` and `app-server` entry points in
`apps/backend/cmd/mock-agent/host_cli.go`. No install/update types were
added — the plan descoped the CLI update mechanism. Covered by
`hostcli_test.go`, `codex_models_test.go`, and `host_cli_test.go`.
