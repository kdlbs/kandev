---
id: "01-format-byte-progress"
title: "Format startup byte counts and rates"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-STARTUP-PROGRESS-001
  - REQ-PLATFORM-STARTUP-PROGRESS-002
  - REQ-PLATFORM-STARTUP-PROGRESS-003
acceptance_criteria:
  - AC-PLATFORM-STARTUP-PROGRESS-001.5
  - AC-PLATFORM-STARTUP-PROGRESS-001.6
  - AC-PLATFORM-STARTUP-PROGRESS-002.4
  - AC-PLATFORM-STARTUP-PROGRESS-003.8
  - AC-PLATFORM-STARTUP-PROGRESS-003.14
  - AC-PLATFORM-STARTUP-PROGRESS-003.15
  - AC-PLATFORM-STARTUP-PROGRESS-003.16
system_design:
  - ../../specs/platform/system-design/startup-progress-visibility.md
---

# Task 01: Format startup byte counts and rates

## Summary

Show the same readable byte sizes in every existing startup renderer.
Show byte rates with scaled units in terminal progress.
Use TDD and the existing shared web formatter's policy.

## In scope

- Shared Go byte formatters, launcher output, initial HTML, embedded poll JavaScript, and the React formatter.
- Localized templates and their inclusion in the page's embedded translation map.
- Focused numeric, renderer, raw-payload, desktop, and mobile regression evidence.
- A brief display-unit explanation in the CLI reference.

## Out of scope

Backup behavior, schema changes, new dependencies, layout changes, and native desktop shell integration.

## Acceptance

