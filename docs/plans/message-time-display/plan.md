---
created: 2026-10-03
status: complete
requirements:
  - REQ-UI-MESSAGE-TIME-DISPLAY-001
system_design:
  - ../../specs/ui/system-design/message-time-display.md
legacy_specs: []
---

# Implementation Plan: Message Time Display

## Overview

Add a per-user setting `message_time_display` (`relative` default, `absolute_short`, `absolute_long`) and apply it to the transcript message footer. The tooltip and touch drawer show only the counterpart form; the accessible name includes the visible label and its counterpart. Work is two vertical slices. Task 01 makes the setting real end to end and makes the transcript honor it, provable by saving the setting through the user-settings API. Task 02 adds the Settings control that makes the setting user-reachable, its discovery and tab ownership, public docs, and browser E2E. Task 01 comes first because the control needs the persisted setting and the transcript behavior to verify against.

## Scope

### In scope

- The portable `message_time_display` setting across model, store, service, DTO, controller, boot state, settings catalog, WebSocket snapshot, and the frontend contract and mapper.
- The pure `formatMessageTime` module and `MessageTimestamp` rendering for all three forms with tooltip, drawer, and a label-in-name accessible name.
- A Task behavior, Conversation tab control with draft, shared Save and Reset, discovery target, tab ownership, and seven-language copy.
- Public docs, unit and integration tests, and Playwright coverage on desktop and a 390 px phone viewport.

### Out of scope

- Live re-render of the relative label, other timestamp surfaces, per-session overrides, custom patterns, time zone selection, footer visibility rules.

## Technical approach

Task 01, backend: mirror `last_seen_display` through `models.NormalizeMessageTimeDisplay`, the `marshalUserSettingsPayload` and `scanUserSettings` codec paths, `applyMessageTimeDisplay`, the `publishUserSettingsEvent` snapshot map, `FromUserSettings`, the controller adapter, `mapUserSettingsState`, and the settings catalog snapshot regenerated with `go run ./cmd/settings-catalog`.

Task 01, frontend: `MessageTimeDisplay` type, `UserSettingsState.messageTimeDisplay`, `parseMessageTimeDisplay` and `buildAppearanceFields` in `lib/ssr/user-settings.ts`, new `lib/i18n/message-time.ts` (with `resolveMessageTimeLocale` and an exported `intlLocale`), `MessageTimestamp` in `components/task/chat/messages/message-actions.tsx`, and the new `task:messageTimestampAriaLabel` key replacing `task:showFullTimestamp`.

Task 02: new `components/settings/message-time-display-settings.tsx` (row presentation) mounted in the Conversation group of `task-behavior-settings.tsx`, entries in `components/settings/task-behavior-tabs.ts`, a discovery target and definition in `lib/settings-discovery/catalog/preferences.ts`, locale files, `docs/public`, and two Playwright specs.

Compatibility matrix:

| Surface | Transport | Behavior | Evidence |
| --- | --- | --- | --- |
| Boot payload | Go boot state, hydrator deep merge | `messageTimeDisplay` hydrated, unknown becomes `relative` | `boot_state_user_settings_test.go` |
| PATCH response | HTTP | applied through `mapUserSettingsResponse` | `user-settings.test.ts`, settings component test |
| `user.settings.updated` | WebSocket | applied through the existing revision gate | `users.test.ts` |
| Older server omitting the field | any | current mapped value preserved, starts `relative` | `user-settings.test.ts` |

## ASCII UI preview

`UI-01: Message footer`. Entry: any session transcript with a saved message. Structural requirement: one `<time>` label and one counterpart in the tooltip or drawer. Spacing and sample strings are illustrative.

```text
relative (default)             absolute_short                    absolute_long
| [copy] [raw]  5m ago  |     | [copy] [raw] 10/3/26, 2:32 PM |  | [copy] [raw] October 3, 2026 at 2:32:05 PM |
 tooltip: 10/3/26, 2:32 PM     tooltip: 5 minutes ago             tooltip: 5 minutes ago
```

The footer stays one non-wrapping row in every form. On narrow widths the long label may wrap inside its own item and must not be clipped by the message wrapper.

