---
id: "03-catalog-parity-gate"
title: "Catalog parity gate"
status: done
wave: 2
depends_on: ["01-frontend-locale-catalogs"]
plan: "plan.md"
spec: "../../specs/platform/requirements/i18n.md"
---

# Task 03: Catalog parity gate

## Acceptance

- `i18n:check` discovers every committed real locale and fails for missing or
  extra namespaces and for missing or extra keys within a namespace.
- Errors name the locale, namespace, and specific drift; pseudo retains its
  generated strict-sync behavior and English is never regenerated.
- Temporary-fixture tests cover all four real-locale drift classes and the
  passing complete-catalog case.

## Verification

```bash
cd apps/web && pnpm exec vitest run scripts/check-i18n-keys.test.ts
cd apps/web && pnpm run i18n:check
```

Write the fixture tests first and confirm the current en/pseudo-only comparison
cannot satisfy them before changing the script.

## Files likely touched

- `apps/web/scripts/check-i18n-keys.mjs`
- `apps/web/scripts/check-i18n-keys.test.ts`
- `apps/web/scripts/lib/i18n-catalogs.mjs` if a pure helper keeps the CLI small

## Dependencies

Task 01 supplies the first real-locale directory and expected complete catalogs.

## Parallelism

Sequential because this task validates Task 01's catalog structure.

## Inputs

- Spec: real-locale parity scenario and resolved decision.
- Plan: **Frontend → Catalog integrity gate**.
- Existing pattern: script utilities with focused Vitest tests under
  `apps/web/scripts/`.

## Output contract

Report RED/GREEN evidence, fixture cases, actual complete-catalog output, exact
commands and results, files changed, blockers/risks, and synchronized task/plan
status.

## Results

- RED: `pnpm exec vitest run scripts/check-i18n-keys.test.ts` passed only the complete fixture and failed four assertions because missing/extra real-locale namespaces and keys were ignored.
- GREEN: `pnpm exec vitest run scripts/check-i18n-keys.test.ts` passed all 12
  fixture tests, including placeholder/tag drift and empty, whitespace-only, and
  non-string translated values.
- `pnpm run i18n:check` passed against current catalogs: 2,790 referenced keys,
  3,300 English entries, 4 orphans (pre-existing), `pseudo` and `zh-cn` in sync,
  53 `<Trans>` elements valid, and no inline plural or module-scope translation
  defects.
