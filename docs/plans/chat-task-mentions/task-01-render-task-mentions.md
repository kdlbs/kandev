---
id: "01-render-task-mentions"
title: "Render and navigate task mentions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-CHAT-TASK-MENTION-001
acceptance_criteria:
  - AC-UI-CHAT-TASK-MENTION-001.1
  - AC-UI-CHAT-TASK-MENTION-001.2
  - AC-UI-CHAT-TASK-MENTION-001.3
  - AC-UI-CHAT-TASK-MENTION-001.4
system_design:
  - ../../specs/ui/system-design/chat-task-mentions.md
---

# Task 01: Render and Navigate Task Mentions

## Summary

Implement distinct, clickable task chips using persisted task reference context
in sent user messages. Deliver parsing, Markdown integration, responsive
navigation, and targeted regression evidence in one pass with `/tdd`.

## In scope

- Bounded reference parser and message-local matching; no live-title lookup.
- TaskLink chip, composed prompt/entity rendering, focus and phone targets.
- Unit/component tests, desktop/phone E2E, and smallest relevant public docs.

## Out of scope

- Backend/persistence changes, composer search, assistant text, task previews.

## Acceptance

1. Recorded task references produce the UI-01 chip and exact canonical task
   destination across refresh/rename, on desktop keyboard and phone touch.
2. Invalid/ambiguous references fail closed, while raw/copy/agent input and
   existing Markdown, prompt, entity, and preview semantics remain intact.
3. Targeted tests pass with rendered evidence of distinct light/dark styling,
   phone containment, and nonoverlapping 44px touch targets.

## ASCII UI preview

UI-01: Sent task reference, normal chat bubble. Full
[preview and annotations](plan.md#ascii-ui-preview), AC .1 through .4.

```text
Desktop: check if [task-icon @Investigate retry] is fixing it?
Phone:   check if
         [task-icon @Investigate terminal ]
         [          turn capture retry   ]
         is fixing it?
Fallback: check if @something is fixing it?
```

The phone bracketed lines are one wrapping link. Keep the existing chat
scroller. Use compact desktop sizing and real phone/coarse-pointer targets.

## Verification

Run from repository root, with dependencies installed from `apps/` if absent.
E2E commands run sequentially and rebuild through the managed runner.

```bash
(cd apps/web && pnpm exec vitest run lib/tasks/task-mention-markdown.test.ts lib/tasks/task-mention-references.test.ts components/task/chat/messages/task-mention-components.test.tsx components/task/chat/messages/user-message-body.test.tsx components/task/chat/messages/prompt-mention-components.round2.test.tsx components/task/chat/messages/chat-message.test.tsx components/task/chat/messages/chat-message.woken-by.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=6144 pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm exec eslint --max-warnings 0 lib/tasks/task-mention-references.ts components/task/chat/messages/task-mention-components.tsx components/task/chat/messages/chat-message.tsx components/task/chat/messages/prompt-mention-components.tsx)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/chat/chat-task-mentions.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/chat/mobile-chat-task-mentions.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Record failing tests before production edits, passing results afterward, and
actual screenshot/geometry evidence. Add any other modified suites to the
targeted command before completion. Run PR-documentation coverage preflight
through `.github/scripts/pr-docs.cjs` against the final changed paths.

## Files likely touched

- `apps/web/lib/tasks/task-mention-references.ts` and `.test.ts` (new)
- `apps/web/lib/tasks/task-mention-markdown.ts` and `.test.ts` (new)
- `apps/web/package.json` and `apps/pnpm-lock.yaml` (explicit CommonMark parser dependency)
- `apps/web/components/task/chat/messages/task-mention-components.tsx` and `.test.tsx` (new)
- `apps/web/components/task/chat/messages/chat-message.tsx`
- `apps/web/components/task/chat/messages/prompt-mention-components.tsx`
- `apps/web/components/task/chat/messages/user-message-body.tsx` and `.test.tsx`
- `apps/web/e2e/tests/chat/chat-task-mentions.spec.ts` (new)
- `apps/web/e2e/tests/chat/mobile-chat-task-mentions.spec.ts` (new)
- Locale catalogs if accessible helper copy is added; public chat documentation

## Dependencies

None. Read the paired draft requirements and design after the explicit
implementation request; use existing `TaskLink` and message projection.

## Risks

Legacy text parsing ambiguity; Markdown component composition; touch targets
inside narrow bubbles. Do not expand storage or task navigation contracts to
solve these without revisiting the design.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/chat-task-mentions.md)
- [System design](../../specs/ui/system-design/chat-task-mentions.md)
- Existing prompt-mention component and chat E2E patterns.

## Results

- Red: recorded task identity extraction failed before implementation; composed
  message tests failed on missing task links; real desktop submission failed
  with zero chips instead of two. A rendered regression also reproduced the
  global Markdown underline overriding chip styling.
- Final targeted Vitest command above: **110 tests passed across seven suites**.
- Review regressions: literal Markdown task titles, shared normalization, real
  math versus literal-dollar prose, ambiguous serialized records, pointer styling,
  and UTF-8 byte budgets are covered. Additional regressions reject oversized
  records and retain valid siblings around long comma-free records and unmatched
  system openers. The malformed fixtures took 4.8 seconds before the parser fix;
  the forward scan and pre-regex record limit remove that repeated scanning. The desktop and phone fixtures select a
  task title containing literal Markdown.
- Typecheck: passed with `NODE_OPTIONS=--max-old-space-size=6144`. The initial
  default 4 GiB Node heap exhausted; the retry stayed within the guarded 8 GiB
  process-group budget. A test-only Testing Library option was corrected.
- ESLint: passed with zero warnings, including all new test and E2E files.
- `pnpm run i18n:check`: passed. Labels use submitted task data; no new
  translatable copy was necessary.
- Managed desktop Playwright command above: **1 passed** after the final style
  change. Proves canonical hrefs, refresh/rename identity, keyboard navigation,
  narrow fine-pointer touch sizing, ordinary-text fallback, and chip decoration.
- Managed phone Playwright: **1 passed**, using `--no-build` against the same
  freshly rebuilt web/backend artifacts from the desktop run. Measured 44px
  minimum targets, bubble containment, adjacent-chip separation, no document
  overflow, exact task navigation, and return navigation.
- Captured and inspected desktop light/dark plus phone screenshots in ignored
  `.pr-assets`; the task icon, tint, wrapped title, and direct link match UI-01.
- Specification catalog validation, full specification lint, PR-documentation
  coverage preflight, and diff checks passed.
- Public docs: added a how-to section to `tasks-and-workflows.md` explaining
  task-chip navigation and legacy/ambiguous fallback.
