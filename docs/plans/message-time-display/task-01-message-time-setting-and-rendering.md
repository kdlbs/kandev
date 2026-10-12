---
id: "01-message-time-setting-and-rendering"
title: "Message time setting, mapping, and transcript rendering"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-MESSAGE-TIME-DISPLAY-001
acceptance_criteria:
  - AC-UI-MESSAGE-TIME-DISPLAY-001.2
  - AC-UI-MESSAGE-TIME-DISPLAY-001.3
  - AC-UI-MESSAGE-TIME-DISPLAY-001.4
  - AC-UI-MESSAGE-TIME-DISPLAY-001.5
  - AC-UI-MESSAGE-TIME-DISPLAY-001.6
  - AC-UI-MESSAGE-TIME-DISPLAY-001.8
system_design:
  - ../../specs/ui/system-design/message-time-display.md
---

# Task 01: Message time setting, mapping, and transcript rendering

## Summary

Persist `message_time_display` end to end and make the transcript footer honor it. After this task, saving the setting through the user-settings API changes every transcript message timestamp, including in other tabs. The tooltip and drawer show only the counterpart; the accessible name includes the visible label and counterpart. The Settings control is task 02.

## In scope

- Backend: model, constants, `NormalizeMessageTimeDisplay`, store default and both codec paths, `applyMessageTimeDisplay`, request field, `publishUserSettingsEvent` snapshot, DTO, controller, `mapUserSettingsState`, settings catalog entry and regenerated snapshots.
- Frontend contract: `MessageTimeDisplay` type, `UserSettingsState.messageTimeDisplay`, `parseMessageTimeDisplay`, default, and `buildAppearanceFields` mapping; typed fixtures such as `makeUnloadedSettings` in `hooks/use-ensure-user-settings.test.ts`.
- New pure `lib/i18n/message-time.ts` with `formatMessageTime` and `resolveMessageTimeLocale`; export `intlLocale` from `lib/i18n/formats.ts`.
- `MessageTimestamp` rendering and the new `task:messageTimestampAriaLabel` key replacing `task:showFullTimestamp` in all locales. Generate Traditional Chinese task catalog entries with `pnpm run i18n:zh-hant`; regenerate pseudo locale after the locale edits.
- Updating the two existing timestamp E2E specs (pin locale and timezone, pass a fixed `createdAt` to `seedSessionMessage`, assert exact label and absolute short tooltip) and the two `toLocaleString()` assertions in `message-actions.test.tsx`.

## Out of scope

- The settings control, discovery, tab mapping, and public docs (task 02). Live relative-label ticking.

## ASCII UI preview

`UI-01: Message footer` from [the plan](plan.md#ascii-ui-preview). Maps to AC-001.3 to .6.

```text
relative:       "5m ago"                        tooltip: 10/3/26, 2:32 PM
absolute_short: "10/3/26, 2:32 PM"              tooltip: 5 minutes ago
absolute_long:  "October 3, 2026 at 2:32:05 PM" tooltip: 5 minutes ago
```

## Acceptance

- A PATCH with each valid value persists and reads back, an invalid value is rejected, an omitted field leaves the saved value unchanged, and a missing or unknown stored value reads as `relative` in the DTO, boot state, and `user.settings.updated` payload.
- `formatMessageTime` returns the label and counterpart table in the design for each display and `null` for an unparseable time. Week-old and month-old messages never produce a date-shaped counterpart, absolute forms do not change as time passes, and an `en` interface on an `en-GB` browser yields day-first order.
- `MessageTimestamp` renders one `<time dateTime>` label, sets `title` to the counterpart, shows the counterpart in the touch drawer, exposes an accessible name containing both label and counterpart, and renders nothing for an invalid time, for every display. PATCH responses, WebSocket snapshots, and boot hydration reach the store; unknown becomes `relative`, an omitted field preserves the current value, and stale revisions are ignored.

## Verification

```bash
(cd apps/backend && go test -tags fts5 ./internal/user/... ./internal/backendapp/... ./internal/settingscatalog/... && go run ./cmd/settings-catalog --check)
make -C apps/backend lint
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm vitest run lib/i18n/message-time.test.ts components/task/chat/messages/message-actions.test.tsx lib/ssr/user-settings.test.ts lib/ws/handlers/users.test.ts hooks/use-ensure-user-settings.test.ts lib/settings-discovery/delivery-coverage.test.ts lib/settings-discovery/coverage-inventory.test.ts)
(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:pseudo && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps && pnpm --filter @kandev/web lint)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/message-timestamp-tooltip.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-message-timestamp-tooltip.spec.ts)
```

## Files likely touched

- `apps/web/e2e/helpers/api-client.ts` (`message_time_display` on the `saveUserSettings` type) and `apps/web/e2e/fixtures/test-base.ts` (reset `message_time_display: "relative"` in BOTH per-test reset lists: the `testPage` fixture and the global `test.beforeEach`).
- Backend: `apps/backend/internal/user/{models/models.go,store/sqlite.go,service/service.go,dto/dto.go,controller/controller.go}`, `internal/backendapp/boot_state_routes.go`, `internal/settingscatalog/defaults.go`, generated snapshots under `apps/web/lib/settings-discovery/`, and new focused test files because `sqlite_test.go` and `service_test.go` exceed the revive file-length limit (for example `store/sqlite_message_time_display_test.go`, `service/message_time_display_test.go`); boot evidence in `internal/backendapp/boot_state_user_settings_test.go`.
- Frontend: `apps/web/lib/types/http-user-settings.ts`, `lib/types/http.ts`, `lib/state/slices/settings/types.ts`, `lib/ssr/user-settings.ts` and test, `lib/ws/handlers/users.test.ts`, `lib/i18n/formats.ts`, `lib/i18n/message-time.ts` and test, `components/task/chat/messages/message-actions.tsx` and `message-actions.test.tsx`, `src/locales/*/task.json`.
- E2E: `apps/web/e2e/tests/chat/message-timestamp-tooltip.spec.ts`, `mobile-message-timestamp-tooltip.spec.ts`.

## Dependencies

None.

## Risks

- Changing only the model struct drops the field on write and read; the store round-trip test guards this.
- A required new `UserSettingsState` field breaks every typed fixture; typecheck finds them.
- Replacing the `showFullTimestamp` key must hit all eight locale directories or `i18n:check` fails.
- `navigator.languages` is browser-dependent; tests must stub it explicitly.

## Parallelism

`sequential`

## Inputs

- Design: Setting contract, Time forms, Transcript timestamp component. Precedents: `last_seen_display` in the same backend files, `parseLastSeenDisplay`, and the existing `MessageTimestamp` drawer path.

## Results

`make fmt`; backend tests passed for 9 packages and `go run ./cmd/settings-catalog --check` completed. Backend lint reported 0 issues. Frontend typecheck, focused Vitest (7 files, 145 tests), i18n generation/check/ratchet, and frontend lint passed. Desktop Chromium and mobile Chromium timestamp E2E each passed (1 test).

PR review follow-up additionally passed focused Vitest (4 files, 74 tests), frontend typecheck and lint, i18n ratchet/check, `go test -tags fts5 ./internal/user/... ./internal/backendapp/... ./internal/settingscatalog/...`, and `go run ./cmd/settings-catalog --check`. The desktop `message-time-display.spec.ts` and mobile `mobile-message-timestamp-tooltip.spec.ts` Playwright runs each passed (1 test); desktop rebuilt backend/Vite assets and mobile reused that build.
