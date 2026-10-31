---
id: "01-localize-release-notifications"
title: "Localize release notifications"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-NOTIFICATIONS-001
acceptance_criteria:
  - AC-PLATFORM-NOTIFICATIONS-001.16
system_design:
  - ../../specs/platform/system-design/localized-update-notifications.md
---

# Task 01: Localize Release Notifications

## Summary

Format a versioned Local Kandev update notification using the active locale,
including native delivery, even when the backend supplies English fallback text.

## In scope

- Client notification copy and focused hook and browser regressions.
- Existing application locale keys and semantic release-version payload.

## Out of scope

- Remote provider formatting, agent runtime update copy, and UI geometry.

## Acceptance

- Browser and native Local notifications use the selected locale and retain
  the supplied version; no-version legacy events keep their fallback copy.
- Pseudo-locale browser and unit regression tests pass.

## Verification

```bash
(cd apps && pnpm --filter @kandev/web exec vitest run hooks/use-update-available-toast.test.ts components/update-available-toast-bridge.test.tsx)
(cd apps && ../scripts/run-quiet e2e --summary -- pnpm --dir web e2e:run --host --shards 1 --project chromium tests/i18n/pseudo-coverage.spec.ts -- --grep 'localizes a Kandev update notification received from the backend' --retries=0)
(cd apps && ../scripts/run-quiet e2e --summary -- pnpm --dir web e2e:run --host --no-build --shards 1 --project chromium tests/i18n/pseudo-coverage.spec.ts -- --retries=0)
(cd apps/web && pnpm exec eslint --max-warnings 0 hooks/use-update-available-toast.ts hooks/use-update-available-toast.test.ts e2e/tests/i18n/pseudo-coverage.spec.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=7168 pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Files likely touched

- `apps/web/hooks/use-update-available-toast.ts`
- `apps/web/hooks/use-update-available-toast.test.ts`
- `apps/web/e2e/tests/i18n/pseudo-coverage.spec.ts`

## Dependencies

None.

## Risks

Events with a version use standard client copy; custom backend text remains
available only on legacy events without a version.

## Parallelism

Sequential.

## Inputs

Platform semantic notification requirement and localized update design.

## Results

The hook and bridge suites passed 18 tests. The new hook and browser tests
failed against the old implementation and pass after the correction; the full
pseudo-locale browser spec passed 23 tests with retries disabled. Typecheck,
changed-file lint, i18n checks, and specification checks passed. The original
CI shard contains other specs and uses four concurrent processes; it was not
repeated locally under the repository's resource guard.

After merging the latest base, the selected frontend suites passed 54 tests and
the rebuilt Chromium run of this spec and the deleted-task spec passed all 25
tests with retries disabled.

## SDK integration reconciliation

PR #4534 retains the landed version guard and legacy no-version fallback.
Its browser/native hook matrix switches locale after mounting and before
delivering populated backend copy. This subsumes the earlier native-only
locale case; the legacy unit control and controlled WebSocket pseudo-locale
scenario remain intact. No SDK, payload identity or transport policy changes
are introduced. The public operations guide clarifies Local release language.

A controlled two-release WebSocket case reproduces an ambiguous title-only
selector. Selecting the intended version fixes that fixture while retaining
both localized title and body assertions. It fails before the selector change
and passes afterward, with retries disabled. The production hook matches main
exactly; the backend producer includes the version for standard release notices.

Focused rebase verification passed 126 frontend tests across ten files, the
selected backend session/Git race checks, all 29 rebuilt Chromium cases
(pseudo-locale, deleted-task recovery and executor keyboard access), and both
phone cases (executor navigation and release-toast/composer containment).
Browser retries were disabled. Typecheck, scoped zero-warning lint, i18n,
docs/spec lint and direct documentation coverage passed. SDK and release
files are byte-identical to the prior published green head.
