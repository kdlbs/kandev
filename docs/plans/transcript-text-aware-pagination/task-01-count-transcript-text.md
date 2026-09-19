---
id: "01-count-transcript-text"
title: "Count transcript text during lazy loads"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001
acceptance_criteria:
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.3
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.7
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.9
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.11
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.13
system_design:
  - ../../specs/ui/system-design/task-prompt-transcript-visibility.md
---

# Task 01: Count transcript text during lazy loads

## Summary

Make the native transcript accumulate older raw pages until it loads 20 text
parts, without counting tool or other activity rows. Preserve existing
boundaries and prove the same one-reach behavior on desktop and mobile.

## In scope

- Add the text-part accumulation option and predicate to the shared lazy-load
  hook.
- Opt the native transcript into a target of 20 text parts.
- Add red-first unit coverage for mixed pages and all stop conditions.
- Add a distinct-turn tool-page fixture and desktop/mobile Playwright coverage.
- Preserve the existing prepend-anchor tolerance and successful-load recovery
  state.

## Out of scope

- Backend/API changes or a different raw page size.
- Prompt History, raw search, message drain, grouping, or rendering changes.
- New UI, copy, or responsive composition.

## Acceptance

- One transcript `loadMore` call continues past raw pages whose activity rows
  leave the 20-text-part target unmet.
- `message`, `content`, and legacy untyped rows advance the target; tool and
  other activity rows do not. Existing early stops still terminate safely.
- One desktop or mobile upward reach crosses a standalone tool page, reveals
  the older text marker, preserves its anchor, and does not show recovery UI.

## Verification

```bash
cd apps
pnpm install --frozen-lockfile
pnpm --filter @kandev/web test -- hooks/use-lazy-load-messages.test.ts
pnpm --filter @kandev/web run typecheck
cd web
pnpm exec eslint hooks/use-lazy-load-messages.ts hooks/use-lazy-load-messages.test.ts components/task/chat/message-list-native.tsx components/task/chat/message-list-native-scroll.ts components/task/chat/message-list-native.test.tsx e2e/tests/chat/message-pagination-helpers.ts e2e/tests/chat/message-pagination.spec.ts e2e/tests/chat/mobile-message-pagination.spec.ts
pnpm e2e:run --host --project chromium tests/chat/message-pagination.spec.ts -- --retries=0
pnpm e2e:run --host --no-build --project mobile-chrome tests/chat/mobile-message-pagination.spec.ts -- --retries=0
cd ../..
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/hooks/use-lazy-load-messages.ts`
- `apps/web/hooks/use-lazy-load-messages.test.ts`
- `apps/web/components/task/chat/message-list-native.tsx`
- `apps/web/components/task/chat/message-list-native-scroll.ts`
- `apps/web/components/task/chat/message-list-native.test.tsx`
- `apps/web/e2e/tests/chat/message-pagination-helpers.ts`
- `apps/web/e2e/tests/chat/message-pagination.spec.ts`
- `apps/web/e2e/tests/chat/mobile-message-pagination.spec.ts`

## Dependencies

None.

## Risks

- Tool-heavy histories can require several sequential requests within one
  load; retain the bounded loop.
- Joined flights may use a different effective page size.
- React can commit between pages, so anchoring and loading state must remain
  stable across the full operation.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001`, especially acceptance
  criteria `.3`, `.7`, `.9`, `.11`, and `.13`.
- Text-aware load batches, upward pagination, recovery, responsive behavior,
  and test boundaries in the system design.
- Existing `minUserPromptsPerLoad`, cursor coordination, and desktop/mobile
  message-pagination patterns.

## Results

- RED: the mixed tool/text hook case and desktop/mobile tool-page scenarios
  stopped after one raw request instead of reaching the 20-text target.
- GREEN: the native transcript now counts only `message`, `content`, and legacy
  untyped rows toward its 20-part batch while retaining all raw activity.
- GREEN: prompt `#1`, exhaustion, zero-result, and 10-page safety stops passed;
  aggregate loading and per-commit prepend anchoring passed focused tests.
- GREEN: 50 focused Vitest tests, typecheck, changed-file ESLint, the production
  E2E build, all 7 desktop pagination tests, and all 7 mobile pagination tests
  passed.
- GREEN: i18n and E2E sleep ratchets, 20 specification-linter tests, the full
  specification lint, and `git diff --check` passed.
