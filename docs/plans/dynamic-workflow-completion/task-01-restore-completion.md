---
id: "01-restore-completion"
title: "Restore Dynamic workflow completion"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.1
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.5
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.11
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.14
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.1
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.3
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.7
system_design:
  - ../../specs/tasks/system-design/workflow-profile-readiness.md
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
---

# Task 01: Restore Dynamic workflow completion

## Summary

A Dynamic logical profile must survive workflow reuse validation.
A rejected completion preflight must leave the owned completed conversation ready for another prompt.

## In scope

- Implement the profile validation and completion recovery sections of the design.
- Add all five planned Go tests and the browser scenario from the plan.
- Preserve concrete exact-model checks and existing cancellation, queue, and stale-event guards.

## Out of scope

Provider selection, fallback policy, migrations, flag rollout, UI composition, and broad refactoring.

## Acceptance

1. Default and explicit Dynamic reuse pass preflight and preparation without resolving a virtual CLI or replacing logical identity.
2. Rejected completion preflight preserves the workflow source, publishes readiness, and accepts a follow-up without cancelling or repeating entry actions.
3. Concrete model checks, active manual moves, newer turns, terminal sessions, and duplicate completion retain their existing safety behavior.

## Implementation sequence

1. Mark this order `in_progress`.
2. Add the planned Go and browser regressions before production changes.
3. Run the focused tests. Record expected failures for virtual resolution and stranded `RUNNING` state.
4. Handle only wrapped `ErrVirtualProfile` as no logical exact-model drift.
5. Restore readiness only for the owned completed-turn preflight failure path.
6. Run all commands below. Record results and synchronize the plan.

Use typed error matching. Do not infer profile kind from names or swallow unrelated errors.
At least one regression must use the real settings store and concrete resolver.
Test READY followed by complete-stream delivery, as observed in the report.
Assert no destination launch or committed transition after preflight rejection.
For race cases, coordinate channels or existing hooks instead of elapsed sleeps.

## Verification

Run each command from the repository root. Install frontend dependencies once if this worktree lacks them.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test ./internal/orchestrator -run '^TestWorkflow(Dynamic|ProfileLookupFailure|CredentialPreflightFailure|Completion|PreflightFailure)' -count=1)
(cd apps/backend && go test -race ./internal/orchestrator -run 'TestWorkflow(Dynamic|ProfileLookupFailure|CredentialPreflightFailure|Completion|PreflightFailure)|TestPrepareWorkflowStepSession|TestPreflightWorkflowStepCredentials|TestExactModelWorkflowStartPolicy|TestHandleAgentReady|TestHandleCompleteStreamEvent' -count=1)
(cd apps/backend && go test ./internal/agent/runtime/lifecycle -run 'Test.*Profile' -count=1)
make -C apps/backend lint
(cd apps/web && pnpm e2e:run --project chromium tests/workflow/dynamic-workflow-completion.spec.ts tests/workflow/workflow-session-targeting.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use the managed E2E runner for fresh builds and cleanup. Confirm both spec files have discovered tests.
The browser scenario uses the existing mock candidate and real backend lifecycle, without external provider credentials.

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/agent/runtime/errors.go` (runtime facade error alias)
- `apps/backend/internal/orchestrator/event_handlers_workflow_dynamic_completion_test.go` (new)
- `apps/web/e2e/tests/workflow/dynamic-workflow-completion.spec.ts` (new)
- `docs/plans/dynamic-workflow-completion/plan.md`
- This work order

## Dependencies

None. Execute this order sequentially in the primary session.

## Risks

The shared transition helper has callers that still own active turns.
Do not broaden readiness settlement beyond a completed turn or weaken the concrete resolver.
Avoid recursive acquisition of the session guard in the READY path.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/workflow-profile-session-lifecycle.md), criteria 001.1, 001.5, 001.11, and 001.14.
- [Profile readiness design](../../specs/tasks/system-design/workflow-profile-readiness.md).
- [Dynamic requirements](../../specs/agents/requirements/dynamic-agent-routing.md).
- `event_handlers_workflow_profile_session_policy_test.go` and `event_handlers_workflow_profile_session_policy_resolution_test.go`.
- `event_handlers_agent.go`: `handleAgentReady` and its ownership guards.
- `event_handlers_streaming.go`: waiting-state publication and complete-stream processing.
- `internal/agent/runtime/lifecycle/profile_resolver.go`: typed virtual-profile rejection.
- E2E patterns: Dynamic settings profile creation and workflow session targeting.
- [Plan evidence and browser scenario](plan.md).

## Results

Completed on 2026-09-25. The regression suite reproduced the virtual-profile preflight failure and the stranded `RUNNING` session before the production change. After the fix, targeted orchestrator tests, the race-enabled control suite, lifecycle resolver tests, backend lint, and all six selected Chromium tests passed. The managed E2E run built the backend and Vite assets. Both selected specs were discovered and executed. Spec validation, spec lint, formatting, and diff checks passed. Frontend dependencies were already installed, so the frozen install step was skipped.

Review follow-up completed on 2026-09-25. The profile-lookup recovery check is now separate from a credential-preflight integration regression that delivers `handleAgentReady` followed by complete-stream, verifies the completed turn and unchanged primary source session, then sends a follow-up through queue admission and confirms exactly one prompt and new turn with no stranded queue entry. The destination-only credential rejection uses a parked target with a profile lacking the source session's valid credential. The browser scenario now polls the exact persisted session identity, primary flag, and `WAITING_FOR_INPUT` state after the second response marker. The runtime facade re-exports `ErrVirtualProfile` so the orchestrator respects the lifecycle import boundary. Focused orchestrator tests and race-enabled recovery coverage passed, architecture lint passed, backend lint passed, and the managed Chromium run passed all six selected tests after rebuilding the backend and web assets. The E2E file's ESLint and Prettier checks passed.
