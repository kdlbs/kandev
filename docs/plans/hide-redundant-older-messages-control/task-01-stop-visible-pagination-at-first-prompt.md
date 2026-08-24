---
id: "01-stop-visible-pagination-at-first-prompt"
title: "Stop visible pagination at the first prompt"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001
acceptance_criteria:
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.4
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.7
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.9
system_design:
  - ../../specs/ui/system-design/task-prompt-transcript-visibility.md
---

# Task 01: Stop Visible Pagination at the First Prompt

## Summary

Make the shared older-message hook stop at loaded prompt `#1`. Prove the behavior with hook tests and the existing desktop and mobile pagination flows.

## In scope

- Derive visible `hasMore` in `useLazyLoadMessages`.
- Update joined and completed request state with the same boundary.
- Stop the multi-page loop at prompt `#1`.
- Align the Prompt History panel with the shared boundary.
- Seed hidden pre-prompt rows in the shared browser fixture.

## Out of scope

- Backend, API, persistence, and schema changes.
- Changes to visible message filtering.
- Removal of the recovery control before the first prompt is known.
- Layout, copy, and translation changes.

## Acceptance

- Prompt `#1` makes the shared hook report no visible older history, even when raw `has_more` is true.
- Missing prompt ordinals keep the current raw-pagination fallback.
- Desktop and mobile show prompt `#1` without an older-page control while hidden pre-prompt rows remain unloaded.

## Verification

```bash
cd apps
pnpm --filter @kandev/web test -- \
  hooks/use-lazy-load-messages.test.ts \
  components/task/prompt-history-panel-content.test.tsx
pnpm --filter @kandev/web run typecheck
pnpm --filter @kandev/web lint
```

```bash
cd apps/web
pnpm e2e:run --project chromium tests/chat/message-pagination.spec.ts
pnpm e2e:run --project mobile-chrome tests/chat/mobile-message-pagination.spec.ts
```

## Files likely touched

- `apps/web/hooks/use-lazy-load-messages.ts`
- `apps/web/hooks/use-lazy-load-messages.test.ts`
- `apps/web/components/task/prompt-history-panel-content.tsx`
- `apps/web/components/task/prompt-history-panel-content.test.tsx`
- `apps/web/e2e/tests/chat/message-pagination-helpers.ts`
- `apps/web/e2e/tests/chat/message-pagination.spec.ts`
- `apps/web/e2e/tests/chat/mobile-message-pagination.spec.ts`

## Dependencies

None.

## Risks

- Keep raw store metadata separate from the visible pagination result.
- Recalculate the boundary before another page starts in one load operation.
- Keep the compatibility path for payloads without `prompt_index`.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001` and its acceptance criteria.
- The task transcript system design.
- The existing prompt-ordinal boundary in the Prompt History panel.
- The current desktop and mobile message-pagination tests.

## Results

- `pnpm --filter @kandev/web test -- hooks/use-lazy-load-messages.test.ts components/task/prompt-history-panel-content.test.tsx` — 60 tests passed.
- `pnpm --filter @kandev/web run typecheck` — passed.
- `pnpm --filter @kandev/web lint` — passed.
- `pnpm e2e:run --project chromium tests/chat/message-pagination.spec.ts` — 1 test passed.
- `pnpm e2e:run --project mobile-chrome tests/chat/mobile-message-pagination.spec.ts` — 1 test passed.
