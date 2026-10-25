---
id: "04-e2e-and-docs"
title: "E2E coverage and documentation"
status: done
wave: 4
depends_on: ["03-frontend-copy-move-dialog"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/secret-scope-transfer.md"
---

# Task 04: E2E Coverage and Documentation

## Acceptance

- Desktop Chromium E2E covers: copy a Global secret to a workspace with the
  default suffixed name (both lists correct, source retained), move a
  workspace secret to General (source gone, destination present), move a
  Global secret to a workspace (source gone from the Global list, destination
  present, returned-item scope routing correct), and a duplicate target name
  blocked with the name field invalid.
- The desktop E2E actively proves plaintext never reaches the UI: after each
  copy/move flow, `expect(page.locator("body")).not.toContainText(<known
  value>)` is asserted (mirroring `repository-secrets.spec.ts`); known values
  are never asserted or displayed positively.
- The E2E flow submits through the dialog's own primary button (a labeled
  control inside the Copy/Move dialog that switches label with the selected
  mode, e.g. `Copy`/`Move`), waits for the transfer response or the returned
  list item to appear, and asserts the settings floating Save control was NOT
  used for the transfer (the `createSecretFromSettings` helper's
  floating-Save pattern is for page-draft creation only).
- The desktop spec's describe/title contains the literal `secrets-copy-move`
  fragment so the `--grep` verification command selects the tests; the
  direct spec-path run is documented as an alternative in the task's
  verification block.
- Mobile-chrome E2E covers opening the dialog at phone width, exercising the
  destination picker (including a Workspace → General switch), completing one
  copy and one move, and asserting the implemented mobile presentation: ≥44px
  touch targets for the radios, picker, name input, actions, and the dialog
  close button (bounding-box assertions mirroring
  `mobile-repository-secrets.spec.ts`), no horizontal overflow, single scroll
  owner, and focus return on close.
- `docs/public/agents-and-profiles.md` gains a short copy/move paragraph in the
  secret-scope section, and `docs/specs/INDEX.md` lists the new spec under
  `workspaces/`.

## Files likely touched

- `apps/web/e2e/tests/settings/secrets-copy-move.spec.ts` (new)
- `apps/web/e2e/tests/settings/mobile-repository-secrets.spec.ts` or a sibling
  mobile spec (new)
- `docs/public/agents-and-profiles.md`
- `docs/specs/INDEX.md`

## Inputs

- Task 03's dialog UI (selectors/labels from the i18n keys).
- Existing E2E conventions: `repository-secrets.spec.ts` helpers
  (`createSecretFromSettings`, API client fixtures, negative-value assertions),
  mobile-chrome project patterns from `mobile-repository-secrets.spec.ts`, and
  the grep semantics documented in `apps/web/e2e/README.md`.

## Dependencies

Task 03.

## TDD sequence

1. Write the desktop spec with the `secrets-copy-move` describe fragment and
   the negative plaintext assertions; run it after Task 03 lands.
2. Add the mobile spec with the picker interaction and geometry assertions.
3. Update the two docs files; run doc/i18n validation.

## Verification

```bash
cd apps/web && pnpm e2e:run -- --grep="secrets-copy-move"
cd apps/web && pnpm e2e:run -- --project=mobile-chrome --grep="copy/move|secrets-copy-move"
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
```

`e2e:run` performs the managed backend/web production build before running
(`e2e:raw` alone would launch whatever `apps/backend/bin/kandev` and
`apps/web/dist` already contain, which can be stale). If the repo runner
supports it, the direct spec-path form is the most reliable selector; use
whichever the runner accepts, and verify the grep actually matched (non-zero
test count).

## Risks

- E2E must assert via visible names/lists only and must never positively
  assert secret values; negative body assertions are required to catch leaks.
- The mobile project gates on the repo's mobile-chrome config; follow the
  existing mobile spec's structure exactly.
- Grep-based verification can silently match zero tests; confirm the describe
  fragment or use the spec path.
- Keep the docs paragraph short and consistent with the existing scope
  wording in `agents-and-profiles.md`.

## Output contract

Report the E2E specs added, results (including the grep match counts), docs
changes, and any residual risks.
