---
id: "01-native-runtime"
title: "Native MiniMax runtime"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-MINIMAX-001
  - REQ-AGENTS-MINIMAX-002
  - REQ-AGENTS-MINIMAX-003
acceptance_criteria:
  - AC-AGENTS-MINIMAX-001.1
  - AC-AGENTS-MINIMAX-001.2
  - AC-AGENTS-MINIMAX-002.1
  - AC-AGENTS-MINIMAX-002.2
  - AC-AGENTS-MINIMAX-002.3
  - AC-AGENTS-MINIMAX-003.1
  - AC-AGENTS-MINIMAX-003.2
system_design:
  - ../../specs/agents/system-design/minimax-code.md
---

# Task 1: Native MiniMax runtime

## Summary

Native agent identity, installation/discovery, ACP and inference commands, native login command, passthrough ID conversion and runtime isolation.

## In scope

Native agent identity, installation/discovery, ACP and inference commands, native login command, passthrough ID conversion and runtime isolation.

## Out of scope

New transports, Office routing and cross-executor token copying.

## Acceptance

- The scoped native behavior satisfies all linked criteria.
- Task-defined tests pass with no fabricated live subscription evidence.
- Existing credential and permission ownership remains intact.

## Verification

```bash
(cd apps/backend && go test ./internal/agent/agents ./internal/agent/registry ./internal/agent/discovery)
(cd apps/backend && go test ./internal/agentctl/server/utility ./internal/agentctl/server/adapter/transport/acp -run "MiniMax|ConfigOptions|SessionModel|ProbeAllowlist")
```

## Files likely touched

- `apps/backend/internal/agentctl/server/utility/acp_executor.go`
- `apps/backend/internal/agent/agents/minimax_acp.go`
- `apps/backend/internal/agent/agents/minimax_acp_test.go`
- `apps/backend/internal/agent/registry/registry.go`
- `apps/backend/internal/agent/registry/minimax_test.go`
- `apps/backend/internal/agent/agents/logos/minimax_acp_*.svg`

## Dependencies

None.

## Risks

CLI/ACP syntax and credential-location differences; live OAuth unavailable.

## Parallelism

`sequential`

## Inputs

[Requirement](../../specs/agents/requirements/minimax-code.md),
[system design](../../specs/agents/system-design/minimax-code.md), existing native
ACP agents, shared protocol tests and current official upstream source.

## Results

Passed: Go agents, registry and discovery packages; the full utility and ACP transport packages, including permission, HTTP/SSE MCP, cancellation, resume and model selection tests. The probe allowlist completeness regression also passed. Registry tests first failed on missing native identity, then passed after registration. Discovery fixtures cover missing/broken/working executable. Command tests cover valid/invalid encoded models and resume.

Changed-package golangci-lint passed against base `17f0367d6dbbf043a8b1d3ab4a471632bb923b8f`. Empty native variants map to `#none-thinking`; malformed IDs remain explicit inputs.
