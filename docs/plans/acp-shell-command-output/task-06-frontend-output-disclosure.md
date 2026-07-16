---
id: "06-frontend-output-disclosure"
title: "Frontend shell output disclosure"
status: done
wave: 5
depends_on: ["05-backend-output-projection"]
plan: "plan.md"
spec: "../../specs/ui/acp-shell-command-output.md"
---

# Task 06: Frontend Shell Output Disclosure

## Acceptance

- The complete normalized command and working directory remain visible and wrap on desktop/mobile, while running and terminal output disclosures start collapsed and no output request occurs while collapsed.
- Opening fetches one snapshot; running output refreshes with one non-overlapping request at a time, one-second success cadence, and five-second maximum failure backoff. A projected terminal transition replaces active polling with one final snapshot; collapse or unmount aborts immediately.
- Loading, empty, populated stdout/stderr, unavailable/retry, truncation, known exit, and unknown exit states render from the snapshot without reading transcript bodies from message metadata.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- hooks/domains/session/use-shell-command-output.test.ts components/task/chat/messages/tool-execute-message.test.tsx
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
```

## Files likely touched

- `apps/web/lib/api/domains/session-api.ts`
- `apps/web/hooks/domains/session/use-shell-command-output.ts` (new)
- `apps/web/hooks/domains/session/use-shell-command-output.test.ts` (new)
- `apps/web/components/task/chat/types.ts`
- `apps/web/components/task/chat/messages/tool-execute-message.tsx`
- `apps/web/components/task/chat/messages/shell-output-disclosure.tsx` (new, if needed)
- `apps/web/components/task/chat/messages/tool-execute-message.test.tsx`

## Dependencies

Task 05 defines and tests the exact summary and snapshot endpoint contract.

## Inputs

- Spec `What`, `API surface`, failure scenarios, and desktop/mobile scenarios.
- ADR-0042 polling and component-local state decision.
- `apps/web/AGENTS.md` data-flow, component-size, accessibility, and mobile constraints.
- Existing `ToolExecuteMessage`, `normalizeToolCallStatus`, shared `fetchJson`, and path transformation behavior.
- Invoke `/mobile-parity` and `/tdd` while implementing this user-facing task.

## Output contract

Report the disclosure interaction, fetch/poll lifecycle, stale-response guards, status/empty/error rendering, desktop/mobile layout considerations, files changed, exact tests run, blockers, and residual risks. Set this task to `in_progress` before code and `done` with the matching `plan.md` checkbox only after tests, typecheck, and lint pass.
