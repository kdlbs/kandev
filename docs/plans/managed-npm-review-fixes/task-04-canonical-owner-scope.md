---
id: "04-canonical-owner-scope"
title: "Unify repair owners across interactive and background work"
status: done
wave: 4
depends_on:
  - "03-discovery-arguments"
plan: "plan.md"
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-007
acceptance_criteria:
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.3
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.5
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.6
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.14
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.18
system_design:
  - ../../specs/agents/system-design/managed-npm-repair-reuse.md
---

# Task 04: Unify repair owners across interactive and background work

## Summary

Resolve repair owner identity through one trusted policy.
Make disabled-authentication background launches compatible with the equivalent interactive owner without weakening enabled-authentication isolation.

## In scope

- Add one pure shared repair-owner policy and a narrow backend adapter for the live auth mode/default-user source.
- Resolve real actor, explicit workspace owner, then permitted single-user fallback in that order.
- Supply fallback only when the auth service explicitly reports disabled mode. Unknown/setup/enabled mode never implies that fallback.
- Hydrate actual workspace ownership in the session workspace-information path and remove the two conflicting precedence rules.
- Cover fresh tasks, saved resume, workflow auto-start, queued prompts, restart recovery, Office, and agent-wide boot/UI discovery.
- Preserve authorized profile scopes and explicit enabled-authentication system discovery. Keep old store records and workspace rows unchanged.

## Out of scope

- Workspace ownership migration, authorization-policy changes, a new feature flag, SQL changes, or merging profile and agent-wide result scopes.

## Acceptance

1. `TestManagedRuntimeOwnerPolicy` covers real actor, synthetic actor, unowned/owned workspace, disabled/setup/enabled/unknown mode, and unavailable service. Distinct authorized owners remain distinct.
2. `TestManagedRuntimeBackgroundOwnerReuse` uses actual task/resume/workspace-info producers and the default empty-owner row. Interactive and background disabled-mode launches send identical nonempty task scopes.
3. `TestManagedRuntimeProbeOwnerScopeModeParity` covers startup versus UI refresh in disabled mode, enabled system/user separation, and profile isolation. Backend wiring reads effective auth state rather than inferring it from a missing request identity.

## Verification

Run sequentially from the repository root after the required failing regression.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/managedruntime ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle ./internal/agent/hostutility ./internal/task/service ./internal/backendapp -run 'ManagedRuntimeOwner|ManagedRuntimeBackgroundOwner|ManagedRuntimeProbeOwnerScope|ManagedRuntimeOwnerModeWiring' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/orchestrator/executor ./internal/agent/hostutility -run 'ManagedRuntimeOwner|ManagedRuntimeBackgroundOwner|ManagedRuntimeProbeOwnerScope' -count=1)
```

## Files likely touched

- apps/backend/internal/agent/managedruntime/repair_owner.go and matching tests (new)
- apps/backend/internal/orchestrator/executor/executor_repair_owner.go and matching tests
- apps/backend/internal/orchestrator/executor/executor_execute.go, executor_resume.go, and executor_interaction.go callers
- apps/backend/internal/task/service/service_turns.go and service_turns_owner.go
- apps/backend/internal/agent/runtime/lifecycle/manager_execution.go and owner tests
- apps/backend/internal/agent/hostutility/manager.go and managed_runtime_owner_scope_test.go
- apps/backend/internal/backendapp/agents.go and composition wiring/tests

## Dependencies

Task 03.

## Risks

- Missing identity is not proof that authentication is disabled.
- `WorkspaceInfo.OwnerUserID` currently comes from the caller, despite its workspace-oriented use.
- A changed canonical owner can cause a normal cache miss. Never delete or migrate earlier signed generations to hide that miss.

## Parallelism

`sequential`

## Inputs

- Reuse design: Canonical application owner.
- `auth.Service.Mode`, `httpmw.SyntheticIdentity`, and the existing backend single-user identity adapter pattern.
- Review finding 2 and source evidence for empty-owner workspace creation.

## Results

Done on 2026-11-09.

- Added the shared repair-owner policy and wired live auth-mode fallback through orchestrator, lifecycle, and host utility.
- Hydrated durable workspace ownership in the workspace-info producer.
- Added policy, background parity, probe parity, and auth-mode wiring tests.
- Focused Go verification passed for all affected packages.
- Remediation: the policy now receives synthetic-identity status explicitly. Synthetic actors yield to a durable workspace owner, so an owned workspace has the same owner through interactive and background launches.
