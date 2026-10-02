---
id: "04-storage-feedback"
title: "Storage policy and feedback"
status: done
wave: 3
depends_on:
  - "02-busy-cleanup"
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-GO-CACHE-005
  - REQ-SYSTEM-PAGE-GO-CACHE-004
acceptance_criteria:
  - AC-SYSTEM-PAGE-GO-CACHE-005.1
  - AC-SYSTEM-PAGE-GO-CACHE-005.2
  - AC-SYSTEM-PAGE-GO-CACHE-005.3
  - AC-SYSTEM-PAGE-GO-CACHE-005.4
  - AC-SYSTEM-PAGE-GO-CACHE-005.5
  - AC-SYSTEM-PAGE-GO-CACHE-004.2
  - AC-SYSTEM-PAGE-GO-CACHE-004.4
  - AC-SYSTEM-PAGE-GO-CACHE-004.6
  - AC-SYSTEM-PAGE-GO-CACHE-004.7
system_design:
  - ../../specs/system-page/system-design/go-cache-reclamation.md
---

# Task 04: Storage policy and feedback

## Summary

Expose the persisted busy policy, its build-failure warning, and accurate results on desktop and phone.

## In scope

Web types, domain hooks, existing Go policy/analysis/history, seven-language copy, desktop/phone E2E, and public operations documentation.

## Out of scope

Storage redesign, consumer counters, generation displays, and unrelated quarantine controls.

## Acceptance

- Users save and reload the off-by-default switch with a visible warning and no added typed confirmation.
- With the option enabled, UI cleanup deletes real fixture files while a mock task remains active, without quarantine or a repeated force prompt.
- Desktop/phone results preserve partial states and permissions; documentation states the actual interruption and scheduling tradeoff.

## Verification

Use TDD for changed logic. Commands start from the repository root.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/settings/system/storage hooks/domains/system/use-storage-maintenance.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/system/storage-maintenance.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/system/mobile-storage-maintenance.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run E2E commands sequentially with fresh managed builds. Restore saved settings and fixture files. Test the off state, warning, saved on state, active-task deletion, reload, and member denial. Promote draft specifications only after all work orders pass.

## Files likely touched

- `apps/web/lib/types/system.ts`
- `apps/web/hooks/domains/system/use-storage-maintenance.ts`
- `apps/web/components/settings/system/storage/`
- `apps/web/src/locales/`
- `apps/web/e2e/helpers/storage-maintenance.ts`
- `apps/web/e2e/tests/system/storage-maintenance.spec.ts`
- `apps/web/e2e/tests/system/mobile-storage-maintenance.spec.ts`
- `docs/public/operations.md`
- `apps/backend/AGENTS.md`

## Dependencies

Task 02 must pass first. PR #4160 integration does not block this work order.

## Risks

Preserve path safety and the exact activity-admission scope. Busy deletion intentionally can fail active builds.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/system-page/requirements/go-cache-reclamation.md)
- [Design](../../specs/system-page/system-design/go-cache-reclamation.md)
- [Plan](plan.md), including compatibility matrix and existing regression suites.

## Results

Implemented the persisted busy-cleanup switch and visible warning, localized in
all seven supported languages. Go cleanup results now expose removed and
remaining/unknown bytes, partial outcomes, and a busy-policy snapshot in the Go
resource row and run history. The phone control retains a 44-pixel touch target.
Operations documentation describes direct deletion, build interruption, and
the scheduled-cleanup prerequisites. E2E cleanup restores policy settings while
preserving adopted-path state because adoption has a dedicated endpoint.

Validation passed:

- Focused storage UI and hook suite: 217 tests.
- Desktop storage-maintenance E2E: 10 passed.
- Mobile storage-maintenance E2E: 7 passed.
- Web typecheck, changed-file ESLint and Prettier, `i18n:check`, and `i18n:ratchet`.
- Public documentation validator tests: 62 passed; all 47 public pages validated.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check`.

## ASCII UI preview

See the [combined preview](plan.md#ascii-ui-preview).

UI-01: Settings > System > Storage > Go build cache.

```text
Managed Go cache                         [On]
Cache cleanup threshold (GB)             [15]
Allow cleanup while tasks are running    [Off]
Active builds may fail and need a retry.
Cache data is deleted without quarantine.
```

Desktop retains the existing label/switch row. Phone places the warning under
the label and keeps the switch reachable within the card. Both use the page
scroll owner, wrapped text, touch help, and existing responsive controls.
Phone targets are at least 44 pixels; desktop retains compact sizing.
The warning remains visible when the setting is off. This is a policy switch,
not a confirmation dialog. Labels and warning placement are structural; spacing is illustrative.

UI-02: Result in the existing Go row and run history.

```text
Removed: 6 GB
Remaining: 2 GB (partial measurement)
Go cleanup completed. Other cleanup skipped: tasks are running.
```

Show fields inline on desktop or stacked on phones. Use a partial-failure message
when deletion stops early. Never substitute zero for an unknown measurement.
Map both views to `AC-SYSTEM-PAGE-GO-CACHE-005.1` through `.5`.