`UI-02: Message time row`. Entry: Settings, Preferences, Task behavior, Conversation tab, after Todo list panel as the last row. Structural requirement: one labeled select with three options, the dirty marker, and the shared floating Save changes action. The description is one string for all pointers.

```text
Task behavior   [ Tasks ] [ Conversation* ] [ Runtime ]
  Agent tab close behavior ...                       [ ... ]
  Unread messages ...                                 [ ... ]
  Show anchored prompt bar ...                       [ ... ]
  Todo list panel ...                                 [ ... ]
  Message time                                  [ Relative         v ]
  How transcript message times are written.
  The other form appears in the tooltip, or when you tap the time on touch devices.
                                                Relative
                                                Absolute (short)
                                                Absolute (long)
```

Phone: the row stacks, with label and description above a full-width select. The select trigger is at least 44 px tall (the shared `Select` trigger already is below 768 px and on coarse pointers; option rows keep the shared default). Tapping a message time label opens the bottom drawer holding the counterpart form.

```text
+----------------------------+
| Message time               |
| How transcript message     |
| times are written. The     |
| other form appears in the  |
| tooltip, or when you tap   |
| the time on touch devices. |
| [ Relative              v ]|
+----------------------------+
```

## Tests

| Criterion | Evidence |
| --- | --- |
| AC-UI-MESSAGE-TIME-DISPLAY-001.2, .8 | `apps/backend/internal/user/service`: `applyMessageTimeDisplay` and the published event map; `store`: round trip for default, stored value, and unknown coercion; `dto_test.go`; `controller_test.go` mapping; `boot_state_user_settings_test.go` |
| .8 | `apps/web/lib/ssr/user-settings.test.ts`; `apps/web/lib/ws/handlers/users.test.ts` (valid, unknown, omitted, stale) |
| .2, .3, .4, .6 | `apps/web/lib/i18n/message-time.test.ts` (vitest runs in a `threads` pool, so `process.env.TZ` assigned in a test does not re-init ICU; tests therefore use instants at 12:00:00Z, build expected strings in the test from `Intl.DateTimeFormat` with the same options, locale, and the runtime zone, and assert day/month order by shape; `navigator.languages` is stubbed and the clock uses fake timers): label and counterpart per display; at exactly `7 * 86_400_000` ms a relative-mode message labels as a numeric date in the resolved locale (not the runtime default locale) with an absolute short tooltip, and one millisecond less keeps the ladder label; week-old and month-old messages never produce a date-shaped counterpart; `en` interface on an `en-GB` browser yields day-first order; a `zh-HK` interface with a `zh-CN` browser keeps `zh-HK` order; no matching language falls back to the interface locale; invalid input returns `null`; repeated calls reuse cached formatters |
| .2, .3, .5, .6 | `apps/web/components/task/chat/messages/message-actions.test.tsx` (existing suite, extended, and its two `toLocaleString()` assertions updated): title, drawer content, the touch drawer trigger's accessible name contains label and counterpart, nothing rendered for invalid time, per display |
| .1 | `apps/web/components/settings/task-behavior-tabs.test.ts` (target map `toEqual`; `taskBehaviorTab("general-message-time-display")` is `conversation`); settings discovery tests |
| .1, .7, .8 | `apps/web/components/settings/message-time-display-settings.test.tsx`: three options, dirty draft with no request, save sends only `message_time_display` and applies the response through the mapper, discard restores, failed save keeps the draft, and a stale PATCH response preserves the confirmed store value and draft while making the shared save coordinator return `canLeave: false` |

## E2E tests

