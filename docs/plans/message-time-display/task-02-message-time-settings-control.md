---
id: "02-message-time-settings-control"
title: "Settings control, discovery, docs, and E2E"
status: completed
wave: 2
depends_on: ["01-message-time-setting-and-rendering"]
plan: "plan.md"
requirements:
  - REQ-UI-MESSAGE-TIME-DISPLAY-001
acceptance_criteria:
  - AC-UI-MESSAGE-TIME-DISPLAY-001.1
  - AC-UI-MESSAGE-TIME-DISPLAY-001.3
  - AC-UI-MESSAGE-TIME-DISPLAY-001.4
  - AC-UI-MESSAGE-TIME-DISPLAY-001.5
  - AC-UI-MESSAGE-TIME-DISPLAY-001.7
  - AC-UI-MESSAGE-TIME-DISPLAY-001.8
  - AC-UI-MESSAGE-TIME-DISPLAY-001.9
system_design:
  - ../../specs/ui/system-design/message-time-display.md
---

# Task 02: Settings control, discovery, docs, and E2E

## Summary

Add the **Message time** select to Settings, Preferences, Task behavior, Conversation tab with the shared draft, Save, and Reset flow. Register its discovery target and tab ownership, localize its copy in seven languages, document it, and prove the whole flow in a browser on desktop and a 390 px phone viewport.

## In scope

- `components/settings/message-time-display-settings.tsx` (`row` presentation) mounted after `TodoListPanelSettings`, as the last Conversation row, saving through `mapUserSettingsResponse`.
- Target and contributor-id entries in `components/settings/task-behavior-tabs.ts` and an updated `task-behavior-tabs.test.ts`.
- Discovery target and definition in `lib/settings-discovery/catalog/preferences.ts` (frontend only; generated snapshots are task 01's, and `go run ./cmd/settings-catalog --check` here is a no-drift check).
- English copy, `pt-pt`, `zh-cn`, `zh-hk`, `zh-tw`, `ja`, `ko`, and the generated pseudo locale. Generate Traditional Chinese settings entries with `pnpm run i18n:zh-hant`, then regenerate pseudo locale.
- Public docs for the Conversation tab preference (use `/docs-maintainer`): update `docs/public/configuration.md` for the setting and `docs/public/feature-status.md` to qualify the existing locale statement with the resolution rule from AC-001.4 (regional interface locales stay authoritative; regionless interface locales use the first browser language with a matching base language, then fall back to the interface locale).
- Two Playwright specs from the plan.

## Out of scope

- Backend behavior and transcript rendering logic (task 01). The unrouted card presentation in `general-settings.tsx`.

## ASCII UI preview

`UI-02: Message time row` from [the plan](plan.md#ascii-ui-preview), including the phone composition. Maps to AC-001.1, .7, .9.

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

## Acceptance

- Changing the select shows the dirty marker on the row and the Conversation tab and sends no request until Save; Save sends only `message_time_display` and applies the response through `mapUserSettingsResponse`; Reset restores the confirmed value with no request; a failed save keeps the draft.
- A settings search for the control opens the Conversation tab; the control passes `pnpm run i18n:check` and `i18n:ratchet`.
- Desktop E2E corroborates AC-001.3 and .4: choose each form, save, observe label and `title` in a seeded transcript, and reload to observe persistence. Mobile E2E corroborates AC-001.5 and .9 at 390 px. Seed a user message with `authorType: \"user\"`, `newTurn: true`, `turnStartedAt`, and `turnCompletedAt` several seconds after its fixed `createdAt` so `message-turn-duration` is visible; locate its `#msg-${messageId}` wrapper and duration footer using the existing mobile prompt-turn-duration pattern. For all three forms, assert full bounding-box containment (`label.x >= wrapper.x` and `label.right <= wrapper.right`), footer `scrollWidth <= clientWidth`, and no document-level horizontal overflow (secondary); tap the label and assert the counterpart in the drawer; assert the select trigger is at least 44 px tall. The document check alone is insufficient because the user-message wrapper clips overflow.

## Verification

```bash
(cd apps/backend && go run ./cmd/settings-catalog --check)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm vitest run components/settings lib/settings-discovery)
(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:pseudo && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps && pnpm --filter @kandev/web lint)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/message-time-display.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-message-time-display.spec.ts)
node scripts/validate-public-docs.mjs
node --test scripts/validate-public-docs.test.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- docs
```

## Files likely touched

- `apps/web/components/settings/message-time-display-settings.tsx` and its test
- `apps/web/components/settings/task-behavior-settings.tsx`, `task-behavior-tabs.ts`, `task-behavior-tabs.test.ts`
- `apps/web/lib/settings-discovery/catalog/preferences.ts`
- `apps/web/src/locales/*/settings.json`
- `docs/public/configuration.md` and `docs/public/feature-status.md` (the locale statement for dates and relative timestamps), plus any other page the docs-maintainer check names
- `apps/web/e2e/tests/chat/message-time-display.spec.ts`, `mobile-message-time-display.spec.ts`

## Dependencies

Task 01.

## Risks

- Without the `task-behavior-tabs.ts` entries, a search hit opens the wrong tab and a dirty draft shows no badge; the updated test pins the target map with `toEqual` and asserts `taskBehaviorTab("general-message-time-display")` because `CONTRIBUTOR_TABS` is private.
- The shared `Select` trigger already meets 44 px below 768 px and on coarse pointers; only a bounding-box assertion is needed.
- `mobile-*` spec names route to the `mobile-chrome` project, whose Pixel 5 device is 393 px wide; the spec sets 390 px with `setViewportSize`, since a `test.use` viewport is overridden by the fixture's device context.
- The settings-discovery catalog tests enforce unique ids and targets and valid parents; `aliasesKey` is optional copy.

## Parallelism

`sequential`

## Inputs

- Design: Settings control. Precedents: `AgentTabCloseBehaviorSettings` (`components/settings/agent-tab-close-behavior-settings.tsx`) for the enum Select, `mapUserSettingsResponse`, and `isUserSettingsResponseCurrent` gating; `security-settings.tsx` (`components/settings/account/`) for the confirmed-store reconcile.

## Results

`make fmt`; `go run ./cmd/settings-catalog --check`; focused settings and discovery Vitest passed (3 files, 20 tests); frontend typecheck, i18n check/ratchet, and frontend lint passed. Desktop Chromium and mobile-chrome E2E each passed (1 test). Public-doc validation passed (47 pages; 62 validator tests), documentation index validation passed (343 decisions, 1323 specs), spec lint passed, and `git diff --check -- docs` passed. The broader `pnpm vitest run components/settings lib/settings-discovery` was not clean: `storage-policy-card.test.tsx` and `workflow-sync-status-banner.test.tsx` failed, and Vitest timed out terminating the worker for `editors-settings-state.test.tsx`.
