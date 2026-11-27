---
status: draft
system: ui
requirements:
  - REQ-UI-MESSAGE-TIME-DISPLAY-001
created: 2026-10-03
owners:
  - kandev
---

# Message Time Display System Design

## Purpose and boundaries

The UI system owns the message time preference, its formatting rules, and the transcript footer rendering. The preference is a portable per-user setting, so it follows [ADR 0041](../../../decisions/0041-backend-owned-portable-user-settings.md): the backend owns the default and normalization, and the frontend uses the single wire-to-store mapper. The design copies the enum-setting path of `last_seen_display` from [relative last seen](../requirements/relative-last-seen.md) and the settings-row pattern of `AnchoredPromptBarSettings`. It does not change the message model, message ordering, or footer visibility rules.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-MESSAGE-TIME-DISPLAY-001` | [Setting contract](#setting-contract), [Time forms](#time-forms), [Transcript timestamp component](#transcript-timestamp-component), [Settings control](#settings-control), [Failure and recovery](#failure-and-recovery) |

AC coverage: 001.1 and 001.7 in the settings control; 001.2 and 001.8 in the setting contract; 001.3 to 001.6 in the time forms and timestamp component; 001.9 in the timestamp component and settings control.

## Setting contract

The setting is a string enum `message_time_display` with values `relative` (default), `absolute_short`, and `absolute_long`. It rides the existing `users.settings` JSON blob, so no SQL migration is required.

Backend, mirroring `last_seen_display`:

- `internal/user/models/models.go`: `MessageTimeDisplay` on `UserSettings`, three value constants, and `NormalizeMessageTimeDisplay`, which maps any value outside the two absolute values to `relative`.
- `internal/user/store/sqlite.go`: default `relative` in `defaultUserSettings`, an entry in `marshalUserSettingsPayload`, and a tolerant read in `scanUserSettings` (a missing, null, non-string, or unknown stored value reads as `relative`), following `normalizeLastSeenDisplayStored`. The model struct alone does not persist the field.
- `internal/user/service/service.go`: `MessageTimeDisplay *string` on `UpdateUserSettingsRequest`, an `applyMessageTimeDisplay` validator called from the settings apply path that rejects values outside the three, and the normalized field in the `publishUserSettingsEvent` snapshot so `user.settings.updated` carries it. The write goes through the existing `updateUserSettingsCAS`, so a concurrent omitted-field PATCH cannot revert it.
- `internal/user/dto/dto.go`: field on `UserSettingsDTO` (normalized in `FromUserSettings`) and `*string` on the PATCH request. `controller/controller.go` forwards the field; the controller is a manual field-by-field adapter.
- `internal/backendapp/boot_state_routes.go`: `messageTimeDisplay` in `mapUserSettingsState` through a normalize helper. The boot payload is a camelCase map that the frontend hydrator deep-merges into the store (`lib/state/hydration/hydrator.ts`), so boot normalization comes only from this Go mapper.
- `internal/settingscatalog/defaults.go`: add the field to the alias listing, then regenerate the generated settings snapshots with `go run ./cmd/settings-catalog` from `apps/backend`. The generated coverage inventory picks up the new DTO field on its own.

Frontend:

- `lib/types/http-user-settings.ts`: `MessageTimeDisplay` union, optional `message_time_display` on the response and PATCH payload types, exported through `lib/types/http.ts`.
- `lib/state/slices/settings/types.ts`: `messageTimeDisplay: MessageTimeDisplay` on `UserSettingsState`.
- `lib/ssr/user-settings.ts`: `parseMessageTimeDisplay` (unknown becomes `relative`), the `relative` default in `createDefaultUserSettings`, and `mapDefined` mapping in `buildAppearanceFields`. This one mapper serves PATCH responses (`mapUserSettingsResponse`) and `user.settings.updated` snapshots (`mapUserSettingsData`); the WebSocket handler in `lib/ws/handlers/users.ts` needs no change beyond it. Boot hydration does not pass through this mapper (see the Go mapper above).

## Time forms

A pure module `lib/i18n/message-time.ts` exposes `formatMessageTime(createdAt, display)` returning `{ label, counterpart }`, or `null` when `parseStrictRfc3339Timestamp` (`lib/utils/strict-timestamp.ts`) rejects the input. It is the only place that decides the label and tooltip pairing. Tests control the clock with fake timers.

| Display | Label | Counterpart (tooltip, drawer, accessible name) |
| --- | --- | --- |
| `relative` | relative label | absolute short |
| `absolute_short` | absolute short | relative counterpart |
| `absolute_long` | absolute long | relative counterpart |

Two relative helpers exist and the module names them by path to avoid the same-name collision:

- The relative label uses `formatRelativeTime` from `@/lib/utils` as the footer does today ("5m ago") while the elapsed time is under `7 * 86_400_000` ms. At or above that, the helper falls back to `date.toLocaleDateString()` in the runtime default locale, which can disagree with the tooltip locale. `formatMessageTime` therefore renders that label itself with `{ year: "numeric", month: "numeric", day: "numeric" }` in the resolved locale below, which keeps the shape of today's label (for example `7/20/2026`) while sharing the tooltip's locale convention.
- The relative counterpart uses `formatRelativeTime` from `@/lib/i18n/formats`, which is `Intl.RelativeTimeFormat` based ("3 weeks ago") and never falls back to a date, so an absolute label is never paired with a date-shaped counterpart. Its phrasing is longer than the label style; this difference is accepted because the counterpart appears only in the tooltip and drawer.

Absolute forms and the 7-day date label use `Intl.DateTimeFormat` with a resolved locale. `resolveMessageTimeLocale()` returns the interface locale when it names a region subtag (`zh-HK`, `zh-CN`, `zh-TW`, `pt-PT`), so an explicit regional interface choice is never overridden. When the interface locale has no region (`en`, `ja`, `ko`), it returns the first `navigator.languages` entry whose base language equals the interface base language (an `en` interface on an `en-GB` browser yields `en-GB`), otherwise the interface locale from `intlLocale()` in `lib/i18n/formats.ts` (exported for this use; `pseudo` maps to `en`). The regional order is otherwise lost because the interface locale carries no region for those languages. Absolute short is `{ dateStyle: "short", timeStyle: "short" }` and absolute long is `{ dateStyle: "long", timeStyle: "medium" }`. The validity gate runs before any formatter, because `Intl.DateTimeFormat` throws `RangeError` on an invalid date.

`Intl.DateTimeFormat` instances are memoized in `message-time.ts` by resolved locale and style, as `formats.ts` does for `RelativeTimeFormat`, because a streaming transcript re-renders each footer often.

The relative-mode tooltip changes from `date.toLocaleString()` to the absolute short form, so the tooltip loses seconds. This is an intentional consequence of AC-UI-MESSAGE-TIME-DISPLAY-001.2 and .3.

## Transcript timestamp component

`MessageTimestamp` in `components/task/chat/messages/message-actions.tsx` reads `state.userSettings.messageTimeDisplay` through `useAppStore`, calls `formatMessageTime`, and returns `null` when the result is `null`. The `<time>` element keeps `dateTime={createdAt}`, renders `label`, and sets `title` to `counterpart`. The coarse-pointer path keeps `useTouchDrawer`, the `Drawer` trigger with `data-testid="message-timestamp-trigger"`, and shows `counterpart` in the drawer body.

The trigger's `aria-label` changes. The current `task:showFullTimestamp` value ("Show full timestamp: {{absoluteTime}}") would announce a false statement with a relative counterpart and omits the visible text. A new key `task:messageTimestampAriaLabel` takes `label` and `counterpart` and renders both ("Message time: {{label}}, {{counterpart}}"), satisfying label-in-name. `task:showFullTimestamp` is removed from every locale if no other caller uses it. The component adds no ticking hook, so the relative label's refresh behavior is unchanged.

Footer width: `absolute_long` is the longest label. The footer row (`flex items-center gap-2`) does not wrap, and a user message footer sits inside a `max-w-[85%] overflow-hidden` wrapper that clips overflow without changing document scroll width. The time element and trigger get `min-w-0` and the label text wraps inside its own flex item. The mobile E2E therefore asserts full bounding-box containment for all three forms (`label.x >= wrapper.x` and `label.right <= wrapper.right`), plus footer `scrollWidth <= clientWidth`, with the document-level check only secondary. Formatting logic stays in `lib/i18n/message-time.ts`, because `message-actions.tsx` has little headroom under the file-length lint limit.

## Settings control

A new `components/settings/message-time-display-settings.tsx` renders a labeled `Select` with three options in the `row` presentation only; the card presentation in `general-settings.tsx` is an unrouted component and is not extended. It is mounted in `components/settings/task-behavior-settings.tsx` in the Conversation tab group, after `TodoListPanelSettings` as the last row.

Tab ownership is part of the contract. `components/settings/task-behavior-tabs.ts` maps discovery targets (`TASK_BEHAVIOR_TARGET_TABS`) and contributor ids (`CONTRIBUTOR_TABS`) to tabs. The new target `GENERAL_SETTINGS_TARGETS.messageTimeDisplay` and the contributor id `general-message-time-display` are both mapped to `conversation`, so a search hit opens the Conversation tab and a dirty or failed draft shows the unsaved or attention badge on it. `task-behavior-tabs.test.ts` is updated: the `toEqual` pins the target map, and `taskBehaviorTab("general-message-time-display")` is asserted to be `conversation` because `CONTRIBUTOR_TABS` is private.

State follows `AgentTabCloseBehaviorSettings` (`components/settings/agent-tab-close-behavior-settings.tsx`), which is the same-tab enum precedent: `saved` and `draft` local state, `useSettingsSaveContributor`, and a `discard` that restores `saved`. Save follows ADR 0041: `updateUserSettings({ message_time_display })` returns the `UserSettingsResponse`, applied with `setUserSettings(mapUserSettingsResponse(response, state.userSettings))`. `isUserSettingsResponseCurrent` (`lib/settings/user-settings-revision.ts`) gates the result: `saved` is set only from a current response, or from the confirmed store value, so a newer cross-tab revision that makes `setUserSettings` ignore the response never leaves the control showing the submitted value with no dirty marker. The submitted value is never spread into the store. A store change reconciles `saved` and replaces `draft` only when the draft was clean, so a newer cross-tab value never overwrites an unsaved choice.

The select trigger already meets 44 px below 768 px and on coarse pointers through the shared control sizing (`control-sizing.tsx`), so it needs no extra class; option rows keep the shared default height. A `kind: "control"` discovery definition under the Task behavior parent in `lib/settings-discovery/catalog/preferences.ts` makes the control searchable. New copy lives in `src/locales/en/settings.json` and `task.json`, with translations in `pt-pt`, `zh-cn`, `zh-hk`, `zh-tw`, `ja`, and `ko`, plus the generated pseudo locale (`pnpm run i18n:pseudo`; Traditional Chinese through `pnpm run i18n:zh-hant`). Copy uses no em dash. Public documentation is updated with the control in `docs/public/configuration.md` and with the exact locale resolution in `docs/public/feature-status.md`: regional interface locales remain authoritative; a regionless interface locale uses the first browser language with a matching base language, then falls back to the interface locale. This qualifies the existing general statement that dates follow the selected locale.

## Failure and recovery

- Settings write fails: the draft stays dirty and the shared save flow reports the error; the transcript keeps rendering the confirmed store value.
- Unknown stored or wire value: normalized to `relative` on read at the backend and in `parseMessageTimeDisplay`; an unknown submitted value is rejected by `applyMessageTimeDisplay`.
- Unparseable `created_at`: `formatMessageTime` returns `null` and the footer renders nothing for that message.
- Older server without the field: an absent `message_time_display` preserves the current mapped value, which starts at `relative`.
- No `navigator.languages` match, or the property is missing: the interface locale is used.

## Persistence

The value is stored in the existing per-user settings JSON. There is no schema migration, no browser storage, and no per-session state.

## Observability

No new metrics or logs. The existing `user.settings.updated` event and settings-write errors cover diagnostics.

## Related decisions

- [ADR 0041: Backend-owned portable user settings](../../../decisions/0041-backend-owned-portable-user-settings.md)
