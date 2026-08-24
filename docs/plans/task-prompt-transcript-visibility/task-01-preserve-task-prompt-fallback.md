---
id: "01-preserve-task-prompt-fallback"
title: "Preserve the task prompt fallback"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/task-prompt-transcript-visibility.md"
---

# Task 01: Preserve the task prompt fallback

## Intent

If loaded transcript history has no visible user-authored message, keep the
original task prompt visible.

## Acceptance

- Agent-only or environment-only visible history keeps the synthetic task
  description as the first message.
- A visible stored user message suppresses the synthetic task description.
- An empty task description does not create an empty synthetic message.
- The transcript keeps its existing message order after the fallback decision.
- The unit regression fails before the production change and passes after it.

## Files likely touched

- `apps/web/hooks/use-processed-messages.ts`
- `apps/web/hooks/use-processed-messages-fallback.test.ts`

## Steps

1. Add the agent-only regression to
   `apps/web/hooks/use-processed-messages-fallback.test.ts`.
2. Run the focused unit test and record the expected failure.
3. Change the fallback condition in `use-processed-messages.ts` to test for a
   visible user-authored message.
4. Add the stored-user case and run the focused unit test again.
5. Run the web type check and record all results in this task file.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- hooks/use-processed-messages.test.ts
cd apps && pnpm --filter @kandev/web test -- hooks/use-processed-messages-fallback.test.ts
cd apps/web && pnpm run typecheck
```

## Dependencies

None.

## Parallelism

Sequential. Task 03 validates this condition with the complete browser flow.

## Inputs

- Spec scenarios in
  `docs/specs/ui/requirements/task-prompt-transcript-visibility.md`.
- Root-cause location in `apps/web/hooks/use-processed-messages.ts`.
- Existing launch-persistence coverage in
  `apps/web/e2e/tests/chat/kandev-system-wrap.spec.ts`.

## Risks

- Use `visibleMessages`, not the unfiltered message list.
- Do not use task-description equality to detect a real user prompt. Author
  type is the stable distinction.
- Do not change backend persistence as part of this task.

## Output contract

Report the condition change, tests added, exact commands and results, and any
remaining risks. Then mark this task as done and update its plan checkbox.

## Results

- Updated `useProcessedMessages` to keep the synthetic task description when
  visible history contains no user-authored message, while suppressing it for
  a visible stored user prompt and preserving the empty-description guard.
- Red: `cd apps && pnpm --filter @kandev/web test --
hooks/use-processed-messages.test.ts` failed at the agent-only fallback
  assertion before the production change (1 failed, 51 passed).
- Green: `cd apps && pnpm --filter @kandev/web test --
hooks/use-processed-messages-fallback.test.ts` passed (3 tests), and the
  existing processed-message suite remained green.
- Added coverage for agent-only history, stored user history, and an empty task
  description.
- Exported the synthetic row identity so transcript navigation and scroll
  anchoring can treat it as display-only, and split fallback visibility from
  message construction so streaming updates reuse the synthetic message
  object when its inputs are unchanged.
- Final web typecheck passed with `cd apps/web && pnpm run typecheck`.