- `apps/web/e2e/tests/chat/message-time-display.spec.ts` (`chromium` project): AC-001.1, .3, .4, .7, .8. Pin locale/timezone, seed one fixed old message for exact absolute strings and the seven-day date label, and a recent message during test setup for the compact relative label. Save each form, assert label/title, reload, and assert persistence.
- `apps/web/e2e/tests/chat/mobile-message-time-display.spec.ts` (`mobile-chrome` project; `test.use` pins locale and timezone; the 390 px width is set with `await testPage.setViewportSize({ width: 390, height: 844 })` before navigation, because the fixture's `devices["Pixel 5"]` context overrides a `test.use` viewport): AC-001.5, .9. Seed a user message with `authorType: "user"`, `newTurn: true`, `turnStartedAt`, and `turnCompletedAt` several seconds after the fixed `createdAt`, so `message-turn-duration` is visible; locate its `#msg-${messageId}` wrapper and duration footer using the existing mobile prompt-turn-duration pattern. For each of the three forms (set through `apiClient.saveUserSettings`): assert full bounding-box containment (`label.x >= wrapper.x` and `label.right <= wrapper.right`), the footer's `scrollWidth <= clientWidth`, and no document-level horizontal overflow (secondary); tap the label and assert the exact drawer counterpart; assert the select trigger is at least 44 px tall.
- The desktop tooltip spec uses a fixed old `createdAt` to assert the numeric-date relative label and exact short tooltip. The mobile companion uses a recent `createdAt` so its tap-to-open test covers the compact relative label and absolute-short drawer counterpart.

## Work orders

- [x] [Task 01: Message time setting, mapping, and transcript rendering](task-01-message-time-setting-and-rendering.md)
- [x] [Task 02: Settings control, discovery, docs, and E2E](task-02-message-time-settings-control.md)

Dependency order: 01, then 02. Both sequential.

## Verification results

Task 01 completed; results are recorded in its work order. Task 02 passed formatting, catalog drift check, focused settings/discovery tests (3 files, 20 tests), frontend typecheck, i18n check/ratchet, frontend lint, desktop Chromium E2E (1 test), mobile-chrome E2E (1 test), public-doc validation (47 pages; 62 validator tests), docs-index validation (343 decisions, 1323 specs), spec lint, and `git diff --check -- docs`. The broader settings/discovery Vitest run had two failures (`storage-policy-card.test.tsx`, `workflow-sync-status-banner.test.tsx`) and a worker termination timeout (`editors-settings-state.test.tsx`); these are recorded without being treated as passing.

PR review follow-up passed focused Vitest (4 files, 74 tests), frontend typecheck and lint, i18n ratchet/check, backend user/backendapp/settingscatalog tests with the `fts5` tag, and the settings-catalog consistency check. The managed desktop E2E runner passed `message-time-display.spec.ts` (1 test) and rebuilt backend/Vite assets; mobile Chrome passed `mobile-message-timestamp-tooltip.spec.ts` (1 test) against that build.

## Risks

- The hand-built store codec paths and the WebSocket snapshot map drop a field silently when only the model struct changes. Task 01 lists each path and tests the stored-JSON round trip and the published event.
- Tooltip interpretation is settled: retain all three requested label choices; absolute long is available only as a label, never as a counterpart. The relative label's tooltip is absolute short, and absolute labels' tooltips are relative.
- The default tooltip changes from the browser's `toLocaleString()` (with seconds) to the locale short form (no seconds). Both existing timestamp E2E specs and two assertions in `message-actions.test.tsx` change with it.
- Locale: the interface locale carries no region for `en`, `ja`, and `ko`, so absolute forms take the regional order from `navigator.languages` for those languages only; regional interface locales (`zh-HK`, `pt-PT`, and similar) keep their own order. A wrong match would reorder dates for non-US users.
- Locale interpretation is settled: explicit regional interface locales remain authoritative; for regionless interface locales (`en`, `ja`, `ko`), use the first browser language with the same base language when available, otherwise the interface locale. Thus an English interface on a German browser with `["de-DE", "en-US"]` uses `en-US` (`10/3/26, 2:32 PM`), not the browser's first language (`03.10.26, 14:32`); this avoids mixed-language month names in the long form.
- Generated settings snapshots and the seven-locale catalogs gate CI; both must be regenerated, not hand-edited.
- Counterpart wording differs in style from the label ("5 minutes ago" versus "5m ago"); accepted because it only appears in the tooltip and drawer.
- E2E settings isolation: `apiClient.saveUserSettings` (`e2e/helpers/api-client.ts`) is a closed type, and `e2e/fixtures/test-base.ts` has two per-test reset lists (the `testPage` fixture and the global `test.beforeEach`). The type and both lists gain `message_time_display` (reset to `relative`), or specs leak a non-default form into later specs in the same worker, including the two rewritten timestamp specs.
- `message-actions.tsx` has about 36 effective lines of headroom under the 600-line lint limit, so formatting logic stays in `lib/i18n/message-time.ts`.
