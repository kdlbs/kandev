---
id: "04-workspace-identity"
title: "Preserve workspace identity during recovery"
status: complete
wave: 4
depends_on: ["03-restore-coordinator"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-004
acceptance_criteria:
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-004.1
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-004.2
system_design:
  - ../../specs/agents/system-design/harness-session-continuity.md
---

# Task 04: Preserve workspace identity during recovery

## Summary

Resolve directory compatibility using the original agent-visible CWD and retained native-state identity.

## In scope

- Persist original CWD at native creation. Keep it separate from source paths and mutable task names.
- Use stable executor mount paths where the existing environment contract supports them.
- Add deterministic harness fixtures for native load/resume with same CWD, changed CWD, missing state, and unsupported capability.
- Add an opt-in compatibility test for the shipped Codex, Claude Code, and OpenCode adapter versions.
- Preserve inherited-environment detachment on executor mismatch and the current completed-task follow-up mode.
- Retain resume_new_branch as native-only. Offer a separate continuation action after an incompatible rebind.

## Out of scope

Universal state export, credential copying, arbitrary cross-machine migration, and journal volume wiring.

## Acceptance

- A supported directory override uses native resume. An unsupported override returns a typed reason without replacing the token.
- Branch replacement alone never becomes a new native conversation.
- Codex, Claude Code, and OpenCode fixtures record the adapter/runtime version that supports each expectation.

## Verification

Run from the repository root. New test names describe required evidence, not existing passing tests.
Use TDD for implementation. Record the failing assertion before the implementation result.

```bash
(cd apps/backend && rtk go test -tags fts5 -race ./internal/agent/runtime/lifecycle ./internal/orchestrator/... ./internal/agentctl/server/adapter/transport/acp -count=1)
(cd apps/backend && rtk env KANDEV_TEST_REAL_AGENT_RESUME=1 go test -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run '^TestHarnessResumeCompatibility$' -count=1)
rtk make -C apps/backend lint
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
```

Target evidence in `apps/backend/internal/agent/runtime/lifecycle/session_workspace_identity_test.go`:

- `TestResumeChecksDirectoryAndNativeState`: `AC-AGENTS-HARNESS-SESSION-CONTINUITY-004.1`.
- `TestBranchRecoveryNeverFallsBackToNewConversation`: `AC-AGENTS-HARNESS-SESSION-CONTINUITY-004.2`.

The new opt-in test uses disposable directories and native sessions, not active user sessions.
It records installed versions and raw classified outcomes without prompt bodies or credentials.
It covers all three shipped adapters with configured test credentials.
A missing binary or authentication prerequisite leaves that adapter's live evidence incomplete.
Deterministic fixtures remain the default test path.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_workspace_rebind.go`
- `apps/backend/internal/orchestrator/executor/executor_execute_workspace_path_test.go`
- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session.go`
- `apps/backend/internal/agent/agents/`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/harness_resume_compatibility_test.go` (new).
- `apps/backend/internal/agent/runtime/lifecycle/session_workspace_identity_test.go` (new tests or extensions).

Regression evidence includes `executor_resume_inherited_env_test.go` and `TestCompletedTaskFollowUpAdmissionIsConversationalOnly`.
A stable workspace path cannot authorize an executor mismatch.

## Dependencies

[Task 03](task-03-restore-coordinator.md).

## Risks

- The same path on an empty executor is not the same native state. Protocol capability alone does not prove relocation support.

## Parallelism

`sequential`

The primary session owns integration. This work order does not authorize subagents.
Preserve existing user edits and unrelated changes.

## Inputs

- [Owned system design](../../specs/agents/system-design/harness-session-continuity.md).
- [Package manifest](plan.md), including shared regression gates and test prerequisites.
- Existing source and adjacent tests in the listed files.
- [Boundary decision](../../decisions/2026-09-10-durable-harness-session-boundaries.md).

## Results

Implemented workspace identity checks and candidate-session publication fencing in lifecycle rebind/resume paths. Regression tests cover changed roots, source restoration, inherited executor state, and branch replacement safety.
