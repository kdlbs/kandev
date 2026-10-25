---
id: "01-enforce-launch-identity"
title: "Enforce exact profile model identity before inference"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-001
acceptance_criteria:
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.1
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.2
system_design:
  - ../../specs/agents/system-design/no-silent-model-fallback-01.md
---

# Task 01: Enforce Exact Profile Model Identity

## Historical scope

This work order is complete for the original PR implementation. The 2026-09-15
amendment in [Task 02](task-02-explicit-profile-policy.md) supersedes its implicit
strict predicate. Its transport, warning, and validated-reuse fixes remain.
The original scope below is historical, not the current compatibility contract.

## Change

Apply the shared start-model policy before the first inference on every
launch path: initial session start, context reset, and fresh ACP
workspace-rebind sessions. A concrete profile with a non-empty model,
`auto_fallback=false`, and no `fallback_model` must fail the launch before
a prompt or tool call when the executor does not advertise the configured
model; no speculative `SetModel` is sent for an unadvertised model.

Workflow step entry must also isolate stale runtime identity: a parked
session whose persisted provider model disagrees with the destination
profile's configured model policy is replaced with a fresh session before
any auto-start prompt runs. Explicit `fallback_model` and `auto_fallback`
profiles remain the only authorized deviation paths, each with one durable
warning.

## Verification

- `go test ./internal/agent/runtime/lifecycle` covers
  `TestApplyStartModelPolicy` behavior for advertised, fallback, and
  unauthorized substitution cases (`start_model.go`,
  `start_model_executor_authority_test.go`).
- `go test ./internal/orchestrator` covers session isolation on workflow
  entry (`event_handlers_workflow.go`,
  `event_handlers_workflow_profile_session_policy_resolution_test.go`,
  `event_handlers_workflow_profile_session_policy_test.go`).
- Turn/message attribution and pre-prompt warnings agree on the effective
  model; failures never expose credentials or provider secrets.
