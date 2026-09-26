---
id: "01-restore-focus"
title: "Restore editor focus on history Escape"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-REVERSE-SEARCH-FOCUS-001
acceptance_criteria:
  - AC-UI-REVERSE-SEARCH-FOCUS-001.1
  - AC-UI-REVERSE-SEARCH-FOCUS-001.2
  - AC-UI-REVERSE-SEARCH-FOCUS-001.3
system_design:
  - ../../specs/ui/system-design/reverse-search-focus.md
---

# Task 01: Restore Editor Focus on History Escape

## Summary

Make Escape dismissal of Ctrl+R message-history search return focus to the
composer that opened it. Prove task chat, Quick Chat, and phone Quick Chat
keyboard behavior with focused regression tests.

## In scope

- Route both search Escape handlers through an Escape-specific callback.
- Close search and focus the owning live TipTap editor without changing draft
  or selection behavior.
- Protect the existing outside-click, viewport-dismissal, dialog, and
  unrelated Escape behavior with targeted tests.

## Out of scope

- Search ranking, history fetching, result insertion, and overlay geometry.
- New mobile touch controls or copy.

## Acceptance

1. After Escape from search, the task chat or Quick Chat editor is focused and
   accepts the next typed character without a click.
2. The original draft remains intact, and the Quick Chat dialog stays open.
3. Outside-click and viewport dismissals keep their current close semantics;
   Escape cannot redirect focus from an unrelated composer.

## ASCII UI preview

UI-01 and UI-02 from the [full plan](plan.md#ascii-ui-preview) apply to
`AC-UI-REVERSE-SEARCH-FOCUS-001.1`, `.2`, and `.3`. Desktop and phone use the
same editor and search interaction; phone Quick Chat keeps its full-screen
dialog.

```text
UI-01 task chat:  [History search |] --Escape--> [Chat editor |]
UI-02 Quick Chat: [History search |] --Escape--> [Chat editor |]
                   (dialog open)                 (dialog open)
```

## Verification

Run from the repository root after the red test has failed as expected and the
implementation has been made. The E2E build is required because Playwright
serves the built web bundle through the Go backend.

```bash
(cd apps/web && pnpm test -- components/task/chat/message-history-search.test.ts)
make build-backend build-backend-linux-helpers build-web-e2e build-e2e-plugin-package
(cd apps/web && pnpm e2e:raw --project=chromium e2e/tests/chat/reverse-search-focus.spec.ts e2e/tests/chat/quick-chat.spec.ts --grep "reverse-search")
(cd apps/web && pnpm e2e:raw --project=mobile-chrome e2e/tests/chat/mobile-reverse-search-focus.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/chat/message-history-search.tsx components/task/chat/message-history-search.test.ts components/task/chat/tiptap-input.tsx components/task/chat/use-reverse-search-select-handler.ts e2e/tests/chat/reverse-search-focus.spec.ts e2e/tests/chat/quick-chat.spec.ts e2e/tests/chat/mobile-reverse-search-focus.spec.ts --max-warnings 0)
(cd apps/web && pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/ui docs/plans/reverse-search-focus apps/web
```

## Files likely touched

- `apps/web/components/task/chat/message-history-search.tsx`
- `apps/web/components/task/chat/tiptap-input.tsx`
- `apps/web/components/task/chat/use-reverse-search-select-handler.ts`
- `apps/web/components/task/chat/message-history-search.test.ts`
- `apps/web/e2e/tests/chat/reverse-search-focus.spec.ts`
- `apps/web/e2e/tests/chat/quick-chat.spec.ts`
- `apps/web/e2e/tests/chat/mobile-reverse-search-focus.spec.ts`

## Dependencies

None.

## Risks

- The Quick Chat focus trap may capture Escape before the search handler; keep
  its existing guard and verify behavior in a real browser.
- An editor may unmount while search closes; focus only the live editor owned
  by the same `TipTapInput`.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/ui/requirements/reverse-search-focus.md) and
  [system design](../../specs/ui/system-design/reverse-search-focus.md).
- Existing history-search tests and Quick Chat browser test.

## Results

- RED confirmed the component Escape callbacks and browser focus restoration failed before the implementation.
- `pnpm test -- components/task/chat/message-history-search.test.ts`: 11 passed.
- `make build-backend build-backend-linux-helpers build-web-e2e build-e2e-plugin-package`: passed. After the final frontend edit, `make build-web-e2e` passed.
- Desktop reverse-search E2E command: 3 passed. Mobile Quick Chat E2E command: 1 passed.
- `pnpm run typecheck`: passed.
- Focused ESLint command: passed with zero warnings.
- `pnpm run i18n:ratchet`: passed.
- `python3 scripts/list-docs.py validate`: passed; `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs/ui docs/plans/reverse-search-focus apps/web`: passed.
