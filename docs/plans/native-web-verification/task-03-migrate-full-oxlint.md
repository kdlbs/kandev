---
id: "03-migrate-full-oxlint"
title: "Migrate the complete lint ruleset"
status: completed
wave: 3
depends_on: ["02-repair-compiler-compatibility"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-009
  - REQ-PLATFORM-CI-PERFORMANCE-011
acceptance_criteria:
  - AC-PLATFORM-CI-PERFORMANCE-009.1
  - AC-PLATFORM-CI-PERFORMANCE-009.2
  - AC-PLATFORM-CI-PERFORMANCE-009.3
  - AC-PLATFORM-CI-PERFORMANCE-009.4
  - AC-PLATFORM-CI-PERFORMANCE-009.5
  - AC-PLATFORM-CI-PERFORMANCE-011.1
system_design:
  - ../../specs/platform/system-design/web-native-verification.md
---

# Task 03: Migrate the complete lint ruleset

## Summary

Make the full native lint candidate pass with every existing guard effective.
Resolve compatibility findings through equivalent source repairs and complete plugin wiring.

## In scope

- Add exact Oxlint `1.91.0` development dependency resolution.
- Create the main and focused native configurations with shared i18n options and wait exemptions.
- Register required JS plugins and first-party rule exports without copying guard implementations.
- Record every original rule, severity, options, scope, exclusion, suppression, native mapping, and fixture in `rule-ledger.json`.
- Preserve intentionally disabled rules and their reasons.
- Add real native CLI wiring tests through TDD. Use isolated projects with the real configuration and original owner filenames.
- Exercise all four System owner paths, all four Query rules, i18n, task links, E2E waits, native rules, and file exemptions.
- Exercise JSX entities, attributes, inline suppressions, renamed files, missing plugins, parser errors, and selected paths containing spaces.
- Refresh the 37-finding inventory and resolve each finding without exclusions or weaker checks.
- Preserve rendered punctuation through equivalent Unicode representation.
- Require recorded callbacks before invocation. Missing callbacks must still fail the fixture.
- Remove the two shadowed helper declarations while preserving each effective last implementation.
- Add focused request-contract coverage for `ApiClient.createSentryInstance`. Preserve the retained `SessionPage.prStatusChip` body and existing page-object consumers.
- Keep existing RuleTester and ESLint adapter tests operational.

## Out of scope

- Required package-command cutover, a hybrid final CLI, skipped source files, and residual full-project ESLint passes.
- New blanket suppressions, weaker callback assertions, new translations, or different rendered copy.
- Runtime query, navigation, state ownership, or browser behavior changes.

## Acceptance

1. Full native lint selects the complete eligible source inventory and exits successfully with the original zero-warning policy.
2. Every mapped rule and scoped guard has executable evidence through the real host. Exempt and valid fixtures remain clean.
3. All compatibility repairs preserve text, assertions, request semantics, and effective selectors. Existing required lint still passes before cutover.

## Verification

The configurations, wiring test, and evidence command are planned new files.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run scripts/lib/native-lint-wiring.test.ts scripts/lib/migrated-system-query-owner-wiring.test.ts scripts/lib/tanstack-query-eslint-config.test.ts scripts/lib/eslint-i18n-config.test.ts scripts/lib/e2e-sleep-wiring.test.ts eslint-rules/no-migrated-system-query-owner.test.ts eslint-rules/no-task-link-bypass.test.ts eslint-rules/no-unsanctioned-sleep.test.ts e2e/helpers/api-client.test.ts)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings .)
python3 .github/scripts/web-toolchain-evidence.py compare-lint --web-root apps/web --config apps/web/oxlint.config.mjs
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
git diff --check
```

The native wiring suite must reject a disabled guard, missing owner path, lost exception, and swallowed plugin failure.
The comparison must not equate normalized rule names with proven behavior without the mapped fixtures.
The original full ESLint configuration remains available for the baseline and API consumers.

## Files likely touched

- `apps/web/package.json` and `apps/pnpm-lock.yaml`.
- `apps/web/oxlint.config.mjs`, `oxlint.i18n.config.mjs`, and `oxlint.e2e-sleeps.config.mjs` (new).
- Existing `apps/web/eslint-rules/*.mjs` exports and their tests.
- `apps/web/scripts/lib/native-lint-wiring.test.ts` (new) and existing guard wiring suites.
- `apps/web/e2e/helpers/api-client.ts`, its existing test, and `apps/web/e2e/pages/session-page.ts`.
- The 15 remaining lint-affected files listed in `baseline-2026-11-03.json`.
- `docs/plans/native-web-verification/rule-ledger.json` (new, durable migration mapping).

## Dependencies

Tasks 01 and 02.
Task 02 owns the preceding lockfile change and compiler compatibility repairs.
Read the design's Lint host, Oxlint migration, and Verification corpus sections.
Read the existing Query and architecture guard contracts before changing their execution host.

## Risks

- Converted override ignores can apply rules outside their intended scope.
- JS plugin AST differences can change entity or TypeScript assertion handling.
- The first duplicate Sentry method differs from the effective last method.
- A native-only fast result omits required JS guards and is not acceptance.

## Parallelism

`sequential`

## Results

Completed on November 3, 2026.

- Added `oxlint` 1.91.0 with the exact version in `apps/web/package.json` and
  the lockfile. The required configuration is `apps/web/oxlint.config.mjs`;
  focused previews keep the same imported options and exemptions in
  `oxlint.i18n.config.mjs` and `oxlint.e2e-sleeps.config.mjs`. Default Oxlint
  categories and plugins are off, so no unrelated recommended preset runs.
- `rule-ledger.json` records all 96 configured registrations with severity,
  options, scope, exclusions, inline-suppression policy, native target, and
  fixture, plus the disabled rules and the native-parser `no-octal` coverage.
- Resolved all 37 migration findings without a new exclusion: 23 JSX entities
  became the identical Unicode characters, 12 unsafe optional-chain expressions
  received explicit guards, and the two shadowed `createSentryInstance` and
  `prStatusChip` declarations were removed while retaining each effective later
  implementation. Added focused `createSentryInstance` request coverage.
- Added `scripts/lib/native-lint-wiring.test.ts`. All 21 cases pass through the
  real CLI in isolated projects with the original owner filenames. They cover
  every System owner path, all four Query rules, i18n text/attributes/entities,
  task links, E2E waits, exemptions, inline suppressions, renamed files, parser
  errors, a missing plugin, and selected paths containing spaces. Mutation cases
  prove a disabled guard or lost exemption cannot pass silently.
- The retained RuleTester, ESLint adapter, i18n, Query, System-owner, E2E-wait,
  and `ApiClient` suites pass. `compare-lint` reported equal 9,810-file
  inventories and zero findings. `pnpm run lint`, `pnpm run typecheck`,
  `pnpm run i18n:check`, and `pnpm run i18n:ratchet` pass.
