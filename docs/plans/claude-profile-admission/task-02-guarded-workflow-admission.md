---
id: "02-guarded-workflow-admission"
title: "Use authorized authentication evidence for guarded workflow changes"
status: done
wave: 2
depends_on:
  - "01-scoped-auth-evidence"
plan: "plan.md"
requirements:
  - REQ-AGENTS-REMOTE-AUTH-ADMISSION-001
  - REQ-TASKS-CHANGE-WORKFLOW-002
  - REQ-EXECUTORS-OWNERSHIP-004
  - REQ-WORKSPACES-REPOSITORY-SECRETS-001
acceptance_criteria:
  - AC-AGENTS-REMOTE-AUTH-ADMISSION-001.1
  - AC-AGENTS-REMOTE-AUTH-ADMISSION-001.2
  - AC-AGENTS-REMOTE-AUTH-ADMISSION-001.3
  - AC-AGENTS-REMOTE-AUTH-ADMISSION-001.5
  - AC-TASKS-CHANGE-WORKFLOW-002.2
  - AC-TASKS-CHANGE-WORKFLOW-002.3
  - AC-TASKS-CHANGE-WORKFLOW-002.4
  - AC-TASKS-CHANGE-WORKFLOW-002.5
  - AC-EXECUTORS-OWNERSHIP-004.1
  - AC-WORKSPACES-REPOSITORY-SECRETS-001.4
  - AC-WORKSPACES-REPOSITORY-SECRETS-001.10
system_design:
  - ../../specs/agents/system-design/remote-auth-admission.md
  - ../../specs/tasks/system-design/change-workflow.md
  - ../../specs/executors/system-design/per-user-ownership.md
  - ../../specs/workspaces/system-design/repository-secrets.md
---

# Task 02: Guarded workflow integration

## Summary

Use Task 01's checker in the real backend validator after task/executor/actor authorization.
Prove that an authorized profile API key permits both preview and guarded apply without changing mappings or session policies.

## In scope

- Extend `taskAgentExecutorCompatibilityValidator` and its construction in `backendapp/services.go`.
  Pass the concrete profile into credential checks and inject a user-visible secret store.
- Prepare a read-only, authorized context in the task service before credential resolution.
  Reuse executor ownership and identity resolution, including current-user status checks and caller precedence.
  Check both the actual parent executor and selected executor profile.
  Missing identity must never reach the store as an unscoped internal caller.
- Cover `normalizeWorkflowChangeAgentOverrides` and `validateWorkflowAgentOverrideProfiles`.
  Keep defaults and overrides on the same validator, and preserve per-profile deduplication within one request.
- Preserve source workflow, step, timestamp CAS, atomic override replacement, WIP/deferred behavior, and all existing session guards.
  Preview remains read-only. Apply and launch repeat current checks.
- Use `workflow_agent_incompatible` and source-profile attribution for binding failure.
  Do not log raw store errors or include secret values/IDs in any failure surface.
- Verify Review and QA destinations with explicit new-session policy and their real profile mappings.
  Preview predicts independent conversations. Actual transitions create distinct sessions rather than reuse the source or each other.

## Out of scope

No owner-grant implementation, prepared-session attribution redesign, cold-resume repair, workflow schema change, session-policy rewrite, or model selection change.
Do not import lifecycle internals or call admission-write operations from preview.

## Acceptance

1. A real backend validator with a usable caller-owned API key permits preview and guarded apply for defaults and replacements on the task's SSH executor.
   Persisted mappings and independent Review/QA session policies remain correct.
2. Unauthorized task/profile/executor/actor choices fail before secret reads. Missing, revoked, or deleted bindings fail on apply even after successful preview.
   Stale source guards and rejected writes preserve task assignment/history and emit no successful move.
3. Existing local, dynamic, file-route, OAuth, no-methods, and workspace-scope controls pass. Cancellation produces neither a move nor successful compatibility.

## TDD and verification

Add `backendapp/claude_workflow_admission_test.go:TestClaudeWorkflowAdmission` using the real validator and scoped store.
Use temporary repositories and synthetic actors. Do not replace credential validation with an always-allow stub.
The valid profile-key case must fail on baseline with `workflow_agent_incompatible`.
The existing planning probe demonstrates this validator failure but is not a permanent regression test.

Add service-level `TestWorkflowCredentialAdmission` cases with a secret-read recorder.
Assert zero reads for unauthorized choices and missing identity, rather than asserting only a returned error.
Delete or revoke the selected binding between preview and apply. Retain an unrelated valid binding in mixed-state cases.
Verify the task-selected executor wins over a different workspace default.

