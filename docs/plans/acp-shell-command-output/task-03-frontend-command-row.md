---
id: "03-frontend-command-row"
title: "Frontend command output row"
status: done
wave: 2
depends_on: ["01-backend-normalization"]
plan: "plan.md"
spec: "../../specs/ui/acp-shell-command-output.md"
---

# Task 03: Frontend Command Output Row

## Acceptance

- The expanded `ToolExecuteMessage` shows live/final combined output, explicit `Exit code N` or `Exit code unavailable`, and truncation state.
- Exit `0`, nonzero exit, ACP error, and unknown exit use success, failure, failure, and neutral semantics respectively; missing exit is never success.
- Focused component tests cover the four states without changing chat state or introducing a new API/hook.

## Verification

```bash
(cd apps && pnpm --filter @kandev/web test -- components/task/chat/messages/tool-execute-message.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web lint)
```

## Files likely touched

- `apps/web/components/task/chat/types.ts`
- `apps/web/components/task/chat/messages/tool-execute-message.tsx`
- `apps/web/components/task/chat/messages/tool-execute-message.test.tsx` (new)

## Dependencies

Task 01 defines the serialized optional-exit/truncation contract. This task can run in parallel with Task 02 afterward because their file ownership does not overlap.

## Inputs

- Spec scenarios for known success, known failure, unknown exit, live output, and truncation.
- Existing `ExpandableRow`, `useExpandState`, and `transformPathsInText` patterns in `tool-execute-message.tsx`.
- Mobile-parity constraints in `apps/web/AGENTS.md`.

## Output contract

Report rendered labels/status rules, tests run, files changed, blockers, and mobile layout risks. Set this task to `done` and update `plan.md` only after targeted tests, typecheck, and lint pass.

## Completion Report

- Added expandable live/final stdout and stderr, exact or unavailable exit labels, truncation state, and neutral unknown-exit semantics.
- Normalized ACP `in_progress` to the running UI state and treated cancellation as terminal without inventing an exit code.
- Changed the shell payload types, command-row component, and focused component tests.
- Component tests, web typecheck, lint, production build, and desktop/mobile Playwright coverage passed. No blockers remain; very long unbroken output relies on the tested wrapping and scroll bounds.
