---
id: "02-korean-ui-catalog"
title: "Ship the Korean UI translation catalog"
status: in_progress
wave: 2
depends_on:
  - "01-register-ko-locale"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-I18N-001
acceptance_criteria:
  - AC-PLATFORM-I18N-001.3
  - AC-PLATFORM-I18N-001.4
  - AC-PLATFORM-I18N-001.6
system_design:
  - ../../specs/platform/system-design/i18n.md
---

# Task 02: Ship the Korean UI translation catalog

## Outcome

Selecting `한국어` renders the web UI from a complete Korean catalog. All 37
`apps/web/src/locales/ko/*.json` namespaces mirror `en` key-for-key, and
`ko` joins the real locales that `pnpm run i18n:check` gates.

## Scope

- Add `apps/web/src/locales/ko/*.json` with the same namespaces, keys, and key
  order as `en`. Translate values only; keep keys, plural suffixes,
  `{{placeholders}}`, `<n>` Trans tags, command tokens, shortcuts, and brand
  names unchanged.
- Add `apps/web/src/locales/ko/_verbatim.json` for the few values that stay
  English in Korean, each with a reason.
- Relative-time phrases reuse the suffix the formatter already supplies
  (`5분 전`, `5분 후`) instead of adding a second one.
- Update the Korean rows in `lib/i18n/index.test.ts`, `lib/i18n/formats.test.ts`,
  and `e2e/tests/i18n/language-switch.spec.ts` from English fallback to the
  Korean catalog.
- List `ko` among the shipped and gated locales in `docs/i18n.md`, `AGENTS.md`,
  `apps/web/AGENTS.md`, the header comment of `scripts/check-i18n-keys.mjs`, the
  public feature status and web development pages, and the i18n requirement and
  system design.
- Exclude backend negotiation and the fixed switcher label, which Task 01
  delivered.

## Acceptance

- `i18n:check` reports `ko` complete: no missing or extra namespace or key, no
  dropped placeholder or tag, and no undeclared value identical to English.
- Activating `ko` (including a `ko-KR` request) loads the Korean `settings`
  bundle, and the Display language label reads `표시 언어`.
- Catalog-backed relative time and sidebar elapsed units render in Korean
  (`5분 전`, `3주`).

## UI preview

UI-01 in the [plan](plan.md) shows the Settings language row with the Korean
catalog active. The layout is unchanged on desktop and phone.

## Verification

- `pnpm run i18n:check` from `apps/web`.
- `pnpm exec vitest run lib/i18n` from `apps/web`.
- `pnpm run typecheck` and `pnpm run lint` from `apps/web`.
- `python scripts/list-docs.py validate` and `python scripts/lint-spec-files.py --all`
  from the repository root.
- E2E: the Korean case of `e2e/tests/i18n/language-switch.spec.ts` (selection,
  reload, and restoration to English) runs in CI. It was not run locally.

## Result

The catalog, test, and documentation changes are in the catalog PR. The
checks above except the E2E spec were run locally and passed. The E2E spec was
updated but not run locally, so this work order stays `in_progress` until the
E2E run passes in CI.
