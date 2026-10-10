---
created: 2026-09-29
status: done
requirements:
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-001
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003
system_design:
  - ../../specs/tasks/system-design/queued-session-ownership.md
  - ../../specs/tasks/system-design/workflow-explicit-completion-signal.md
legacy_specs: []
---

# Implementation plan: Resume todo turn boundary

## Overview

[Issue #4047](https://github.com/kdlbs/kandev/issues/4047) reports rejected
completion signals after a parked worker returns from review. Resume todo
metadata creates an open turn under Review. The next Work prompt adopts that
turn, so the completion guard correctly rejects its old step stamp.

One sequential work order keeps todo snapshots from creating open prompt turns
or changing immutable step stamps, while preserving the latest todo list for
reload. Production implementation and validation are complete.

## Evidence and root cause

Investigation used checkout `cb9a530004f7d624629c9d9dd6cebdff4a32e7de`.
The issue contains log and database extracts, with no image attachments.

1. ACP `adapter_session.go:LoadSession` captures replayed plans and calls
   `emitReplayPlan` after history suppression ends.
2. `event_handlers_streaming.go:handleSessionTodosEvent` broadcasts that plan.
   `persistTodoMessage` then calls the turn-creating `getActiveTurnID`.
3. `service.go:startTurnForSessionWithOwnershipChecked` starts a turn when none
   exists. The real task service stamps the current Review step atomically.
4. `recordAutoStartMessage` later reuses the cached or persisted open turn.
   The Work prompt therefore retains the Review stamp.
5. The completion guard rejects this mismatch, as required by criterion 003.1.

A temporary real-service reproduction demonstrated steps 2 through 4. It used
`newServiceBackedMessageCreator(repo).svc` as `TurnService`, because
`repoTurnService.StartTurn` does not implement production step stamping.
The temporary file is removed after diagnosis. Permanent tests belong to Task 01.

The automatic resume caller in the reported production runs remains unproven.
Opening an earlier conversation is an authorized caller under the
[session-open decision](../../decisions/2026-09-18-session-open-resumes-conversation.md).
The repair does not depend on which caller started the resume.

The newer ACP resume path reduces history transfer only for providers that
advertise and implement it. `adapter_session_resume.go` retains load fallback.
The todo handler still creates a turn whenever such a snapshot reaches it.
No live `omp-acp` version comparison was performed.

## Scope

### In scope

- Todo persistence with live broadcasts and lifecycle-only durability outside
  prompt turns.
- Existing active and reserved prompt turns, empty todo lists, and lookup errors.
- Re-entry after recovery, including a cold orchestrator turn cache.
- Existing immutable stamps, clarification barriers, and signal rejection.

### Out of scope

- Disabling automatic recovery or changing park/reuse policy.
- Re-stamping turns, weakening signal validation, or forcing a task move.
- Repairing old polluted turns by inference from message text or missing prompts.
- General stream-event redesign, provider capability changes, or UI changes.

## Technical approach

Use the existing non-creating turn lookup and reserved-prompt identity in
`apps/backend/internal/orchestrator/event_handlers_streaming.go`.
Keep the event-bus todo broadcast before persistence. Resolve reserved prompt
ownership first, then query the authoritative active turn. Persist against an
explicit ID when one exists. After a successful lookup proves there is no active
turn, use the task service's completed lifecycle-only message path. A lookup
error omits persistence; it is not proof of absence. Never pass an empty ID to
the normal message path.

The requirement extension clarifies recovery behavior in the existing owner.
The existing session-open and immutable-stamp decisions settle the alternatives.
No new ADR, flag, database migration, or runtime setting is required.

| Provider path | Identity | Intended behavior | Evidence |
| --- | --- | --- | --- |
| ACP load fallback | Existing session and restored plan | Broadcast and persist in active/reserved or completed lifecycle-only turn | Adapter replay tests and new orchestrator tests |
| ACP advertised resume | Existing session, optional metadata | Same ownership and durability rule | Resume selection tests and shared handler tests |
| Other normalized plan sources | Session with active/reserved turn | Preserve normal persistence and explicit identity | Active/reserved/empty-list controls |
| Confirmed absence | No active turn after successful lookup | Persist in a completed lifecycle-only turn | Empty-list and readback tests |
| Failed lookup | Turn ownership unknown | Broadcast only and skip persistence | Lookup-error test |

## Tests

Task 01 maps criteria 001.10-.12 to new tests in
`apps/backend/internal/orchestrator/resume_todo_turn_boundary_test.go`.
Existing task-service and MCP tests protect criteria 003.1 and 003.3.
Use the exact commands in the work order.

## E2E tests

Backend integration is the faithful end-to-end boundary for this defect:
normalized resume todo/status events, real task-service persistence, workflow
prompt recording/dispatch, and current-step turn stamping.
`TestWorkflowAutoStartAfterResumeTodos` covers Work -> Review -> Work with the
same worker session. Use the existing workflow auto-start harness and a fake
runtime to assert one prompt delivery. Existing MCP tests verify fresh-turn
eligibility and stale-turn rejection. No browser layout or control changes are
planned, so no artificial Playwright test is required.

## Work orders

- [x] [Task 01: Preserve prompt ownership during todo recovery](task-01-todo-turn-ownership.md)

## Verification results

- Diagnosis: temporary `TestRepro4047ResumeTodoAbsorbsWorkflowPrompt` reproduced
  an open Review-stamped turn adopted by Work. The expected invariant failed.
- The first permanent regression failed against the original implementation
  with one open turn after todo recovery. The final regression suite passes.
- `(cd apps/backend && go test -tags fts5 ./internal/orchestrator -count=1)`:
  passed (243.714s).
- `(cd apps/backend && go test -tags fts5 ./internal/task/service -run 'TestStartTurn.*Stamp|TestCreateCompletedTurn' -count=1)`: passed.
- `(cd apps/backend && go test -tags fts5 ./internal/mcp/handlers -run 'TestHandleStepComplete_' -count=1)`: passed.
- `(cd apps/backend && go test ./internal/agentctl/server/adapter/transport/acp -run 'TestLoadSuppression_|TestLoadSession' -count=1)`: passed.
- `(cd apps/backend && go test ./internal/backendapp -run '^TestMessageCreatorAdapter_CreateLifecycleSessionMessage$' -count=1)`: passed.
- `(cd apps/backend && go test ./internal/integration -run '^$')`: passed; test adapter compiled.
- `(cd apps/backend && go build ./internal/orchestrator ./internal/backendapp)`: passed.
- `python3 scripts/list-docs.py validate`: passed (327 decisions, 1243 specifications).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed after implementation and result updates.
- `.github/scripts/pr-docs.cjs:validateCoverage`: returned `covered` for the
  implementation and specification changes, with both requirement/design pairs
  accepted and no errors.
- Temporary reproduction file removed. Implementation changes are uncommitted.
- GitHub assignment verified as `carlosflorencio`.

## Risks

- An empty message turn ID silently creates a turn in the task service.
- Ignoring reserved prompt turns can lose legitimate early todo output.
- Existing historical polluted turns still require ordinary fresh-turn recovery.
- Each out-of-turn recovery snapshot gets its own completed lifecycle-only
  message; repeated identical provider replays can add repeated timeline rows.
- This evidence proves the reported todo path, not every unsolicited provider event.
- The production resume trigger and behavior of the reporter's newer agent remain unverified.

## Related delivery records

The completed [session-open package](../session-open-recovery-eligibility/plan.md)
retains authority over recovery admission. The completed
[stale-turn package](../step-completion-stale-turn-recovery/plan.md) retains
completion diagnostics. Their behavior and verification results remain unchanged.

The historical [queue package](../queued-session-ownership/plan.md) still records
outstanding database verification. Its parking-suppression policy is explicitly
superseded. This package neither claims that verification nor restores that policy.

## Documentation impact

Internal requirements, system design, plan, and work order change in this turn.
Public docs remain unchanged because behavior uses existing todo and recovery
surfaces. Implementation retains existing stale-turn recovery guidance.
