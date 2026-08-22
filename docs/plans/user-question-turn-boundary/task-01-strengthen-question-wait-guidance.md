---
id: "01-strengthen-question-wait-guidance"
title: "Strengthen question wait guidance"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/user-question-turn-boundary.md"
---

# Task 01: Strengthen Question Wait Guidance

## Acceptance

1. Every normal task first-turn context that exposes
   `ask_user_question_kandev` explicitly forbids another tool call, continued
   work, or a final response until completed user answers return.
2. The same guidance requires an immediate turn end when the call returns
   without completed answers, while completed answers or a structured rejection
   remain usable results.
3. Autopilot child/root capability guidance and the question tool schema,
   transport, timeout, and persistence behavior remain unchanged.

## Verification

Follow strict TDD, then run:

```bash
cd apps/backend
go test ./internal/sysprompt ./internal/mcp/server -run \
  'TestFormatKandevContext_(UserQuestionIsHardInputBarrier|AutopilotChildUsesParentQuestionOnly|AutopilotRootHasNoQuestionTool)|TestAskUserQuestionDocs_MatchSchema|TestAskUserQuestion_StreamsKeepAliveDuringWait' \
  -count=1
cd ../..
git diff --check
```

## Files likely touched

- `apps/backend/internal/sysprompt/sysprompt.go`
- `apps/backend/internal/sysprompt/sysprompt_test.go`
- `docs/specs/tasks/user-question-turn-boundary.md`
- `docs/specs/INDEX.md`
- `docs/plans/user-question-turn-boundary/plan.md`
- `docs/plans/user-question-turn-boundary/task-01-strengthen-question-wait-guidance.md`

## Dependencies

None.

## Parallelism

`sequential` — the prompt wording and its regression test are one localized
behavioral repair.

## Inputs

- [User Question Turn Boundary spec](../../specs/tasks/user-question-turn-boundary.md)
- [Fix plan](plan.md)
- `userQuestionSection`, `parentQuestionSection`, and `autopilotSection` in
  `apps/backend/internal/sysprompt/sysprompt.go`
- Existing question capability tests in
  `apps/backend/internal/sysprompt/sysprompt_test.go`
- Existing blocking/keepalive regression in
  `apps/backend/internal/mcp/server/ask_user_question_test.go`
- [ADR 0015](../../decisions/0015-explicit-completion-signal-for-auto-advance.md)

## Risks

- Keep the distinction between an answered call and an early non-answer return
  explicit. Otherwise, the prompt can stop useful work after the user replies.
- Keep the new text capability-scoped so an autopilot root never receives
  instructions for an unavailable tool.

## Output contract

Report red/green evidence, the final prompt contract, files changed, exact test
results, residual risks, and synchronized task/plan status.

## Results

- RED: `cd apps/backend && go test ./internal/sysprompt -run
  '^TestFormatKandevContext_UserQuestionIsHardInputBarrier$' -count=1` failed
  because `userQuestionSection` did not state a hard input barrier.
- GREEN: The same focused test passed after the prompt update.
- `cd apps/backend && go test ./internal/sysprompt ./internal/mcp/server -run
  'TestFormatKandevContext_(UserQuestionIsHardInputBarrier|AutopilotChildUsesParentQuestionOnly|AutopilotRootHasNoQuestionTool)|TestAskUserQuestionDocs_MatchSchema|TestAskUserQuestion_StreamsKeepAliveDuringWait' -count=1`
  — passed, 5 tests across 2 packages.
- `gofmt -w internal/sysprompt/sysprompt.go internal/sysprompt/sysprompt_test.go` completed.
- `git diff --check` and checks for the three new design files — passed.
- Changed files match the task scope: the sysprompt constant, its regression
  test, the spec index, the repair spec, the plan, and this task file.
- No browser E2E ran because the change affects hidden agent instructions only.
- No security, external side effects, or cleanup work applies.
