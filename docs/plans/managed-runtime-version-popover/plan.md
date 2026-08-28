---
spec: docs/specs/agents/requirements/runtime-updates.md
created: 2026-08-22
status: completed
---

# Fix Plan: Anchored Managed Runtime Version Popover

## Root cause

`RuntimeVersionPicker` renders `RuntimeVersionBrowser` as a conditional block
inside the update body. The browser's command list therefore participates in
normal layout flow and expands the dialog or mobile drawer instead of behaving
like an anchored selector.

## Repair

Use the existing `@kandev/ui/popover` primitive around the version trigger and
render the searchable `Command` content in `PopoverContent`. Keep the existing
shared selection callbacks and close the popover after target selection. The
desktop dialog and mobile drawer must use the same popover state and catalogue;
only their trigger surface and available viewport geometry differ.

## Regression coverage

- Desktop: opening a long catalogue renders a `PopoverContent` outside the
  dialog's layout subtree and leaves the dialog height unchanged.
- Mobile: opening the same catalogue renders the popover outside the drawer's
  layout subtree, leaves the drawer height unchanged, keeps options at least
  44px high, and preserves selection.
- Existing searchable selection and preview callback coverage remains green.

## Files

- `apps/web/components/settings/runtime-version-picker.tsx`
- `apps/web/components/settings/agent-runtime-update-control.test.tsx`
- `apps/web/e2e/tests/settings/agent-runtime-update.spec.ts`
- `apps/web/e2e/tests/settings/mobile-agent-runtime-update.spec.ts`
- `docs/specs/agents/requirements/runtime-updates.md`

## Verification

Completed on 2026-08-22:

- Component tests: 6 passed.
- Desktop managed-runtime E2E: 15 passed.
- Mobile managed-runtime E2E: 5 passed.
- Web typecheck passed.
- Web i18n check passed.
- Changed-file ESLint passed.
- `git diff --check` passed.

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/settings/agent-runtime-update-control.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check
cd apps/web && pnpm e2e:run --project chromium e2e/tests/settings/agent-runtime-update.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/settings/mobile-agent-runtime-update.spec.ts
```

## Out of scope

- Changing version resolution, selection semantics, or update job behavior.
- Replacing the shared dialog, drawer, command, or popover primitives.
