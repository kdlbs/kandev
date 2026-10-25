---
id: "12-e2e-and-mock-agent"
title: "Mock-agent review scenario and Playwright E2E"
status: pending
wave: 9
depends_on: ["11-mobile-parity"]
plan: "plan.md"
spec: "../../specs/agents/requirements/native-code-review.md"
---

# Task 12: Mock-agent review scenario and Playwright E2E

Hermetic coverage for both trigger paths.

## Inputs

- Spec **Scenarios** — the E2E table in `plan.md` maps each covered scenario to a file.
- `apps/backend/cmd/mock-agent/AGENTS.md` — prompt-marker detection precedent (`isChangesWalkthroughRequest`), the inline `e2e:mcp:kandev:<tool>({...})` directive, and the **rebuild before e2e** requirement.
- `apps/web/e2e/README.md` for project layout and helpers.

## Work

1. `apps/backend/cmd/mock-agent/handler.go` — `isCodeReviewRequest(prompt)` matching the `KANDEV_CODE_REVIEW_REQUEST` sentinel, dispatching to a handler that returns a deterministic fenced-JSON payload with: one `blocker` finding, one `nit` finding, one entry that is deliberately malformed (to exercise the rejected-count path), and a short summary. Derive the anchored file/line from the `Available changed files:` block in the prompt so the payload anchors to a file the test actually changed.
2. `apps/backend/cmd/mock-agent/mock_agent_test.go` — assert the dispatch and the payload shape.
3. `apps/web/e2e/review/code-review-on-demand.spec.ts` — run from the Review surface; assert the run reaches completed, the finding card renders at the expected line, the overview count, resolve-then-reload persistence, send-to-agent reaching the chat, and the no-capable-agent inline message (by disabling the `code-review` utility agent and clearing the default).
4. `apps/web/e2e/review/code-review-workflow-step.spec.ts` — configure a step with `run_code_review`, move a task with changes into it, assert findings appear and the run reports the workflow-step trigger.
5. `apps/web/e2e/mobile/code-review-findings.spec.ts` — phone viewport: findings sheet visible, resolve from the sheet, tap-to-navigate.
6. Add `data-testid` attributes to the new components as needed; follow the `data-legacy-testid` rule if any existing id is renamed.

## Acceptance

- All three specs pass locally against a freshly built mock agent.
- No spec depends on wall-clock sleeps; waits are on observable UI state.

## Verification

```
make -C apps/backend build-mock-agent
cd apps/web && pnpm e2e e2e/review/code-review-on-demand.spec.ts e2e/review/code-review-workflow-step.spec.ts
cd apps/web && pnpm e2e --project=mobile-chrome e2e/mobile/code-review-findings.spec.ts
```

## Files likely touched

`apps/backend/cmd/mock-agent/{handler.go,mock_agent_test.go}`, `apps/web/e2e/review/*.spec.ts`, `apps/web/e2e/mobile/code-review-findings.spec.ts`, new `data-testid`s in the review components.

## Output contract

Summary, files changed, tests run with results, blockers, risks, `status: done`, plan checkbox.