1. Implement the [readable byte units design](../../specs/platform/system-design/startup-progress-visibility.md#readable-byte-units) across the four renderers.
2. Prove unit boundaries, rounding parity, byte-rate labels, raw payloads, and existing non-byte and opaque behavior.
3. Prove readable desktop and phone sizes, complete localization, and matching public guidance in the real page and restart dialog.

## ASCII UI preview

UI-01: Startup backup progress, shared desktop and phone composition.
See the [combined preview](plan.md#ascii-ui-preview) and AC-PLATFORM-STARTUP-PROGRESS-003.14 through 003.16.

```text
Database backup
14.3 GB
```

The terminal detail adds `14.3 GB (1.0 MB/s)` when that rate is present.
Keep the existing hierarchy and scroll behavior.
The value and unit must fit the viewport. Sample numbers and spacing are illustrative.
Use the shipped mobile startup flow as the exemplar and rendered check.

## Verification

Start from the repository root. Install workspace dependencies once before the first pnpm command in a fresh worktree.
Run the focused tests in RED before production changes, then in GREEN after implementation.
Keep every existing case except assertions that intentionally specify raw byte display.
Retain plural coverage with a non-byte unit when an existing byte assertion changes.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test -trimpath ./internal/startup)
(cd apps/backend && go test -trimpath ./internal/launcher -run 'TestFormatStartupLine')
(cd apps/backend && go test -trimpath ./internal/backendapp -run 'Test(BuildStartupPageView|WriteStartupPage|StartupPage|E2EStartupPageFixture|NewBootstrapHandler.*Ready)')
(cd apps/backend && go test -trimpath ./internal/i18n)
(cd apps/web && pnpm exec vitest run lib/startup-progress/format.test.ts components/settings/system/restart-progress-dialog.test.tsx)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings lib/startup-progress/format.ts lib/startup-progress/format.test.ts e2e/tests/layout/startup-page.spec.ts e2e/tests/layout/mobile-startup-progress.spec.ts)
bash scripts/lint-go-changed apps/backend/internal/startup/bytes.go apps/backend/internal/startup/bytes_test.go apps/backend/internal/startup/snapshot_wire_test.go apps/backend/internal/launcher/health.go apps/backend/internal/launcher/health_startup_bytes_test.go apps/backend/internal/backendapp/startup_page.go apps/backend/internal/backendapp/startup_page_test.go apps/backend/internal/backendapp/startup_page_bytes_test.go
(cd apps/web && pnpm e2e:run --project chromium tests/layout/startup-page.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/layout/mobile-startup-progress.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Each E2E command builds current artifacts. Run them sequentially.
Confirm that each project discovers the intended tests.
Record the RED and GREEN outcomes and inspect the rendered phone capture against UI-01.
If implementation needs a different test file, update this command list before completion.
Run the PR documentation coverage preflight with the final changed paths and referenced documents.

## Files likely touched

Ownership covers only this vertical display change:

- New `apps/backend/internal/startup/bytes.go` and `bytes_test.go`.
- `apps/backend/internal/startup/snapshot_wire_test.go`.
- `apps/backend/internal/launcher/health.go` and `health_startup_bytes_test.go`.
- `apps/backend/internal/backendapp/startup_page.go`, `startup_page_test.go`, and `startup_page_bytes_test.go`.
- `apps/web/lib/startup-progress/format.ts` and `format.test.ts`.
- Backend `internal/i18n/locales/*.json` and web `src/locales/*/startup.json`, including generated catalogs.
- `apps/web/e2e/tests/layout/startup-page.spec.ts` and `mobile-startup-progress.spec.ts`.
- `docs/public/cli.md`, the referenced requirement/design pair, and this plan package.

The existing restart component consumes `formatStepProgress` without a markup change.
Its existing component tests remain part of verification.

## Dependencies

None. The shared web formatter, real-page fixture route, and phone restart scenario already exist.

## Risks

The [plan](plan.md#risks) identifies rounding, poll-renderer, and locale-map risks.
New tests must exercise each implementation of the policy.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/platform/requirements/startup-progress-visibility.md), especially AC-PLATFORM-STARTUP-PROGRESS-003.14 through 003.16.
- [System design](../../specs/platform/system-design/startup-progress-visibility.md#readable-byte-units).
- Existing `formatBytes`, launcher progress tests, Go page tests, and desktop/mobile startup E2E flows.
- Root and scoped `AGENTS.md`, `/tdd`, `/mobile-parity`, `/e2e`, and `/docs-maintainer`.

## Results

Implemented on 2026-11-10 in the task worktree.

- RED: `go test ./internal/startup` failed on undefined `FormatBytes` and `FormatByteRate`;
  `vitest run lib/startup-progress/format.test.ts` failed the 11 new byte cases against the
  raw-count renderer.
- Added `startup.FormatBytes` and `startup.FormatByteRate` with the shared 1024-based policy
  and a TB cap.
- Corrected a review finding on rounding parity. The first implementation used
  `floor(value*10+0.5)/10`, whose preliminary float multiply can move a value just below a
  midpoint onto it: `4504644163416883` bytes printed `4097.0 TB` in Go and the embedded
  script while the web helper printed `4096.9 TB`. Go now rounds the exact double through
  `math/big.Rat`, and the embedded script calls `toFixed(1)` directly. A 5,011-value
  cross-check of `FormatBytes` against the web helper produced zero mismatches.
- Launcher output, initial page HTML, and the embedded poll script now format `UnitBytes`; the
  React formatter reuses `formatBytes`. Bars and ETA math still use raw counts, and the
  readiness payload keeps integer bytes and bytes per second.
- Added `startup.page.countingBytes` and `startup.page.countedBytes` to the backend and web
  catalogs in all seven locales. Traditional Chinese came from `i18n:zh-hant` (backend run with
  `--backend`) and the pseudo catalog from `i18n:pseudo`.
- Every documented command and its result:
  - `(cd apps && pnpm install --frozen-lockfile)`: already up to date.
  - `(cd apps/backend && go test -trimpath ./internal/startup)`: ok.
  - `(cd apps/backend && go test -trimpath ./internal/launcher -run 'TestFormatStartupLine')`: ok.
  - `(cd apps/backend && go test -trimpath ./internal/backendapp -run 'Test(BuildStartupPageView|WriteStartupPage|StartupPage|E2EStartupPageFixture|NewBootstrapHandler.*Ready)')`: ok.
  - `(cd apps/backend && go test -trimpath ./internal/i18n)`: ok.
  - `(cd apps/web && pnpm exec vitest run lib/startup-progress/format.test.ts components/settings/system/restart-progress-dialog.test.tsx)`: 39 tests passed.
  - `(cd apps/web && pnpm run i18n:zh-hant)`: wrote both web zh-Hant locales. The backend pair was
    generated separately with `node scripts/convert-zh-cn-to-zh-hant.mjs --locale all --backend --write`.
  - `(cd apps/web && pnpm run i18n:pseudo)`: 38 web namespaces and 83 backend messages.
  - `(cd apps/web && pnpm run i18n:check)`: passed (10,381 references, 13,760 English entries,
    605 pre-existing orphans, pseudo in sync, all real locales complete).
  - `(cd apps/web && pnpm run typecheck)`: passed.
  - `(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings ...)`: 0 issues.
  - `bash scripts/lint-go-changed <changed Go files>`: 0 issues.
  - `(cd apps/web && pnpm e2e:run --project chromium tests/layout/startup-page.spec.ts)`: 2 passed,
    including the `4096.9 TB` poll update from the embedded renderer.
  - `(cd apps/web && pnpm e2e:run --project mobile-chrome tests/layout/mobile-startup-progress.spec.ts)`: 2 passed.
    A `CAPTURE_PR_ASSETS=true` rerun produced `restart-byte-progress-mobile` and
    `startup-byte-progress-mobile`; both match UI-01 with no overflow and no bar.
  - `node --test scripts/validate-public-docs.test.mjs`: 63 tests passed.
  - `node scripts/validate-public-docs.mjs`: 55 pages validated.
  - `python3 scripts/list-docs.py validate`: 446 decisions and 1963 specifications validated.
  - `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
  - `python3 scripts/lint-spec-files.py --all`: passed.
  - `git diff --check`: clean.
- Regenerating pseudo and zh-Hant also rewrites unrelated pre-existing drift in `task.json`
  namespaces; those edits were reverted so the diff carries only the startup keys.
- The updated requirement prose was revalidated with both specification gates after the edit.
