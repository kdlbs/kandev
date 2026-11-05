---
id: "02-maintenance-history"
title: "Clarify maintenance history"
status: done
wave: 2
depends_on:
  - "01-cache-scan-recovery"
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-GO-CACHE-005
acceptance_criteria:
  - AC-SYSTEM-PAGE-GO-CACHE-005.5
  - AC-SYSTEM-PAGE-GO-CACHE-005.6
  - AC-SYSTEM-PAGE-GO-CACHE-005.7
  - AC-SYSTEM-PAGE-GO-CACHE-005.8
  - AC-SYSTEM-PAGE-GO-CACHE-005.9
system_design:
  - ../../specs/system-page/system-design/go-cache-reclamation.md
---

# Task 02: Clarify maintenance history

## Summary

Show concise maintenance outcomes with consistent result typography. Keep the
original diagnostic message and JSON available through a separate disclosure.
Deliver the same information and controls on desktop and phone.

## In scope

- Add a pure history summary helper with explicit outcome precedence for every provider.
- Add unit cases for the incident, expected entry-limit progress, unknown/malformed
  errors, mixed Go progress and another provider's failure, cancellation, busy
  skips, legacy result envelopes, and unknown remaining bytes.
- Localize status and trigger labels. Show Incomplete for expected bounded progress,
  Partial for proven successful work with genuine errors, and Failed for other errors.
  Keep stored states unchanged. Unknown states retain a safe localized fallback.
- Deduplicate Go explanations and give unknown errors a concise generic explanation.
  Never treat retained issue samples as exact total counts.
- Apply local 14px body typography to facts and errors. Override inherited
  paragraph margins. Keep intentional metadata/badge sizing separate.
- Put raw message and JSON inside Technical details, initially closed. Preserve
  exact values and contain long text. Keep generic non-Go history results readable.
- Add component tests for default disclosure state, intact raw diagnostics, and keyboard access.
- Add desktop and mobile E2E cases in the existing maintenance specs, including
  measured type sizes, spacing, touch targets, and overflow with details open.
- Add all seven locale catalogs and regenerate pseudo and Traditional Chinese.
- Update `docs/public/operations.md` with history outcomes and the details control.
  Reuse existing guidance about partial deletion and possible cache growth.

## Out of scope

New run-state values, database migrations, retry buttons, shared accordion/card
changes, cleanup settings changes, and interactions with the user's live instance.

## Acceptance

1. Expected progress and actual failures are distinguishable. Any genuine provider error prevents a progress-only result.
2. Results show concise, deduplicated explanations at one body size. Original diagnostics remain intact and initially collapsed.
3. Desktop and phone users can operate every disclosure. Long errors stay contained, and localized labels and unknown measurements remain correct.

## ASCII UI preview

UI-02: Expanded desktop history result. See the [complete previews](plan.md#ascii-ui-preview).

```text
[Partial] Scheduled  Nov 3, 05:54                         [Collapse]
Go build cache
Removed: 10.75 GB             Remaining: 53.69 GB
Some cache entries changed during cleanup.
Other cleanup skipped: tasks were active.
Cleanup during active tasks was enabled.
[Technical details >]
```

UI-03: Phone uses one vertical reading sequence.

```text
[Partial] Scheduled                            [Collapse]
Nov 3, 05:54
Go build cache
Removed: 10.75 GB
Remaining: 53.69 GB
Some cache entries changed
during cleanup.
Other cleanup skipped:
tasks were active.
Cleanup during active tasks
was enabled.
[Technical details >]
```

UI-04: Progress uses Incomplete, an unknown remaining value, and one entry-limit
explanation. An unknown actual error uses Failed and a generic details prompt.
Empty, loading, and section-error states retain their current behavior.

Grouping and disclosure order are required. Spacing and English copy are
illustrative. The entry point and mobile exemplar are the existing storage
history and policy card. The page owns normal scrolling. Open technical details
owns its bounded diagnostic overflow. Phone disclosure targets measure at least
44px. No new overlay or navigation surface is needed.

These previews map to `AC-SYSTEM-PAGE-GO-CACHE-005.5` through `.9`.

## Verification

Run each command from the repository root. Install dependencies once for this
worktree before the first frontend command. Use `/tdd`, `/e2e`, `/mobile-parity`,
and `/docs-maintainer`. Run component regressions before production changes.
The first E2E command builds fresh production artifacts. Run projects sequentially.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/settings/system/storage/storage-run-summary.test.ts components/settings/system/storage/storage-run-history.test.tsx components/settings/system/storage/storage-overview-card-go-cache-result.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/settings/system/storage/storage-run-summary.ts components/settings/system/storage/storage-run-summary.test.ts components/settings/system/storage/storage-run-history.tsx components/settings/system/storage/storage-run-history.test.tsx components/settings/system/storage/storage-go-cache-result.tsx e2e/helpers/storage-maintenance.ts e2e/tests/system/storage-maintenance.spec.ts e2e/tests/system/mobile-storage-maintenance.spec.ts)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --project chromium -- tests/system/storage-maintenance.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome -- tests/system/mobile-storage-maintenance.spec.ts)
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Inspect desktop and phone screenshots against UI-02 and UI-03. Measure computed
14px text and compact paragraph spacing at desktop and phone widths. Include
767px and 768px checks for responsive geometry. Record the results and any
unresolved preview difference. Do not use class strings as geometry evidence.

