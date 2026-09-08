---
id: "05-localization-documentation"
title: "Localization documentation"
status: done
wave: 4
depends_on:
  - "01-frontend-locale-catalogs"
  - "02-backend-locale-negotiation"
  - "03-catalog-parity-gate"
  - "04-chinese-locale-e2e"
plan: "plan.md"
spec: "../../specs/platform/requirements/i18n.md"
---

# Task 05: Localization documentation

## Acceptance

- `docs/i18n.md` documents all steps for adding a real language, fixed endonym
  labels, synchronized frontend/backend locale lists, strict catalog validation,
  the generated pseudo boundary, and the partial-migration limitation.
- `docs/public/feature-status.md` tells users that English and 简体中文 are
  selectable while unmigrated surfaces remain English.
- Public-doc validation passes and the product spec matches implemented behavior.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
rg -n "zh-cn|简体中文|real language|真实语言" docs/i18n.md docs/public/feature-status.md docs/specs/platform/requirements/i18n.md
```

## Files likely touched

- `docs/i18n.md`
- `docs/public/feature-status.md`
- `docs/specs/platform/requirements/i18n.md`

## Dependencies

Tasks 01-04 provide the exact implemented behavior and browser evidence the docs
must describe.

## Parallelism

Sequential. Documentation follows actual implementation and verified behavior.

## Inputs

- Spec, completed task results, and plan **Documentation** section.
- Repository `docs-maintainer` workflow and existing public feature-status table.

## Output contract

Report docs impact, exact commands and results, files changed, terminology and
link checks, blockers/risks, and synchronized task/plan status.

## Results

- Updated `docs/i18n.md` with the shipped `zh-cn` locale, current namespaces,
  canonical locale ids, fixed endonym labels, synchronized frontend/backend
  registration, value-only translation rules, strict real-locale parity,
  focused tests, pseudo generation boundary, and manual reload/layout checks.
- Updated `docs/public/feature-status.md` with a Limited Web UI localization row:
  English and 简体中文 are selectable, while unmigrated surfaces remain English.
- The existing product spec already records the implemented Chinese selection,
  cookie, `Accept-Language`, shell, `Intl`, production visibility, and parity
  contracts.
- `node --test scripts/validate-public-docs.test.mjs`: 58/58 passed.
- `node scripts/validate-public-docs.mjs`: validated 41 published pages.
- The requested terminology search found the documented locale id and endonym
  across the guide, public status page, and product spec.
