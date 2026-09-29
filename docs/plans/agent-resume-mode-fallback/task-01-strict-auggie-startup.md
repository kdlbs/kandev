---
id: "01-strict-auggie-startup"
title: "Strict Auggie task startup and resume"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-EXPLICIT-RESUME-SETTINGS-001
acceptance_criteria:
  - AC-AGENTS-EXPLICIT-RESUME-SETTINGS-001.1
  - AC-AGENTS-EXPLICIT-RESUME-SETTINGS-001.6
system_design:
  - ../../specs/agents/system-design/explicit-resume-settings.md
---

# Task 01: Strict Auggie task startup and resume

## Summary

Enforce effective selected settings on Auggie task start and ordinary/automatic
resume before readiness or prompt dispatch. Preserve normal permission-mode
confirmation and other providers' profile policy.

## In scope

Trace task start, manual ordinary resume, automatic resume, and startup settings
through orchestrator/executor into lifecycle. Carry task/provider scope explicitly;
do not identify Office merely by absence/presence of a task ID. Construct exact
attempt model policy for this scope without saving it to the profile. Apply the
winning runtime/workflow selection once. Empty selections retain provider state.
Preserve typed/sanitized causes for recovery presentation.

## Out of scope

Explicit omission recovery, new UI, Office or other-provider policy changes,
and any ACP acknowledgment/confirmation change.

## Acceptance

- Missing/rejected model and refused/clamped/unconfirmed mode fail before prompt
  or readiness, even with configured fallback; unset selections remain usable.
- Existing other-provider/Office policy and saved profiles remain unchanged.
- Cancellation and transport failures preserve error identity and resume token.

## Verification

Add the Task 01 tests named in [plan.md](plan.md#tests) using TDD, including
both start and resume entry points. From repository root:

```bash
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/orchestrator/executor ./internal/agentctl/server/adapter/transport/acp)
```

`make test PKG=...` is not scoped: the Makefile ignores PKG.

## Files likely touched

- `apps/backend/internal/orchestrator/session_launch.go` and `session_launch_test.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go` and `executor_resume.go`
- `apps/backend/internal/agent/runtime/lifecycle/session.go` and `session_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/start_model.go` (only if needed for explicit attempt input)

## Dependencies

None.

## Risks

A shared model-policy edit can unintentionally change Office and other providers.
Assert scope at callers and resulting outbound selection, not just a policy bool.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/explicit-resume-settings.md)
- [Design](../../specs/agents/system-design/explicit-resume-settings.md#settings-application)
- Existing `start_model_executor_authority_test.go` and `session_test.go` patterns.

## Results

Implemented explicit canonical task/Office launch scope propagation and strict
Auggie ACP task model application for fresh launch and ordinary/native resume.
The policy disables the fallback inputs for task-scoped Auggie only; Office,
other providers, passthrough, and empty selections retain their existing paths.
Permission-mode application and confirmation semantics are unchanged.

Red-green evidence: before the strict task policy existed,
`TestAuggieTaskStartRequiresSelectedModel` accepted an unavailable selected model
through the configured fallback. The focused regression failed as expected; it
passed after the scoped policy was added.

Verification:

- Focused changed suites passed:
  `GOCACHE=/private/tmp/kandev-go-cache go test -tags fts5 ./internal/agent/runtime/lifecycle ./internal/backendapp ./internal/orchestrator/executor -run '^(TestAuggieTaskStartRequiresSelectedModel|TestInitializeAndPromptWithLayers_AuggieTaskRejectsUnappliedMode|TestLaunchPreparedSession_Success|TestBuildLifecycleLaunchRequestCarriesTaskScope|TestResumeSession_PassesResolvedTaskSessionMCPModeToAgentManager)$' -count=1`.
- The merged full orchestrator subtree and backendapp checks passed. The full
  lifecycle package passed with these four unrelated SSH orphan-process tests
  skipped: `TestSSHOrphanStopCommandKillsProcessAndDirectChildProcessGroup`,
  `TestSSHOrphanStopCommandSessionDirSurvivesWhenLiveAgentctlMatchesTaskDir`,
  `TestSSHOrphanStopCommandKillsMultipleChildProcessGroupsUnderZsh`, and
  `TestSSHOrphanStopCommandMismatchedIdentityLeavesProcessAlive`. A clean
  unchanged-baseline checkout reproduces all four failures. The lifecycle run
  used `TMPDIR=/private/tmp`; the socket-path test
  `TestBuildAuthMethodsIdentityAgentOverridesEnvironment` passed with this
  shorter temporary root. Changed lifecycle tests and the adjusted orchestrator
  suite passed. This work order is `done`; no unrelated SSH or worktree tests
  were modified.