Run from the repository root:

```bash
(cd apps/backend && go test -trimpath -race ./internal/backendapp ./internal/task/service -run '^(TestTaskAgentExecutorCompatibility.*|TestClaudeWorkflowAdmission.*|TestWorkflowCredentialAdmission.*|TestMoveTaskWithWorkflowChange.*|TestValidateWorkflowChange.*|TestExecutorOwnership.*)$' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/orchestrator -run '^(TestPreviewWorkflowMove.*|TestBuildWorkflowMovePreview_NewPolicyDoesNotReuseAnExistingSession)$' -count=1)
git diff --check
```

Run `bash scripts/lint-go-changed` with the actual changed Go paths from this work order.
Do not add a full-backend gate unless a focused failure establishes a reason.

## Files likely touched

- `apps/backend/internal/backendapp/task_agent_executor_compatibility.go`
- `apps/backend/internal/backendapp/task_agent_executor_compatibility_test.go`
- `apps/backend/internal/backendapp/claude_workflow_admission_test.go` (new)
- `apps/backend/internal/backendapp/services.go`
- `apps/backend/internal/task/service/workflow_change.go`
- `apps/backend/internal/task/service/workflow_agent_overrides.go`
- `apps/backend/internal/task/service/claude_workflow_admission_test.go` (new)
- `apps/backend/internal/orchestrator/workflow_move_preview_test.go` or a focused companion test file

## Dependencies

Task 01. The model prerequisite is independent.
Recheck #5184, #5176, #5179, and #4764 before changing actor or workflow integration.
Reconcile merged contracts in source. Do not use an unmerged PR's sharing behavior to make a test pass.

## Risks

The secret store treats missing identity as unscoped in internal contexts.
The explicit actor check and authorization-before-read tests are therefore essential.
Preview cannot guarantee future credentials or model availability.
Later queued/deferred execution must retain its existing actor and secret checks.

## Parallelism

`sequential`

## Inputs

- [Design: actor boundary and workflow integration](../../specs/agents/system-design/remote-auth-admission.md)
- [Existing workflow design](../../specs/tasks/system-design/change-workflow.md)
- `task/service/change_workflow_test.go` fixtures and `executor_ownership_test.go`
- `task/handlers/task_move_preview_handlers.go`
- `task/service/{executor_admission,executor_ownership,executor_mutation}.go`

## Results

Done.

- `backendapp/task_agent_executor_compatibility.go` now passes the concrete profile and the user-visible secret store into admission.
- `backendapp/services.go` wires `secrets.NewUserVisibleStore(repos.Secrets)` into the validator.
- `task/service/workflow_change.go` adds the read-only actor/executor authorization boundary. Remote executors require an established actor before any credential read; the resolved identity is carried in the context used by the validator.
- Both `normalizeWorkflowChangeAgentOverrides` and `validateWorkflowAgentOverrides` use that boundary. Mappings, CAS guards, preview behavior, and session policies are unchanged.
- `backendapp/claude_workflow_admission_test.go` exercises the real validator and scoped SQLite store: authorized profile key, executor method, missing actor, foreign owner, invalid OAuth beside a valid key, broken unrelated binding, empty configuration, malformed configuration, and the unchanged local contract.
- `task/service/claude_workflow_admission_test.go` proves unauthorized and actor-less SSH choices never call the compatibility validator.
- It also contains an in-process integration fixture that joins the real scoped `remoteauth` admission, task service, SSH executor/profile, and real persisted stores. It drives preview and guarded apply for destination defaults and an explicit replacement, checks persisted mappings, deletes the binding between preview and apply, checks a foreign actor, and verifies failed applies leave the source workflow/step and prior mappings unchanged with no `task.moved` event.
- The existing create-task compatibility test now establishes the synthetic actor required by the new remote admission boundary.

Verification:

```text
go test -trimpath -race ./internal/backendapp -run '^(TestTaskAgentExecutorCompatibility.*|TestClaudeWorkflowAdmission.*)$' -count=1
ok

go test -trimpath -race ./internal/task/service -run '^(TestWorkflowCredentialAdmission.*|TestMoveTaskWithWorkflowChange.*|TestValidateWorkflowChange.*)$' -count=1
ok

go test -trimpath -race ./internal/task/service -count=1
ok

bash scripts/lint-go-changed <changed-go-files>
0 issues.
```

Limits: the validator tests call the real admission and persistence code, but the guarded preview/apply HTTP boundary is not a browser/SSH end-to-end run. That evidence remains in Task 03.
