---
id: "01-register-ko-locale"
title: "Register the Korean locale in backend and web negotiation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-I18N-001
acceptance_criteria:
  - AC-PLATFORM-I18N-001.4
  - AC-PLATFORM-I18N-001.7
system_design:
  - ../../specs/platform/system-design/i18n.md
---

# Task 01: Register the Korean locale

## Outcome

Users can select `한국어` in Settings. The choice persists, and the document language is `ko`. English UI copy remains the fallback until the Korean namespace catalog is delivered in a later PR.

## Scope

- Register `ko` in backend locale negotiation and its backend message catalog.
- Register `ko` in the web locale and date loaders; retain the fixed endonym in the existing switcher.
- Update existing backend, web, and language-switch E2E tests for this staged behavior.
- Exclude the 37-namespace Korean UI catalog, its key drift reconciliation, and AC-PLATFORM-I18N-001.3's complete-catalog list from this task.

## Acceptance

- The existing Settings selector offers `한국어` on desktop and phone without changing its layout.
- Selecting it persists the locale through reload and sets `<html lang="ko">`.
- UI messages lacking a Korean namespace catalog resolve through the English fallback.

## UI preview

UI-01 in the [plan](plan.md) shows the unchanged Settings language row with `한국어` selected. The same entry point remains available on phone.

## Verification

- `go test -p 2 ./internal/i18n/... ./internal/webapp/...` from `apps/backend`.
- Focused Vitest for `lib/i18n`, plus web `typecheck`, `lint`, and `i18n:check`.
- Targeted Playwright `e2e/tests/i18n/language-switch.spec.ts` Korean case on an isolated host instance, with a synthetic screenshot.
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all` after documentation edits.

## Result

The registration implementation and its targeted host E2E passed on the PR branch. Documentation validation is recorded separately after these files are added.
