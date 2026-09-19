---
id: "06-docs-and-e2e"
title: "Docs update and locale switcher E2E"
status: done
wave: 3
depends_on:
  - "02-register-locales-runtime"
  - "03-generate-web-catalogs"
  - "04-backend-catalogs"
  - "05-high-traffic-review"
plan: "plan.md"
spec: "../../specs/platform/requirements/traditional-chinese-locales.md"
---

# Task 06: Docs update and locale switcher E2E

## Intent

Document the new shipped locales and prove Settings language switching to
`zh-tw` / `zh-hk` works end-to-end (label, Traditional chrome, `lang`, cookie
reload).

## Acceptance

- `docs/i18n.md` and `docs/specs/platform/requirements/i18n.md` list `zh-tw` and `zh-hk` as
  shipped human locales (and switcher endonyms).
- E2E covers selecting each Traditional locale, asserting
  `document.documentElement.lang`, and at least one stable Traditional UI
  string (not English fallback for a known complete key).
- Spec status for `traditional-chinese-locales.md` can move to `building` or
  `shipped` when this task completes with the rest; `docs/specs/INDEX.md`
  already lists the feature.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm run i18n:check
cd apps/web && pnpm e2e:run tests/settings/locale-switcher-zh-hant.spec.ts
# adjust path if the suite extends an existing appearance/locale spec instead
```

## Files likely touched

- `docs/i18n.md`
- `docs/specs/platform/requirements/i18n.md`
- `docs/specs/platform/requirements/traditional-chinese-locales.md` (status)
- `apps/web/e2e/tests/settings/*locale*` (new or extended)
- Possibly settings appearance component test if pure unit coverage is added

## Dependencies

- Runtime registration, catalogs, and high-traffic review so E2E does not
  assert English fallbacks for chrome strings.

## Parallelism

Sequential (final gate).

## Inputs

- Spec scenarios for switcher + cookie.
- Existing e2e patterns under `apps/web/e2e/tests/settings/`.

## Output contract

- Docs aligned; E2E green; plan Verification Results filled; status updated.

## Results

- Glossary documentation now records the reviewed regional software terms and
  the final override/validation contract.
- Managed host E2E rebuilt backend and Vite production assets, then passed all
  5 language-switch scenarios, including `zh-tw` and `zh-hk` reload
  persistence.
