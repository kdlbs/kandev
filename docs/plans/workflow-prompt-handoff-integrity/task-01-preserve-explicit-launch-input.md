---
id: "01-preserve-explicit-launch-input"
title: "Preserve explicit prepared-session launch input"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-INITIAL-TASK-BRIEF-001
  - REQ-TASKS-SAVED-PROMPT-DELIVERY-001
  - REQ-TASKS-PROMPT-ATTACHMENTS-001
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003
acceptance_criteria:
  - AC-TASKS-INITIAL-TASK-BRIEF-001.13
  - AC-TASKS-INITIAL-TASK-BRIEF-001.14
  - AC-TASKS-INITIAL-TASK-BRIEF-001.15
  - AC-TASKS-INITIAL-TASK-BRIEF-001.16
  - AC-TASKS-SAVED-PROMPT-DELIVERY-001.6
  - AC-TASKS-SAVED-PROMPT-DELIVERY-001.8
  - AC-TASKS-SAVED-PROMPT-DELIVERY-001.11
  - AC-TASKS-PROMPT-ATTACHMENTS-001.2
  - AC-TASKS-PROMPT-ATTACHMENTS-001.3
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003.2
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003.8
system_design:
  - ../../specs/tasks/system-design/initial-task-brief.md
  - ../../specs/tasks/system-design/saved-prompt-delivery.md
  - ../../specs/tasks/system-design/prompt-attachments.md
  - ../../specs/tasks/system-design/workflow-step-agent-start-ownership.md
---

# Task 01: Preserve explicit prepared-session launch input

## Summary

Repair #5178 at the launch producer and reuse the existing preserving composer.
Keep task-description and automatic-entry semantics distinct through storage,
capacity deferral, replay, and provider admission.

## In scope

- Audit every production `IntentStartCreated` producer. Add the server-only
  description-origin field proposed in the owning design. Mark
  `TaskHandlers.dispatchTaskSession` as a description producer.
- Set `preserveDirectPrompt` for explicit nonempty launch input before seam 2
  can serialize it. Do not use `AutoStart`, string equality, or prompt wording
  to infer provenance.
- Drive `LaunchSession`, not just `stepPromptBodyWithOptions`, in the
  regression. Use `setupTestRepo`, `repoTurnService`, and
  `repositoryBackedMessageCreator` so stored messages reference real turns.
- Cover the matrix in [the plan](plan.md#tests), including retained plans,
  saved prompts, canvas, attachments, and direct-message equivalence.
- Cover the existing `preserve_direct_prompt` serialization, replay and Send
  Now consumers. Retain entry, generation, cancellation, and supersession fences.
- Inspect matching pending initial-submission callbacks. They still compare raw
  submitted input before native admission. Cover callback failure before dispatch.
- Add focused desktop and phone E2E cases through the existing session API and
  transcript. Reuse the mock agent's `E2E_MOCK_AGENT_ACP_TRACE_FILE` and
  `readMockACPTrace`. no new mock command or product instrumentation is needed.
- Update the public launch reference and session guidance in the same change.
  Keep automatic workflow-template wording scoped to automatic input.

## Out of scope

- #5173 completion changes, new recovery receipts, receipt migrations, and
  replay of accepted or uncertain submissions.
- Changing task descriptions, workflow templates, profile routing, actor or
  executor authorization, model eligibility, title ownership, or permissions.
- Changing `InitialCreatePrompt` trigger ownership or the private composed
  automatic-start seam. No frontend product changes.

## Acceptance

1. The real launch regression fails on the base because its replacing template
   removes the explicit brief. After repair, stored content, API content/raw
   content, and the manager-bound prompt retain that brief once.
2. Explicit input survives immediate and deferred launch with canonical context
   blocks once. The first native ACP trace and desktop/phone transcripts retain
   it after reload. Invalid ownership/attachments and failed provider admission
   produce no native prompt. A retained plan cannot replace the brief.
3. Internal creation, empty/whitespace input, no initial prompt, passthrough,
   already-composed automatic input, and legacy replay keep their documented
   compatibility behavior. Clients cannot set the server-only origin field.

## Verification

Run the proposed failing regression before production edits:

```bash
(cd apps/backend && go test -trimpath ./internal/orchestrator -run '^TestLaunchStartCreated_ExplicitPromptIntegrity$' -count=1 -v)
```

After the correction, run:

```bash
(cd apps/backend && go test -trimpath -race ./internal/orchestrator -run '^(TestLaunchStartCreated_(ExplicitPromptIntegrity|ExplicitPromptWithSavedReference|PromptOriginAndAdmission|DeferredPromptIntegrity)|TestLaunchSessionRequest_DescriptionOriginIsServerOnly|TestStartCreatedSession_InitialTaskBrief|TestWorkflowAutoStartCreatedNonEmptyPromptPreservesQueuedHandoff|TestSendQueuedNowConsumesCeilingLaunchAndPreservesWorkflowPrompt)$' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/task/handlers -run '^(TestTaskCreateDispatch_MarksDescriptionPrompt|TestEligibleInitialCreatePromptRequiresOriginalExplicitStepAndText|TestWSAddMessage_InitialTaskBrief)$' -count=1)
(cd apps/backend && go test -trimpath ./internal/prompts/service ./internal/sysprompt ./internal/task/models -count=1)
bash scripts/lint-go-changed apps/backend/internal/orchestrator/session_launch.go apps/backend/internal/orchestrator/session_launch_prompt_integrity_test.go apps/backend/internal/task/handlers/task_http_handlers.go apps/backend/internal/task/handlers/task_create_prompt_origin_test.go
git diff --check
```

These new test names are planned. Add them in dedicated files and confirm
discovery. Include any additional changed Go file in the lint command.
A manager mock alone does not supply native ACP proof.

Install the workspace dependencies once before the first package command.
Add the native trace regression first and record its expected failure before
the production correction. Run these commands sequentially after the correction:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run --project chromium tests/session/start-created-prompt-integrity.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-start-created-prompt-integrity.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:ratchet)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The runner rebuilds and owns its isolated backend, database, and mock provider.
Require discovered cases in each project. Keep one worker per shard and remove
fixture profiles, tasks, saved prompts, and trace files after each case.
Compare the fresh native `session_new` identity with the target session binding;
never select the first prompt in a workspace-wide log. Assert no native prompt
between prepare and accepted launch. In the text-only case, compare native
prompt text with the stored canonical raw prompt. Attachments retain the
existing materialization additions and their own byte/content assertions.

