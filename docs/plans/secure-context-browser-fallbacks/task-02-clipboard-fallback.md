---
id: "02-clipboard-fallback"
title: "Clipboard fallback migration"
status: done
wave: 2
depends_on: ["01-uuid-fallback"]
plan: "plan.md"
spec: "../../specs/auth/requirements/secure-context-browser-fallbacks.md"
---

# Task 02: Clipboard fallback migration

Make every web copy action tolerate an unavailable or rejected Clipboard API
by sharing the existing dialog-aware DOM fallback.

## Acceptance

- Modern `navigator.clipboard.writeText` is preferred when available.
- Missing or rejected Clipboard API requests use the DOM fallback and do not
  produce uncaught promise rejections or `TypeError`s.
- Existing copied indicators, toasts, and focus behavior remain intact across
  migrated call sites.
- No direct `navigator.clipboard.writeText(...)` calls remain in application
  source outside the shared utility and its tests.

## Files likely touched

- `apps/web/lib/utils/copy-to-clipboard.ts`
- `apps/web/lib/utils/copy-to-clipboard.test.ts`
- `apps/web/hooks/use-copy-to-clipboard.ts`
- `apps/web/hooks/use-copy-to-clipboard.test.ts`
- `apps/web/hooks/use-prompt-result-delivery.ts`
- `apps/web/hooks/use-prompt-result-delivery.test.ts`
- Direct-copy settings, onboarding, task, diff/editor, review, and share
  components listed in the clipboard section of `plan.md`.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run lib/utils/copy-to-clipboard.test.ts hooks/use-copy-to-clipboard.test.ts components/settings components/task components/editors components/review components/diff app/settings
cd apps/web && pnpm run typecheck
rtk rg -n --glob '!**/*.test.*' --glob '!**/e2e/**' 'navigator\.clipboard(?:\?|\.)?\.writeText\s*\(' apps/web
```

## Risks

The fallback is best effort on browsers that block `execCommand`; callers must
retain their current failure/toast semantics and must not report success when
the utility returns false.

## Results

- RED: the prompt-result delivery regression initially reported an error and
  skipped the DOM fallback when `navigator.clipboard` was unavailable.
- GREEN: `cd apps && pnpm --filter @kandev/web test -- --run lib/utils/copy-to-clipboard.test.ts hooks/use-copy-to-clipboard.test.ts hooks/use-prompt-result-delivery.test.ts`
  passed (3 files, 21 tests).
- The task-defined broader command passed (360 files, 2,647 tests, 4 skipped):
  `cd apps && pnpm --filter @kandev/web test -- --run lib/utils/copy-to-clipboard.test.ts hooks/use-copy-to-clipboard.test.ts hooks/use-prompt-result-delivery.test.ts components/settings components/task components/editors components/review components/diff app/settings`.
- `cd apps/web && pnpm run typecheck` passed.
- Targeted ESLint, Prettier, and `i18n:ratchet` checks passed.
- Static audit leaves direct Clipboard API access only in the shared utility and
  tests/E2E helpers.
