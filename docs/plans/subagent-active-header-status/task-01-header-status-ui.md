---
id: "01-header-status-ui"
title: "Keep Working + spinner on every active header"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/subagent-observability.md"
---

# Task 01: Keep Working + spinner on every active header

## Intent

Make a live subagent card glanceable without expanding it: Working copy and spinner stay on the header even when the card has children, and a failed or cancelled finish gets a red mark. Successful completion gets a muted check.

## Acceptance

- `SubagentHeader` renders `t("task:working")` and `GridSpinner` whenever `isActive` is true, including when `hasExpandableContent` is true. The `showInlineWorking` gate is gone.
- When `isActive` is false and `normalizeToolCallStatus(metadata.status)` is `error` or `cancelled`, the header shows a red `IconX` (`h-3.5 w-3.5 text-red-500`) with `aria-label` `task:failed` or `task:cancelled`. A successful complete header shows a muted `IconCheck` with `aria-label` `task:statusCompleted`. Pending-settled headers show neither spinner, Working, nor X.
- Terminal nested payload statuses take precedence over running metadata in `isSubagentEffectivelyActive`. `SubagentMetaRow` and `!isActive` chip timing stay unchanged. No new locale keys.

## Regression tests (write first — must fail before the production change)

In `apps/web/components/task/chat/messages/tool-subagent-message.test.tsx`:

- Active + children: `metadataStatus: "running"` with two `childTool` messages. Assert `getByText("Working...")` and `getByRole("status", { name: "Loading" })`. Current code hides Working when children exist, so this fails first.
- Complete + children: `metadataStatus: "complete"` with children. Assert one `getByLabelText("Completed")`, no Working, and no Loading status.
- Failed: `metadataStatus: "failed"` (or `"error"`). Assert `getByLabelText("Failed")` and no Loading status.
- Cancelled: `metadataStatus: "cancelled"`. Assert `getByLabelText("Cancelled")` and no Loading status.
- Terminal nested payloads: `payloadStatus: "errored"` overrides running metadata and `payloadStatus: "failed"` or `"cancelled"` overrides complete metadata.

Reuse `subagentMessage` / `renderSubagent` / `childTool`. Do not invent a Done chip assertion.

## Files likely touched

- `apps/web/components/task/chat/messages/tool-subagent-message.tsx`
- `apps/web/components/task/chat/messages/tool-subagent-message.test.tsx`

## Dependencies

None.

## Parallelism

Sequential. Component and unit tests share the same files.

## Inputs

- Spec: `### Header status` and the three new header-status scenarios.
- Plan: Frontend and Tests.
- Pattern: `ExecuteStatusIcon` in `tool-execute-message.tsx` (spinner / red X / no check for this card). Import `normalizeToolCallStatus` from `./tool-status` or `@/lib/utils/tool-call-status`.

## Verification

```sh
cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- components/task/chat/messages/tool-subagent-message.test.tsx && cd web && NODE_OPTIONS="--max-old-space-size=6144" pnpm run typecheck && pnpm exec eslint components/task/chat/messages/tool-subagent-message.tsx components/task/chat/messages/tool-subagent-message.test.tsx && pnpm exec prettier --check components/task/chat/messages/tool-subagent-message.tsx components/task/chat/messages/tool-subagent-message.test.tsx
```

## Output contract

Summary, files changed, tests run, blockers, risks, and this task plus `plan.md` status/results updated in the same conversation.

## Results

Done. Removed `showInlineWorking`. Active headers always show `task:working` + `GridSpinner`. Terminal nested failures and cancellations now override stale metadata. Failed/cancelled headers show a labelled red `IconX`. Complete headers show a labelled muted check.

- Unit tests: 52 passed (`tool-subagent-message.test.tsx`). The regression run failed 4 cases before the status precedence fix.
- Typecheck / eslint / prettier: pass.