## Files likely touched

- `apps/web/components/settings/system/storage/storage-run-summary.ts` (new)
- `apps/web/components/settings/system/storage/storage-run-summary.test.ts` (new)
- `apps/web/components/settings/system/storage/storage-run-history.tsx`
- `apps/web/components/settings/system/storage/storage-run-history.test.tsx`
- `apps/web/components/settings/system/storage/storage-go-cache-result.tsx`
- `apps/web/components/settings/system/storage/storage-overview-card-go-cache-result.test.tsx`
- `apps/web/e2e/tests/system/storage-maintenance.spec.ts`
- `apps/web/e2e/tests/system/mobile-storage-maintenance.spec.ts`
- `apps/web/e2e/helpers/storage-maintenance.ts`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-tw,zh-hk,ja,ko,pseudo}/system.json`
- `docs/public/operations.md`

## Dependencies

Task 01 supplies the final scan-invalidation reason. Historical fixture coverage
must also support the old repeated-message payload from before that correction.

## Risks

Any non-Go provider can fail while Go makes progress. Inspect the full provider
collection. Raw error strings and incomplete issue samples must not drive an
optimistic classification or an invented failure count. Keep typography overrides
inside storage history so other accordion consumers retain their current styles.

## Parallelism

`sequential`

## Inputs

- Design: Interface and reporting, Maintenance history presentation.
- Existing storage history component tests and desktop/phone maintenance E2E fixtures.
- [Combined previews and evidence](plan.md).

## Results

Implemented on 2026-11-03. `storage-run-summary.ts` derives one display outcome
from every provider result, keeps stored states and JSON unchanged, and
distinguishes Incomplete, Partial, Failed, and preserved states. The history row
uses localized status and trigger labels, compact 14px facts, one explanation per
distinct issue category, and an initially closed native Technical details
disclosure with contained overflow. The Go result component shows removed and
remaining or unknown bytes and deduplicates issue explanations. Review remediation
keeps the existing partial warning when no issue codes are retained, adds one
generic explanation for raw-only or uncovered errors, maps read and delete
failures to their own explanations, and folds legacy codes into the changed-entry
explanation only when parent-change evidence exists without an unrelated cause.
Non-Go provider failures remain visible on failed runs, and the technical-details
and temporary-artifact renderers moved to a separate feature component to keep
the history file within the web size limit.

Verification:

- `pnpm exec vitest run` for the new summary test, the history component test, and
  the Go result component test: 37 passed. The full storage component and
  maintenance hook run passed 180 tests.
- `pnpm run typecheck` passed. Changed-file ESLint reported no errors or warnings.
- `pnpm run i18n:zh-hant`, `pnpm run i18n:pseudo`, `pnpm run i18n:check`, and
  `pnpm run i18n:ratchet` passed with all seven catalogs updated.
- Desktop Chromium maintenance spec: 11 passed. Mobile Chrome maintenance spec:
  10 passed, including 767px and 768px geometry and 44px touch targets.

Remaining issue-closure gate: the mobile run uses browser and device emulation.
Physical-device validation was not available in this environment and remains
required before closing the phone acceptance criteria.
- `node scripts/validate-public-docs.mjs` validated 54 pages after
  `docs/public/operations.md` gained the history outcome guidance.
