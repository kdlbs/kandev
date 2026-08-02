---
id: "01-reuse-shared-composer"
title: "Reuse shared launch composer"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/agent-launch-prompt-composer.md"
---

# Task 01: Reuse shared launch composer

## Acceptance

- New Agent and handoff render `TaskFormInputs` in session mode, exposing saved-prompt autocomplete, attachments, enhancement recovery, and voice without the bespoke `SessionPromptField`.
- Context copy/blank/summary, prompt enhancement, Ctrl/Cmd+Enter, Start Agent, and voice auto-send use the shared handle's current value; launch includes its accepted attachments.
- Existing profile compatibility, busy/disabled states, automatic handoff summary, environment reuse, session activation, close, and launch-error behavior remain unchanged.

## Verification

Follow TDD: add or adapt the focused regression first and confirm it fails because New Agent does not render/use the shared composer, then implement the minimal integration and rerun it.

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/task/new-session-dialog.test.tsx components/task/new-session-form-actions.test.ts components/task/new-session-form-prompt.test.tsx components/task/session-context-summary.test.ts components/task-create-dialog-selectors.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:ratchet
cd apps/web && pnpm exec eslint components/task/new-session-dialog.tsx components/task/new-session-dialog.test.tsx components/task/new-session-form-actions.ts components/task/new-session-form-actions.test.ts components/task/new-session-form-prompt.test.tsx components/task/session-context-summary.ts
```

## Files likely touched

- `apps/web/components/task/new-session-dialog.tsx`
- `apps/web/components/task/new-session-dialog.test.tsx`
- `apps/web/components/task/new-session-form-actions.ts`
- `apps/web/components/task/new-session-form-actions.test.ts`
- `apps/web/components/task/new-session-form-prompt.tsx` (delete)
- `apps/web/components/task/new-session-form-prompt.test.tsx` (controller regression retained under its original focused-test filename)
- `apps/web/components/task-create-dialog-selectors.tsx` only if the existing handle contract needs a narrow shared adjustment
- `apps/web/components/task-create-dialog-selectors.test.tsx` only with such an adjustment or a missing shared regression

## Dependencies

None.

## Parallelism

Sequential. This task owns the shared composer integration and all directly affected unit tests.

## Inputs

- Spec: `Agent Launch Prompt Composer` — What, Failure modes, and all non-E2E scenarios.
- Plan: Confirmed root cause, Shared composer integration, Existing shared component, and Risks.
- Existing patterns: `DialogPromptSection` in `apps/web/components/task-create-dialog-form-body.tsx`, `TaskFormInputsHandle` in `apps/web/components/task-create-dialog-types.ts`, and task-create session mode.

## Output contract

Report the RED failure, final files changed, exact commands and counts, whether `TaskFormInputs` required any API change, handoff/voice timing evidence, blockers, risks, and synchronized task/plan status.

## Results

- RED: the controller regression exposed that the old controlled-textarea adapter could not represent an unavailable shared composer target; the test harness was updated to exercise the `TaskFormInputsHandle` recovery path.
- GREEN: the shared `TaskFormInputs` is now rendered in session mode by New Agent and handoff. Context updates, enhancement/recovery, submit, attachments, Ctrl/Cmd+Enter, and voice auto-send all use the imperative handle. No `TaskFormInputs` API changes were required.
- Verification: focused unit suite passed with 5 files / 34 tests; typecheck, i18n ratchet, and targeted ESLint all passed.
- Final production files: `new-session-dialog.tsx`, `new-session-form-actions.ts`, `session-context-summary.ts`; bespoke `new-session-form-prompt.tsx` deleted. The controller test remains in `new-session-form-prompt.test.tsx` because the hook is exported from the dialog module.
- Handoff timing: the prompt-result controller snapshots the source before enhancement and falls back to that snapshot if the shared editor unmounts, preserving Apply/Copy recovery.
