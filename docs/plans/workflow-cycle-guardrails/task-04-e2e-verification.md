---
id: "04-e2e-verification"
title: "Workflow cycle guardrail E2E verification"
status: done
wave: 4
depends_on: ["03-diagnostic-ui"]
plan: "plan.md"
spec: "../../specs/workflow-cycle-guardrails/spec.md"
---

# Task 04: Workflow cycle guardrail E2E verification

## Acceptance

- Desktop Playwright covers warning cancel/confirm, blocking draft creation,
  exact trace/prompt source, persisted diagnostics, and an allowed generic
  cycle without auto-start.
- A `mobile-*.spec.ts` test completes the warning confirmation flow by touch
  and asserts the trace/actions fit without horizontal page overflow.
- Focused and repository verification commands run after formatting, with any
  environment-only blocker reported exactly.

## Verification

```bash
make fmt
cd apps/web && pnpm e2e:run --host tests/workflow/workflow-cycle-guardrails.spec.ts
cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/workflow/mobile-workflow-cycle-guardrails.spec.ts
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
make typecheck test lint
```

## Files Likely Touched

- `apps/web/e2e/pages/workflow-settings-page.ts`
- `apps/web/e2e/tests/workflow/workflow-cycle-guardrails.spec.ts`
- `apps/web/e2e/tests/workflow/mobile-workflow-cycle-guardrails.spec.ts`

## Dependencies

Task 03.

## Inputs

- Every user-visible spec scenario.
- Existing workflow settings page object and API helper patterns.
- Mobile parity requirement that mobile tests use the `mobile-` filename
  convention and real touch-accessible actions.

## Output Contract

When finished, change this task's `status` to `done`, check it in `plan.md`, and
report desktop/mobile scenarios, files changed, exact commands/results,
environment blockers, and residual risks.
