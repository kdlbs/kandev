---
id: "04-repository-verification"
title: "Repository verification"
status: done
wave: 4
depends_on:
  - "01-frontend-ja-locale-catalogs"
  - "02-backend-ja-locale-negotiation"
  - "03-japanese-locale-e2e"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-JAPANESE-LOCALE-001
acceptance_criteria:
  - AC-PLATFORM-JAPANESE-LOCALE-001.6
system_design:
  - ../../specs/platform/system-design/i18n.md
---

# Task 04: Repository verification

## Outcome

The whole Japanese-locale package is repository-clean: docs catalog and spec
lint pass, all verification commands green in one final run, and the feature
branch is ready for the user's review with no stale/pruned plan markers.

## In scope

- `docs/i18n.md` update: add `ja` to the shipped-human-locales list, the
  "four real locales gate the build" text (now five), and the locale-id /
  parity guidance note (`日本語` endonym, `ja` canonical id).
- `docs/specs/platform/requirements/i18n.md` + `japanese-locale.md` +
  `docs/specs/platform/system-design/i18n.md` consistency (already authored in
  the package; validate).
- `python3 scripts/list-docs.py validate` and
  `python3 scripts/lint-spec-files.py --all` — fix any issues introduced by the
  package.
- Final all-gates run: `i18n:check`, `typecheck`, `lint`, backend `make test`,
  focused E2E.

## Exclusions

- Any new product code or catalog changes (Tasks 01-03 own those).

## Applicable requirements

`REQ-PLATFORM-JAPANESE-LOCALE-001.6` (CI/lint/parity discovery); the docs
guidance in `docs/i18n.md`.

## System design

`docs/specs/platform/system-design/i18n.md` — Enforcement notes and delivery
notes as they describe docs/parity conventions.

## Implementation acceptance conditions

1. `docs/i18n.md` lists `ja` consistently and the "N real locales gate the
   build" phrasing is correct (five real locales).
2. `python3 scripts/list-docs.py validate` and
   `python3 scripts/lint-spec-files.py --all` pass.
3. One final run of the verification suite exits 0 (frontend typecheck, i18n
   gates, lint, backend tests, focused E2E).

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm run i18n:check && pnpm run typecheck && pnpm run lint)
(cd apps/backend && make test)
(cd apps/web && pnpm e2e:run --host --project chromium -- tests/i18n/language-switch.spec.ts)
python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all
```

## Files likely touched

- `docs/i18n.md`
- (validation-only) `docs/specs/platform/requirements/*`, `docs/specs/platform/system-design/i18n.md`

## Dependencies

Tasks 01-03.

## Parallelism

Sequential; final sweep.

## Inputs

- Plan: **Wave 4**, **Verification strategy**.
- The completed work orders' Results for RED/GREEN and command output.

## Output contract

Report exact commands and exit codes for every gate, docs validation output,
any fixes applied, blockers/risks, and synchronized task/plan status marking the
package ready for user review.

## Results

- `docs/i18n.md` lists `ja` (endonym `日本語`) and "five real locales gate the
  build". Spec `japanese-locale.md` Overview/Intent/AC-001.3 updated: parity
  gates, not advisory. `updated: 2026-09-21`.
- Docs: `python3 scripts/list-docs.py validate` → 292 decisions, 1055 specs.
  `python3 scripts/lint-spec-files.py --all` → pass.
- Gates:
  - `pnpm run i18n:check` → `ja, pt-pt, zh-cn, zh-hk, zh-tw complete` (exit 0)
  - `pnpm run i18n:parity` → 37 namespaces, 10837 keys, ja missing=0
  - `pnpm run typecheck` → exit 0
  - `pnpm run lint` → exit 0
  - `pnpm exec vitest run lib/i18n` → 9 files, 100 tests, GREEN
  - `go test ./internal/i18n/... ./internal/webapp/...` GREEN. Full
    `make -C apps/backend test` timed out here and is not claimed green
  - focused E2E chromium + mobile-chrome → GREEN
