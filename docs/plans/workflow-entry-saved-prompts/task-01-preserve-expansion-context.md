---
id: "01-preserve-expansion-context"
title: "Preserve workflow expansion context"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-SAVED-PROMPT-DELIVERY-001
acceptance_criteria:
  - AC-TASKS-SAVED-PROMPT-DELIVERY-001.4
  - AC-TASKS-SAVED-PROMPT-DELIVERY-001.5
  - AC-TASKS-SAVED-PROMPT-DELIVERY-001.6
  - AC-TASKS-SAVED-PROMPT-DELIVERY-001.8
  - AC-TASKS-SAVED-PROMPT-DELIVERY-001.12
  - AC-TASKS-SAVED-PROMPT-DELIVERY-001.13
  - AC-TASKS-SAVED-PROMPT-DELIVERY-001.14
system_design:
  - ../../specs/tasks/system-design/saved-prompt-delivery.md
---

# Task 01: Preserve workflow expansion context

## Summary

Carry backend-generated saved-prompt context through workflow dispatch and composed
session launch. Prove that profile switches and recovery preserve the same
saved definitions in message storage and agent input.

## In scope

- Implement the explicit context flow described in the plan and system design.
- Cover every `buildWorkflowEntryPrompt` caller and downstream canonicalizer.
- Add all five regression groups named in the plan with the real prompt service.
- Re-resolve queued workflow auto-start references at drain before message
  recording, then preserve that exact context through missing-execution recovery.
- Require `org.config.manage` for shared prompt mutations while retaining member
  reads and reference use.
- Update public saved-prompt and authentication guidance after the fix passes.

## Out of scope

Literal-tag editor validation, new warnings, frontend changes, profile routing,
queue schemas, and passthrough expansion.

## Acceptance

1. P1/P2/P1 moves through the no-`auto_start_agent` implicit path and context resets retain one backend-generated expansion with matching stored and dispatched definitions.
2. A reused profile session that terminalizes before async dispatch is replaced through the production path; the replacement prompt preserves its session identity, one saved expansion in storage and launch, and the carried handoff. Missing-execution paths retain context without duplicate handoffs, mode wrappers, or messages; queued retry remains functional.
3. Forged blocks remain untrusted, lookup failure remains non-fatal, and passthrough sessions receive no hidden expansion.
4. Org members cannot create, update, or delete shared saved prompts without `org.config.manage`; they can still list and use them.

## Implementation sequence

1. [x] Read the linked design, plan, source, and backend test guidance.
2. [x] Mark this work order `in_progress`.
3. [x] Add `TestWorkflowEntrySavedPrompt_DispatchModes` and observe the CREATED case fail from missing saved content.
4. [x] Return trusted context from workflow entry and propagate it through both injection stages.
5. [x] Complete the round-trip, recovery, composition, and guard test groups.
6. [x] Run the exact checks below and record their results.
7. [x] Mark this work order `done` and synchronize the plan.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -race ./internal/orchestrator -run 'Test(WorkflowEntrySavedPrompt|AutoStartStepPrompt|StartSessionForWorkflowStep|BuildWorkflowPrompt|ApplyWorkflowAndPlanMode|ProcessOnEnter_ProfileSwitch|LaunchAfterOnEnterDispatch|StartCreatedSession_Preserves|StartTask_Preserves)' -count=1)
(cd apps/backend && go test -race ./internal/prompts/service ./internal/sysprompt -count=1)
(cd apps/backend && go test -race ./internal/orchestrator -run '^(TestFallbackFreshLaunch_CoordinatorCancellationWinsBeforeResetWrite|TestFallbackFreshLaunch_DoesNotResetCancellationObservedBeforeGuard|TestFallbackFreshLaunch_ComposedPromptSurvivesStepRecomposition|TestHandlePromptDispatchFailure_ComposedPromptSurvivesInternalFallback|TestDisposeSeam3PromptEnsureRefusalWritesRecordWhenReconstructable)$' -count=1)
(cd apps/backend && go test -race ./internal/orchestrator -run '^TestAutoStartStepPrompt_ReplacementLaunchReusesClaimedHandoff$' -count=1)
(cd apps/backend && go test -race ./internal/orchestrator -run '^(TestWorkflowEntrySavedPrompt_(DispatchModes|ProfileSwitchRoundTrip)|TestProcessOnEnterImplicitProfileSwitchTerminalizedGuard)$' -count=1)
(cd apps/backend && go test -race ./internal/orchestrator -run '^TestSendQueuedNowConsumesCeilingLaunchAndPreservesWorkflowPrompt$' -count=1)
(cd apps/backend && go build ./...)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
git diff --name-only -- '*.go' | xargs -r gofmt -l
```

Do not substitute compilation for executing changed tests.

## Files likely touched

- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/workflow_entry_saved_prompt_test.go` (new)
- Existing orchestrator test callers of changed private signatures, plus handoff and queued-launch regressions.
- `apps/backend/internal/orchestrator/ceiling_seam3.go` and `ceiling_replay.go` for trusted context in deferred prompt replays.
- `docs/public/workflow-tips.md`
- This work order and `plan.md` for status and results.

