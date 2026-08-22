---
spec: docs/specs/tasks/user-question-turn-boundary.md
created: 2026-08-22
status: building
---

# Fix Plan: Enforce the User Question Turn Boundary

## Overview

Strengthen the capability-aware `ask_user_question_kandev` entry in the
first-turn task system context and bind the hard-wait wording to a focused
regression test. The transport and clarification lifecycle already block and
recover unanswered calls, so the repair stays within prompt composition.

## Confirmed root cause

`apps/backend/internal/sysprompt/sysprompt.go` currently describes
`ask_user_question_kandev` as waiting for all answers. It does not define the
agent's actions after the call. It also gives no instruction for a result that
contains no answers. By contrast, the neighboring parent-question and
autopilot guidance says that the question ends the turn. That guidance also
forbids further tool calls or work.

The handler in `apps/backend/internal/mcp/server/handlers.go` already waits on
the backend and streams progress keepalives while the user considers the
question. Its existing test proves the final result is withheld until the
backend releases an answer. The remaining failure occurs when an agent or MCP
client treats an early non-answer result as permission to continue. The normal
task prompt does not define a hard input barrier.

Smallest source-level reproduction: format a normal task context with
`sysprompt.FormatKandevContextWithOptions` and inspect the
`ask_user_question_kandev` bullet. It contains "wait for all answers." It has
no instruction to avoid another tool call, further work, or a final response.
It has no fail-closed action for timeout, disconnect, or pending results.

## Backend

### Capability-aware prompt guidance

- Update `userQuestionSection` in
  `apps/backend/internal/sysprompt/sysprompt.go` so the tool entry remains a
  concise overview while defining a hard user-input barrier.
- State that the agent must not call another tool, continue working, or produce
  a final response until the tool returns the user's answers.
- State that a return without completed answers requires the agent to end the
  turn immediately. Preserve the existing ability to continue when completed
  answers or a structured rejection are returned.
- Keep the text inside the capability-controlled section. Do not add it to the
  static `kandev-context.md` template. That location exposes the instruction to
  autopilot roots that have no user-question tool.

## Frontend

No frontend change. The system context is hidden from the rendered chat, and
the clarification overlay and waiting-state behavior remain unchanged.

## Tests

- **What:** A normal task context describes `ask_user_question_kandev` as a hard
  barrier: no later tool call, continued work, or final response before answers,
  and immediate turn end for a non-answer result.
  **File:** `apps/backend/internal/sysprompt/sysprompt_test.go`.
  **How:** Add a focused formatting test against a normal task context. Write it
  first and confirm it fails because the current bullet has only the weaker
  "wait for all answers" wording.
- **What:** Capability boundaries and the live question schema remain aligned.
  **Files:** `apps/backend/internal/sysprompt/sysprompt_test.go` and
  `apps/backend/internal/mcp/server/sysprompt_sync_test.go`.
  **How:** Run the existing autopilot question-capability tests and schema-sync
  test with the new regression.
- **What:** The actual MCP handler continues withholding its final result while
  the backend waits for a user answer.
  **File:** `apps/backend/internal/mcp/server/ask_user_question_test.go`.
  **How:** Rerun the existing streamable HTTP keepalive regression; do not
  change transport code.

No browser E2E is needed because this changes hidden agent instructions, not a
rendered user interaction.

## Verification Results

- RED: `cd apps/backend && go test ./internal/sysprompt -run
  '^TestFormatKandevContext_UserQuestionIsHardInputBarrier$' -count=1` — failed
  as expected because the normal prompt lacked the barrier wording.
- GREEN: `cd apps/backend && go test ./internal/sysprompt -run
  '^TestFormatKandevContext_UserQuestionIsHardInputBarrier$' -count=1` — passed,
  1 test.
- Task checks: `cd apps/backend && go test ./internal/sysprompt ./internal/mcp/server
  -run 'TestFormatKandevContext_(UserQuestionIsHardInputBarrier|AutopilotChildUsesParentQuestionOnly|AutopilotRootHasNoQuestionTool)|TestAskUserQuestionDocs_MatchSchema|TestAskUserQuestion_StreamsKeepAliveDuringWait' -count=1`
  — passed, 5 tests across 2 packages.
- `git diff --check` and checks for all new design files — passed.
- Baseline evidence from planning remains valid: the existing sysprompt package
  suite passed 60 tests, and the two pre-existing MCP tests passed.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Strengthen question wait guidance](task-01-strengthen-question-wait-guidance.md) — done

Execution is sequential in the primary conversation. No subagents are
authorized.

## Risks

- Prompt text cannot mechanically prevent every model from disobeying, so the
  regression can prove only that every normal first-turn context contains the
  unambiguous barrier.
- Wording that says every question call always ends the turn can incorrectly
  forbid the healthy path where the blocking call returns completed answers.
  The repair must distinguish completed answers from early non-answer returns.
- Putting the rule in the static template can expose instructions for a tool
  absent from autopilot roots. Keep it in `userQuestionSection`.

## Out of Scope

- MCP handler, keepalive, timeout, clarification persistence, and resume changes.
- Tool schema or output changes.
- Config, Office, external, or autopilot prompt changes.
- Public documentation changes. The externally observable MCP contract is
  unchanged.
