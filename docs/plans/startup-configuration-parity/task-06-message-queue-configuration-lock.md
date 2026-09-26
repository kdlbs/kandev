---
id: "06-message-queue-configuration-lock"
title: "Message queue configuration lock"
status: done
wave: 4
depends_on: ["05-agentctl-settings-propagation"]
plan: "plan.md"
spec: "../../specs/platform/requirements/startup-configuration-parity.md"
---

# Task 06: Message queue configuration lock

Make the queue capacity resolver honor YAML above database overrides and report
that source accurately in Settings.

## Acceptance

- `messageQueue.maxPerSession` supplies the queue capacity when no environment
  value exists.
- Effective precedence is environment, YAML, database override, then default.
- The API source union adds `configuration` without changing existing
  `environment`, `setting`, and `default` meanings.
- A YAML value locks editing in the API and UI just like an environment value.
- The UI names configuration as the source. It does not show the environment
  variable lock message for a YAML value.
- Desktop and mobile use the same source semantics and accessible labels.
- The shared responsive Settings composition remains unchanged.
- All new user-facing text exists in English, Portuguese, Simplified Chinese,
  Hong Kong Traditional Chinese, and Taiwan Traditional Chinese.

## Files likely touched

- `apps/backend/internal/system/queuesettings/types.go`
- `apps/backend/internal/system/queuesettings/service.go`
- `apps/backend/internal/system/queuesettings/*_test.go`
- Backend settings handler or boot wiring that supplies queue configuration
- `apps/web/lib/types/system.ts`
- `apps/web/components/settings/system/message-queue-settings.tsx`
- `apps/web/components/settings/system/message-queue-settings.test.tsx`
- `apps/web/src/locales/en/system.json`
- `apps/web/src/locales/pt-pt/system.json`
- `apps/web/src/locales/zh-cn/system.json`
- `apps/web/src/locales/zh-hk/system.json`
- `apps/web/src/locales/zh-tw/system.json`
- `apps/web/e2e/tests/system/message-queue-settings.spec.ts`
- `apps/web/e2e/tests/system/mobile-message-queue-settings.spec.ts`

## Dependencies

Task 01 defines the typed queue field and provenance. Task 05 completes the
startup configuration propagation work before the product-facing source change.

## TDD sequence

1. Add backend service tests for YAML source, all precedence layers, and locked
   writes. Run them RED.
2. Add frontend unit tests for configuration source copy and disabled controls.
   Run them RED.
3. Implement the backend source union and frontend rendering. Add translations,
   then generate the Traditional Chinese pair with the repository command.
4. Extend the existing desktop and mobile queue E2E tests. Confirm the mobile
   controls remain reachable, readable, and at least 44 CSS pixels where the
   existing test requires an interactive target.
5. Run focused tests GREEN, then internationalization and E2E checks.

## Verification

```bash
cd apps/backend && go test ./internal/system/queuesettings -run '^Test.*(Config|Source|Precedence|Locked)' -count=1
cd apps && pnpm --filter @kandev/web test -- components/settings/system/message-queue-settings.test.tsx
cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:check
cd apps/web && pnpm e2e:run --project chromium tests/system/message-queue-settings.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/system/mobile-message-queue-settings.spec.ts
```

Run `pnpm install --frozen-lockfile` from `apps/` first when the worktree does
not have `apps/node_modules/`.

## Mobile parity

This task changes source copy and lock state in an existing shared settings
form. It does not introduce a new mobile composition. The existing mobile E2E
spec remains the contract for navigation, scrolling, control reachability, and
touch target size. Extend that spec with the configuration-source state.

## Risks

- Treating YAML as a database value would allow the UI to overwrite an operator
  policy. The backend source and write guard must agree.
- Reusing environment lock copy would misreport provenance even if the control
  is correctly disabled.
- Manual Traditional Chinese edits can drift. Use the repository conversion
  command and run the complete locale check.

## Output contract

Record RED and GREEN results, API source payloads, desktop and mobile evidence,
locale commands, files changed, and remaining risks in `## Results`.

## Results

RED:

- `go test ./internal/system/queuesettings -run '^Test.*(Configuration|EnvironmentLock)' -count=1` failed to compile with the missing configuration source, startup configuration type, and configuration lock error.
- The frontend unit suite failed the new configuration-source test by rendering `Default` instead of `Configuration` before the source union and rendering branch were implemented.

GREEN:

- `go test ./internal/system/queuesettings ./internal/backendapp -run '^Test.*(Configuration|Source|Precedence|Locked|Queue)' -count=1` passed 65 tests.
- `pnpm --filter @kandev/web test -- components/settings/system/message-queue-settings.test.tsx` passed 27 tests.
- `pnpm run i18n:zh-hant` generated the Traditional Chinese pair, and `pnpm run i18n:check` passed with all five locale catalogs complete and no em-dash or non-JSX-copy violations.
- `pnpm e2e:run --project chromium tests/system/message-queue-settings.spec.ts` passed 6 tests, and `pnpm e2e:run --project mobile-chrome tests/system/mobile-message-queue-settings.spec.ts` passed 2 tests for the configuration lock and mobile touch-target behavior.

The queue resolver now uses environment, YAML configuration, persisted setting,
then default precedence. The API adds `configuration` as a source and returns
`locked: true` for a YAML capacity. PATCH requests reject capacity changes with
a conflict while still allowing the two merge settings to change. Desktop and
mobile share the existing responsive form, source label, configuration-specific
lock copy, and accessible input/touch-target semantics.

Files changed:

- `apps/backend/internal/system/queuesettings/types.go`
- `apps/backend/internal/system/queuesettings/resolver.go`
- `apps/backend/internal/system/queuesettings/service.go`
- `apps/backend/internal/system/queuesettings/handler.go`
- `apps/backend/internal/system/queuesettings/queuesettings_test.go`
- `apps/backend/internal/system/system.go`
- `apps/backend/internal/backendapp/orchestrator.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/message_queue_settings_test.go`
- `apps/web/lib/types/system.ts`
- `apps/web/components/settings/system/message-queue-settings.tsx`
- `apps/web/components/settings/system/message-queue-settings.test.tsx`
- `apps/web/e2e/tests/system/message-queue-settings.spec.ts`
- `apps/web/e2e/tests/system/mobile-message-queue-settings.spec.ts`
- the five locale catalogs and pseudo locale
