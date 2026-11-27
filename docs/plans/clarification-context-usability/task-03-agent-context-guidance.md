---
id: "03-agent-context-guidance"
title: "Guide concise decision context"
status: done
wave: 3
depends_on: ["02-host-bounded-expansion"]
plan: "plan.md"
requirements:
  - REQ-UI-CLARIFICATION-CONTEXT-001
acceptance_criteria:
  - AC-UI-CLARIFICATION-CONTEXT-001.7
  - AC-UI-CLARIFICATION-CONTEXT-001.12
system_design:
  - ../../specs/ui/system-design/clarification-context.md
---

# Task 03: Guide concise decision context

## Summary

Guide agents to provide concise decision context while preserving valid long input and the hard question barrier.
Update published tool descriptions and conditional system guidance together.

## In scope

- Update the user-question tool description plus `context` and `context_paragraphs` descriptions.
- Recommend 1-3 short sentences or brief bullets, aiming for about 100 words or fewer.
- Put full reports/logs and repeated conversation outside the tool call, before it. Keep questions/options self-contained.
- Add schema, long-input compatibility, and conditional prompt regression tests.

## Out of scope

Validation limits, truncation, parser changes, parent-question guidance, wait semantics, and new UI.

## Acceptance

1. `TestAskUserQuestion_ConciseContextGuidance` fails before the description update, then passes for the tool and both fields.
2. Long legacy context remains verbatim. Non-empty paragraphs still take precedence. Empty paragraph arrays retain the existing fallback.
3. `TestFormatKandevContext_ConciseQuestionContext` covers normal task guidance and preserves the existing barrier and excluded profiles.

## Verification

From the repository root. Read `apps/backend/AGENTS.md` before implementation.

```bash
(cd apps/backend && go test ./internal/mcp/server -run 'TestAskUserQuestion|TestAskParentQuestion' -count=1)
(cd apps/backend && go test ./internal/sysprompt -count=1)
bash scripts/lint-go-changed apps/backend/internal/mcp/server/server.go apps/backend/internal/mcp/server/ask_user_question_test.go apps/backend/internal/sysprompt/sysprompt.go apps/backend/internal/sysprompt/sysprompt_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

These are text/schema and handler-contract changes. Browser evidence belongs to tasks 01 and 02.
Record the PR-documentation coverage preflight for the final changed paths before delivery.

## Files likely touched

- `apps/backend/internal/mcp/server/server.go`.
- `apps/backend/internal/mcp/server/ask_user_question_test.go`.
- `apps/backend/internal/sysprompt/sysprompt.go`.
- `apps/backend/internal/sysprompt/sysprompt_test.go`.

## Dependencies

Task 02 completes the UI path for long context. This sequencing does not authorize delegation.

## Risks

Placing detailed analysis after the tool call would conflict with the hard input barrier.
A textual recommendation must not become a validation rule or alter paragraph normalization.

## Parallelism

`sequential`

## Inputs

- Context design: Agent guidance and Shared context transport and presentation.
- Existing `TestFormatKandevContext_UserQuestionIsHardInputBarrier` and user/parent context tests.
- [Task question barrier requirement](../../specs/tasks/requirements/user-question-turn-boundary.md).

## Results

Completed. The tool description and both context field descriptions recommend 1-3 short sentences or brief bullets, aiming for about 100 words or fewer. They ask agents to put full reports, logs, and repeated conversation before the tool call, and to keep questions and options self-contained. The conditional task prompt carries the same guidance and retains the hard user-input barrier. No validation, truncation, normalization, or paragraph precedence changed.

- TDD RED: the concise-guidance schema and system-prompt regressions failed before the description updates.
- `go test ./internal/mcp/server -run 'TestAskUserQuestion|TestAskParentQuestion|TestCoreToolDescriptionsStayFocused' -count=1`: passed, including verbatim long legacy context, paragraph precedence, empty-array fallback, and the 600-character tool description cap.
- `go test ./internal/sysprompt -count=1`: passed, including normal-task guidance, excluded autopilot profiles, hard-barrier behavior, and the rendered prompt-size regression.
- `bash scripts/lint-go-changed apps/backend/internal/mcp/server/server.go apps/backend/internal/mcp/server/ask_user_question_test.go apps/backend/internal/sysprompt/sysprompt.go apps/backend/internal/sysprompt/sysprompt_test.go`: passed with 0 issues.
- Specification catalog validation, spec lint, public-doc validation, and `git diff --check`: passed.
- Final `.github/scripts/pr-docs.cjs` `validateCoverage` preflight: 59 changed paths, `covered`, all three work orders selected, and no errors.
