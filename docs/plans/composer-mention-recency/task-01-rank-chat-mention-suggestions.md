---
id: "01-rank-chat-mention-suggestions"
title: "Rank chat mention suggestions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-COMPOSER-MENTION-RECENCY-001
acceptance_criteria:
  - AC-UI-COMPOSER-MENTION-RECENCY-001.1
  - AC-UI-COMPOSER-MENTION-RECENCY-001.2
  - AC-UI-COMPOSER-MENTION-RECENCY-001.3
  - AC-UI-COMPOSER-MENTION-RECENCY-001.4
  - AC-UI-COMPOSER-MENTION-RECENCY-001.5
  - AC-UI-COMPOSER-MENTION-RECENCY-001.6
  - AC-UI-COMPOSER-MENTION-RECENCY-001.7
  - AC-UI-COMPOSER-MENTION-RECENCY-001.8
system_design:
  - ../../specs/ui/system-design/composer-mention-recency.md
---

# Task 01: Rank Chat Mention Suggestions

## Summary

Add the bounded browser-local MRU helper and connect it to the shared TipTap
chat mention path. Prove the same recency-first order on desktop and phone.

## In scope

- Add normalized storage, identity, update, and rank helpers.
- Rank task, prompt, and returned file candidates by MRU position before text
  relevance.
- Preserve the Plan action's baseline position.
- Record pointer, touch, Enter, and Tab selections once.
- Add focused unit, desktop E2E, and phone E2E tests.

## Out of scope

- Backend, API, database, boot-payload, or WebSocket changes.
- New candidate sources or file-search limits.
- Other composer types and suggestion triggers.
- New copy or visual treatment.

## Acceptance

- With no valid history, the helper returns the current filtered order and Plan
  position.
- After selection, that candidate ranks before stronger text matches and stays
  first after a same-device reload.
- Desktop and phone use the same rank and preserve current insertion, focus,
  keyboard, touch, and popup behavior.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm test -- \
  lib/chat-mention-recency.test.ts \
  components/task/chat/tiptap-suggestion.test.ts
cd apps/web && pnpm e2e:run tests/chat/mention-recency.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome \
  tests/chat/mobile-mention-recency.spec.ts
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
```

## Files likely touched

- `apps/web/lib/chat-mention-recency.ts`
- `apps/web/lib/chat-mention-recency.test.ts`
- `apps/web/components/task/chat/tiptap-input.tsx`
- `apps/web/components/task/chat/tiptap-suggestion.tsx`
- `apps/web/components/task/chat/tiptap-suggestion.test.ts`
- `apps/web/e2e/tests/chat/mention-recency.spec.ts`
- `apps/web/e2e/tests/chat/mobile-mention-recency.spec.ts`

## Dependencies

None.

## Risks

- The async file source can return after task and prompt candidates. Rank the
  completed candidate set once and preserve deterministic ties.
- A storage error must not interrupt mention insertion.
- A missing workspace must not give file paths global recency.
- The test must prove recency over a stronger match, not only a source-order
  tie.

## Parallelism

`sequential`

## Inputs

- `docs/specs/ui/requirements/composer-mention-recency.md`
- `docs/specs/ui/system-design/composer-mention-recency.md`
- `apps/web/components/task/chat/tiptap-input.tsx`
- `apps/web/components/task/chat/tiptap-suggestion.tsx`
- `apps/web/lib/recent-tasks.ts` as the nearest browser-storage pattern
- Existing composer mention unit and mobile popup E2E tests

## Results

Implemented device-local MRU ranking and shared TipTap selection recording for
task, saved-prompt, and file mentions. Plan remains at its baseline position,
invalid storage is ignored, and the same ranking path serves desktop and phone.

Red-Green-Refactor evidence:

- The selection-callback test failed before the shared callback was wired.
- The helper tests failed before the new helper module was created.
- Both suites passed after implementation.
- PR fixup added a regression test for Plan's filtered baseline position. It
  failed before the fix and passed after the rank helper captured that index.

Verification:

- `cd apps && pnpm install --frozen-lockfile` passed.
- `cd apps/web && pnpm test -- lib/chat-mention-recency.test.ts components/task/chat/tiptap-suggestion.test.ts` passed (2 files, 25 tests).
- `cd apps/web && pnpm e2e:run tests/chat/mention-recency.spec.ts` passed (1 test).
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-mention-recency.spec.ts` passed (1 test).
- `cd apps/web && pnpm run typecheck` passed.
- `cd apps && pnpm --filter @kandev/web lint` passed.