## Files likely touched

- `apps/backend/internal/orchestrator/session_launch.go`
- `apps/backend/internal/orchestrator/session_launch_prompt_integrity_test.go` (new)
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/task/handlers/task_create_prompt_origin_test.go` (new)
- `apps/backend/internal/orchestrator/task_operations.go` only if an existing
  option fails to carry the selected semantics.
- Existing `ceiling_seam2.go`, `ceiling_replay.go`, and `queue_send_now.go`
  are audit surfaces. Change them only if the regression exposes a missing carry.
- `apps/web/e2e/helpers/start-created-prompt-integrity.ts` (new)
- `apps/web/e2e/tests/session/start-created-prompt-integrity.spec.ts` (new)
- `apps/web/e2e/tests/session/mobile-start-created-prompt-integrity.spec.ts` (new)
- `docs/public/websocket-api.md`, `sessions-and-review.md`, and
  `workflow-tips.md` (the latter only where its replacement wording needs scope).

## Dependencies

None. Task 02 does not block this repair.

## Risks

- Setting preservation for every nonempty internal prompt changes task creation.
- A hand-built message fixture without a persisted turn fails on a foreign key
  before it can prove prompt storage.
- Serialized system-looking text cannot supply saved-prompt or canvas trust.
- Sibling auth/profile fixes can touch the same launch file. Reconcile current
  heads without taking over those policies.

## Parallelism

`sequential`

## Inputs

- [Initial-input design](../../specs/tasks/system-design/initial-task-brief.md#explicit-prepared-session-launches).
- [Evidence](evidence.md#issue-5178-confirmed) and the plan's compatibility matrix.
- Existing direct-message seam
  `StartCreatedSessionWithPromptContextAndCanvasGuidancePreservingDirectPrompt`.
- Existing `TestStartCreatedSession_InitialTaskBrief`,
  `TestWorkflowAutoStartCreatedNonEmptyPromptPreservesQueuedHandoff`, and
  `TestSendQueuedNowConsumesCeilingLaunchAndPreservesWorkflowPrompt`.

## Results

Completed on 2026-11-08. The launch producer now distinguishes explicit input
from internal task-description transport with the server-only
`LaunchSessionRequest.PromptIsTaskDescription` field. `launchStartCreated`
selects the existing `preserveDirectPrompt` composer for explicit nonempty
input, and `TaskHandlers.dispatchTaskSession` marks ordinary task creation while
leaving Quick Chat explicit. The same flag is serialized through seam 2 and
read by the ceiling replay consumer.

Verification run from the repository root:

```bash
(cd apps/backend && go test -trimpath -race ./internal/orchestrator -run '^TestLaunchStartCreated_' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/mcp/handlers -run '^TestHandleCreateTask_InitialPromptUsesRequestedWorkflowStepMode$' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/orchestrator -run '^(TestLaunchSessionRequest_DescriptionOriginIsServerOnly|TestStartCreatedSession_InitialTaskBrief|TestWorkflowAutoStartCreatedNonEmptyPromptPreservesQueuedHandoff|TestSendQueuedNowConsumesCeilingLaunchAndPreservesWorkflowPrompt)$' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/task/handlers -run '^(TestTaskCreateDispatch_MarksDescriptionPrompt|TestTaskCreateDispatch_QuickChatPromptIsNotADescription|TestDispatchTaskSessionMarksExplicitCreationPrompt|TestStartAgentForNewTask_SetsDeferredStart)$' -count=1)
(cd apps/backend && go test -trimpath ./internal/orchestrator ./internal/task/handlers ./internal/prompts/service ./internal/sysprompt ./internal/task/models -count=1)
(cd apps/backend && go build -trimpath ./...)
(cd apps/backend && GOMEMLIMIT=3GiB GOMAXPROCS=2 golangci-lint run ./internal/orchestrator ./internal/task/handlers --timeout=20m --concurrency=2)
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/session/start-created-prompt-integrity.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-start-created-prompt-integrity.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

All commands passed. The focused launch matrix also covers workflow-level
instructions, plan mode, canvas guidance allowed and absent, saved-prompt
expansion, a valid attachment identity, a matching pending initial-submission
receipt, a forced pre-provider CAS failure with no native dispatch, and the
recovery marker that keeps the explicit brief during fresh-start recomposition.
MCP initial creation marks its description prompt. The desktop E2E project ran
both replacing and placeholder cases; the mobile project ran the reload case.
The trace required one native `session_new`, one prompt with matching
session/process/connection identity, and exact equality between that prompt and
the stored structured raw prompt; reload rechecks kept the count at one. The
public pages were updated in `docs/public/websocket-api.md`,
`docs/public/sessions-and-review.md`, and `docs/public/workflow-tips.md`.

Known limits: the mock ACP trace proves local dispatch, not real-provider
obedience, remote executors, or PostgreSQL behavior. No PostgreSQL variant was
needed because the change is prompt composition and request provenance, not
dialect-sensitive storage. #5173 was not modified.