## Dependencies

None. Execute sequentially in the primary session after an explicit implementation request.

## Risks

The composed launch canonicalizes twice. Preserve trust through both boundaries.
Do not weaken exact-content validation or change cancellation, title, admission,
or handoff ownership while modifying argument propagation.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/tasks/requirements/saved-prompt-delivery.md), criteria listed in frontmatter.
- [System design](../../specs/tasks/system-design/saved-prompt-delivery.md), workflow entry and composed launches.
- [Server-owned expansion ADR](../../decisions/2026-09-01-server-owned-saved-prompt-expansion.md).
- `apps/backend/internal/orchestrator/prompt_launch_fallback_test.go` for real resolver fixtures.
- `apps/backend/internal/orchestrator/event_handlers_workflow_profile_test.go` for routing and reset fixtures.
- `apps/backend/internal/orchestrator/step_handoff_carry_dispatch_test.go` for recovery and handoff fixtures.
- `.agents/skills/tdd/references/backend-tests.md` for deterministic synchronization and cleanup.

## Results

Implemented explicit saved-prompt context propagation through workflow composition,
session launch, profile replacement, dispatch recovery, and admission replay.
Added regression coverage for CREATED and reused sessions, context resets, Office
injection, P1/P2/P1 profile switches, composition and trust guards, mutable-prompt
recovery, ceiling deferral, and queued Send Now. Public workflow guidance now
documents the guarantee.

Review follow-up coverage now proves that `DispatchModes` reaches the Office
reset injector; `ProfileSwitchRoundTrip` covers asynchronous P1/P2/P1 dispatches
without `auto_start_agent`; and `TestProcessOnEnterImplicitProfileSwitchTerminalizedGuard`
terminalizes a reused session at a deterministic barrier before dispatch. The
production replacement launch is checked for replacement session identity, one
matching saved-definition block in the stored and launched prompt, and one carried
completion handoff. `TestAutoStartStepPrompt_ReplacementLaunchReusesClaimedHandoff`
remains a same-session shared-claim control.

The original work-order verification passed on 2026-09-25: the orchestrator
race suite, prompt-service/system-prompt race tests, changed-signature and
queued Send Now regressions, `go build ./...`, catalog validation, specification
lint, public-doc tests (62 passed), public-doc validation (47 pages), gofmt, and
`git diff --check`. The review follow-up focused orchestrator race command also
passed.

The PR fixup adds `TestExecuteQueuedWorkflowPrompt_MissingExecutionKeepsDrainExpansion`,
which drives `executeQueuedMessage`, records the drain-time definition, changes
the saved prompt during the failed provider dispatch, and verifies the
fresh-runtime replacement uses the same one-block context.
`TestWorkflowEntrySavedPrompt_UncomposedRecoveryCarriesTrustedContext` covers
the non-composed recovery branch with a saved-prompt edit.
`TestPromptMutationsRequireOrgConfigManage` denies member POST, PATCH, and
DELETE; the read regression keeps list access open.
`TestInjectAutoStartRuntimeContext_NilStepPreservesPrompt` covers the defensive
nil case.

The targeted fixup command passed on 2026-09-25:

```bash
(cd apps/backend && go test ./internal/prompts/handlers ./internal/orchestrator -run '^(TestPromptMutationsRequireOrgConfigManage|TestPromptReadsRemainAvailableToOrgMembers|TestWorkflowEntrySavedPrompt_UncomposedRecoveryCarriesTrustedContext|TestExecuteQueuedWorkflowPrompt_MissingExecutionKeepsDrainExpansion|TestInjectAutoStartRuntimeContext_NilStepPreservesPrompt)$' -count=1)
```

Post-fixup validation also passed:

```bash
(cd apps/backend && go test -race ./internal/orchestrator -run '^(TestWorkflowEntrySavedPrompt_(DispatchModes|ProfileSwitchRoundTrip|Recovery|UncomposedRecoveryCarriesTrustedContext)|TestProcessOnEnterImplicitProfileSwitch.*|TestAutoStartStepPrompt_ReplacementLaunchReusesClaimedHandoff|TestExecuteQueuedWorkflowPrompt_MissingExecutionKeepsDrainExpansion|TestInjectAutoStartRuntimeContext_NilStepPreservesPrompt)$' -count=1)
(cd apps/backend && go test -race ./internal/prompts/handlers -count=1)
(cd apps/backend && go build ./...)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

The public-doc tests passed (62 tests), and the validator accepted all 47
published pages. The follow-up also passed `gofmt` and `git diff --check`.

This work order and its PR fixup are tracked in [PR #3931](https://github.com/kdlbs/kandev/pull/3931).
