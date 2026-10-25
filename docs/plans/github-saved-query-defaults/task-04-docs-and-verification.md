---
id: "04-docs-and-verification"
title: "Document and verify default views"
status: done
wave: 4
depends_on: ["03-default-view-controls"]
plan: "plan.md"
spec: "../../specs/ui/requirements/github-saved-query-defaults.md"
---

# Task 04: Document and Verify Default Views

## Acceptance

- Public GitHub integration docs explain how per-kind saved defaults behave.
- Focused unit, type, lint, i18n, desktop/mobile E2E, and docs checks pass.
- Spec, plan, and task statuses record completed behavior and exact results.

## Files

- `docs/public/integrations.md`
- `docs/specs/ui/requirements/github-saved-query-defaults.md`
- `docs/plans/github-saved-query-defaults/*`

## Verification

```bash
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint -- app/github/github-page-client.tsx components/github/my-github components/integrations/presets-scope-bar-base.tsx
cd apps && pnpm --filter @kandev/web i18n:check && pnpm --filter @kandev/web i18n:ratchet
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Result

- Focused Vitest: 34 tests across 5 files passed.
- TypeScript typecheck and targeted ESLint passed with zero warnings.
- i18n checks and new-code ratchet passed; pseudo locale regenerated.
- Desktop and mobile Playwright suites each passed 4 tests.
- Public-doc tests passed 58 tests; 41 published pages validated.
- `git diff --check` passed.
