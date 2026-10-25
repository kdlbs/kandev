---
id: "01-timeout-feedback"
title: "Concise temporary-folder timeout feedback"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-003
acceptance_criteria:
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-003.6
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-003.8
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-003.10
system_design:
  - ../../specs/system-page/system-design/storage-analysis-presentation.md
---

# Task 01: Concise temporary-folder timeout feedback

## Summary

Replace repeated deadline messages with one localized explanation in the expanded temporary-folder row.
Preserve sampled bytes, partial state, skipped counts, and bounded non-deadline diagnostics.

## In scope

- TDD tests for repeated and mixed joined errors, multiple roots, counts, and cancellation.
- Normalize warning leaves at the tempstore projection boundary.
- Localize the existing deadline reason and normalize older warning payloads in the resource projection.
- Desktop and mobile browser assertions for bounded timeout feedback.
- Update the temporary-folder explanation in public operations guidance during implementation.

## Out of scope

Bars, sorting, new scanner workers, scan deadline changes, scan-performance work, cleanup, and new API fields.

## Acceptance

1. Deadline results retain sampled bytes and counts, with at most ten distinct non-deadline diagnostic examples.
2. Both new and older snapshots show one localized timeout explanation. Mixed joined errors retain unrelated diagnostics.
3. Desktop and phone users can inspect partial details without repeated deadline text or horizontal overflow.

## ASCII UI preview

UI-01 and UI-02 excerpt: expanded temporary-folder details at Settings > System > Storage.
See the [combined preview](plan.md#ascii-ui-preview). Bars belong to Task 02.

```text
Desktop:
System temporary folders                      51.08 GB  ^
  Read-only. This footprint can overlap counted categories.
  /tmp: 51.08 GB | Partial | 1,015 entries skipped
  Scan timed out. Showing partial usage.
  <bounded distinct diagnostic examples>

Phone:
System temporary folders                    ^
51.08 GB
  Read-only. This footprint can
  overlap counted categories.
  /tmp: 51.08 GB
  Partial | 1,015 entries skipped
  Scan timed out.
  Showing partial usage.
```

Maps to AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-003.6, .8, .10.
Existing inline expansion and one page scroll owner remain required.
Copy and paths wrap within the row. Example numbers and spacing are illustrative.

## Verification

Run from the repository root. Install workspace dependencies once before the first pnpm command.
Use deterministic injected scanner results instead of waiting for the production deadline.
Write the focused regression tests first and observe their failures before changing production logic.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test ./internal/system/storage/tempstore ./internal/system/storage/filescan)
(cd apps/web && pnpm test components/settings/system/storage/storage-overview-card.test.tsx components/settings/system/storage/storage-totals.test.ts)
(cd apps/web && pnpm exec eslint components/settings/system/storage/storage-overview-resources.ts components/settings/system/storage/storage-overview-card.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/system/storage-temporary-folders.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/system/mobile-storage-temporary-folders.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Before the final checks, add translations in en, pt-pt, zh-cn, and ja, then generate zh-hk and zh-tw.
Extend existing temporary-folder browser cases with a controlled deadline payload and one unrelated warning.
Assert one explanation, retained sampled size/counts, wrapped long paths, and no raw deadline repetitions.
Capture and inspect the desktop and phone detail region through `prCapture`.

## Files likely touched

- `apps/backend/internal/system/storage/tempstore/provider.go`
- `apps/backend/internal/system/storage/tempstore/provider_test.go`
- `apps/web/components/settings/system/storage/storage-overview-resources.ts`
- `apps/web/components/settings/system/storage/storage-overview-card.test.tsx`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja}/system.json`
- `apps/web/e2e/helpers/storage-maintenance.ts`
- `apps/web/e2e/tests/system/storage-temporary-folders.spec.ts`
- `apps/web/e2e/tests/system/mobile-storage-temporary-folders.spec.ts`
- `docs/public/operations.md`

## Dependencies

None.

## Risks

- `errors.Is` on a joined error matches any deadline leaf. Do not discard the entire mixed error.
- Old cached warnings can contain many newline-separated deadline messages in one string.
- Existing cancellation and strict scanner consumers must retain their behavior.

## Parallelism

`sequential`. This order shares frontend and browser helper files with Task 02.

## Inputs

- [Requirements](../../specs/system-page/requirements/storage-maintenance.md), section 003.
- [Design](../../specs/system-page/system-design/storage-analysis-presentation.md), Deadline feedback.
- Existing `TestAnalyzeTemporaryDeadlineReturnsPartialSample` and temporary-folder component/browser cases.
- Scoped backend and web `AGENTS.md`, plus TDD, E2E, mobile-parity, and docs-maintainer skills.

## Results

- `go test ./internal/system/storage/tempstore ./internal/system/storage/filescan`: passed.
- Focused Storage component tests: passed, including the localized timeout and retained diagnostics case.
- Existing desktop and mobile temporary-folder browser cases: passed with one timeout explanation,
  retained partial values and counts, unrelated diagnostics, and no horizontal overflow.
- Public operations guidance now documents partial timeout samples and their non-cleanup meaning.
