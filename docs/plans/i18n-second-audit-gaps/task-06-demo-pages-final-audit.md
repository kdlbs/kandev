---
id: "06-demo-pages-final-audit"
title: "Localize demos and verify audit"
status: done
wave: 6
depends_on: ["05-isolated-ui-leaks"]
plan: "plan.md"
spec: "../../specs/platform/requirements/i18n-second-audit-gaps.md"
---

# Task 06: Demo Pages And Final Audit

## Acceptance

- All 21 demo-page lint violations in page chrome are localized, with count-based plurals and localized accessibility copy.
- Intentional agent-message fixture payloads remain verbatim and every remaining sweep candidate is categorized.
- Catalog parity, module-scope checks, guard lint, and focused rendered verification pass or have an exact recorded blocker.

## Likely Files

`apps/web/app/demo/agent-messages/page.tsx`, `app/demo/messages/page.tsx`, catalogs,
`apps/web/eslint.i18n.options.mjs`, and plan/task result sections.

## Verification

```bash
cd apps/web && pnpm run lint:i18n -- app/demo/agent-messages/page.tsx app/demo/messages/page.tsx && pnpm run i18n:check && pnpm run i18n:sweep -- app components hooks lib
```

## Risks

Fixture content is intentionally visible but not application chrome; document rather than translate those candidates.

## Results

All demo page-chrome violations are externalized with count-based plurals. Fixture payloads remain
verbatim. Required guard lint, catalog checks, sweep, typecheck, and focused tests passed with the
pre-existing advisories documented in the plan.
