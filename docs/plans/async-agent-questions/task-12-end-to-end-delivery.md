---
id: "12-end-to-end-delivery"
title: "Prove answers reach agents end to end"
status: completed
wave: 10
depends_on:
  - "07-parent-session-routing"
  - "08-agent-mode-guidance"
  - "11-api-and-cleanup"
plan: "plan.md"
requirements:
  - REQ-TASKS-ASYNC-QUESTIONS-003
  - REQ-TASKS-ASYNC-QUESTIONS-004
  - REQ-TASKS-ASYNC-QUESTIONS-005
  - REQ-TASKS-ASYNC-QUESTIONS-006
  - REQ-TASKS-ASYNC-QUESTIONS-009
acceptance_criteria:
  - AC-TASKS-ASYNC-QUESTIONS-003.2
  - AC-TASKS-ASYNC-QUESTIONS-004.1
  - AC-TASKS-ASYNC-QUESTIONS-004.2
  - AC-TASKS-ASYNC-QUESTIONS-005.1
  - AC-TASKS-ASYNC-QUESTIONS-005.2
  - AC-TASKS-ASYNC-QUESTIONS-006.2
  - AC-TASKS-ASYNC-QUESTIONS-006.4
  - AC-TASKS-ASYNC-QUESTIONS-009.1
  - AC-TASKS-ASYNC-QUESTIONS-009.2
  - AC-TASKS-ASYNC-QUESTIONS-009.3
  - AC-TASKS-ASYNC-QUESTIONS-009.4
  - AC-TASKS-ASYNC-QUESTIONS-009.6
system_design:
  - ../../specs/tasks/system-design/async-agent-questions.md
---

# Task 12: Prove answers reach agents end to end

## Summary

Verify the feature's core promise in the running app: an answer given in the UI reaches the agent, and the agent acts on it.
Current specs stop when the Inbox row disappears, so this has not been shown yet.
Any defect found here is fixed in this work order, with a regression test at the right level.

## In scope

- Mock-agent scenarios that emit a marker containing the delivered answer after resuming.
- Parent/child mock scenarios for forwarding, and a withdrawal scenario.
- The E2E flows listed in the [plan](plan.md#e2e-tests), on desktop and phone.
- A run with the Inbox disabled.
- A focused re-run of the backend and frontend suites against current main.

## Out of scope

- Real-agent evaluation of mode choice.
- Backend restart safety, which integration tests own.

## Acceptance

1. An answer from the chat strip produces a mock-agent marker containing that answer, and the same holds after an explicit wait.
2. A forwarded question answered from the parent's chat reaches the child, and a withdrawn question keeps its transcript entry and leaves the strip.
3. All of this passes on current main, on `chromium` and `mobile-chrome`, including with the Inbox disabled.

## ASCII UI preview

These flows exercise UI-04 to UI-07 in [the plan](plan.md#ascii-ui-preview), and add no new views.

```text
UI-04 -> [Submit] -> mock agent: ASYNC_ANSWER_RECEIVED PostgreSQL
```

## Verification

Run from the repository root. In a fresh worktree, first run `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/backend && go test -trimpath ./cmd/mock-agent/... -count=1)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/async-agent-questions.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-async-agent-questions.spec.ts)
(cd apps/backend && go test -trimpath ./internal/task/questions ./internal/clarification ./internal/mcp/server ./internal/mcp/handlers ./internal/sysprompt ./internal/orchestrator/messagequeue -count=1)
(cd apps/backend && go test -trimpath ./internal/task/repository/sqlite -run '^TestAsyncQuestions' -count=1)
(cd apps/backend && go test -trimpath ./internal/orchestrator -run '^TestAsyncQuestions' -count=1)
(cd apps/web && pnpm run typecheck && pnpm run lint)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/decisions docs/plans/async-agent-questions
```

Record CI evidence that the async PostgreSQL tests ran, not only that their package passed.

## Files likely touched

- `apps/backend/cmd/mock-agent/scenarios.go`, `scenarios_clarification.go`
- `apps/web/e2e/tests/chat/async-agent-questions.spec.ts`
- `apps/web/e2e/tests/chat/mobile-async-agent-questions.spec.ts`
- `docs/plans/async-agent-questions/plan.md` and this work order's Results

## Dependencies

Tasks 07, 08, and 11.

## Risks

- This is the first run of the full delivery path. A failure here may point to a real bug in queue readiness, not a test problem, and fixing it could grow this work order.
- E2E feature flags are profile-managed. The Inbox-off run needs an existing per-spec override, so check how specs under `e2e/tests/auth` set flags. If none fits, cover that case with backend and hook tests, and record the gap.
- Use causal waits as `apps/web/e2e/README.md` requires. Do not budget for the effect with long timeouts.

## Parallelism

`sequential`

## Inputs

- `apps/backend/cmd/mock-agent/AGENTS.md`, and `/e2e` skill guidance.
- [Plan E2E section](plan.md#e2e-tests).
- [Answer reads and queued delivery](../../specs/tasks/system-design/async-agent-questions.md#answer-reads-and-queued-delivery).

## Results

Added mock-agent handling for correlated answer deliveries and E2E scenarios for Inbox answers, mid-turn chat answers, explicit waits, parent forwarding, withdrawal history, and the Inbox-disabled path.

Passed:

- `go test -trimpath ./cmd/mock-agent/... -count=1`
- Chromium async question E2E: 7 tests passed
- Mobile Chrome async question E2E: 2 tests passed
- Focused async question and Inbox Vitest suites: 217 tests passed
- Backend async question, MCP, clarification, orchestrator, persistence, and SQL guard checks
- Typecheck, lint, i18n check, and i18n ratchet
- Changed-file Go lint: `0 issues`
- Public docs validation, catalog validation, 36 specification-linter tests, and full specification lint

PostgreSQL-specific tests were skipped because `KANDEV_TEST_POSTGRES_DSN` is not configured.
