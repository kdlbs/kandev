---
id: "04-settings-ui-card"
title: "Settings UI card"
status: done
wave: 1
parallelism: sequential
depends_on: ["03-frontend-settings-plumbing"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/prevent-agent-autostart-on-open.md"
---

# Task 04: Settings UI card

## Acceptance

- Settings → Task Actions shows a new switch card "Prevent auto-start on open"
  with helper text describing the two gated cases (post-restart open, final
  workflow step). All copy goes through `t()`; no em dash (U+2014).
- The switch follows the `ArchiveConfirmationSettings` pattern: draft state,
  `data-settings-dirty`, `useSettingsSaveContributor`, save via
  `updateUserSettings({ prevent_auto_start_agent_on_open })`, store updated
  through `setUserSettings`. The contributor id MUST be unique (e.g.
  `general-prevent-auto-start-on-open`): the save registry is a `Map` keyed by
  contributor id (`settings-save-provider.tsx:224-248`), and the archive card
  already owns `general-task-actions` (`archive-confirmation-settings.tsx:33`),
  so a literal clone would silently replace the archive contributor.
- The card registers a settings-discovery target and definition.
- i18n check and ratchet pass.

## Verification

```bash
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
```

```bash
(cd apps/web && pnpm vitest run components/settings/prevent-auto-start-agent-settings.test.tsx components/settings/archive-confirmation-settings.test.tsx)
```

## Files Likely Touched

- `apps/web/components/settings/prevent-auto-start-agent-settings.tsx` (new; clone `archive-confirmation-settings.tsx`)
- `apps/web/components/settings/prevent-auto-start-agent-settings.test.tsx` (new; mirror `archive-confirmation-settings.test.tsx`)
- `apps/web/components/settings/general-settings.tsx` (`TaskActionsSettings`)
- `apps/web/lib/settings-discovery/catalog/preferences.ts` (target + control definition; mirror `archiveConfirmation` at `:35`/`:311`; `GENERAL_SETTINGS_TARGETS` lives here after the PageShell restructure #2322)
- `apps/web/src/locales/en/settings.json`, `apps/web/src/locales/pseudo/settings.json`, `apps/web/src/locales/pt-pt/settings.json`, `apps/web/src/locales/zh-cn/settings.json`

## Dependencies

Task 03 (store field + `updateUserSettings` payload).

## Inputs

- Spec "What" bullet on the settings surface.
- `components/settings/archive-confirmation-settings.tsx` as the pattern
  reference; `GENERAL_SETTINGS_TARGETS` structure.

## Output Contract

The setting is user-visible and persisted from the Task Actions settings page.
Component test pins toggle → save payload; i18n ratchets stay green.

Proposed copy (translated via the four locale files):
- Title: "Prevent auto-start on open"
- Help: "Don't start the agent automatically when opening a task after a restart or from the final workflow step. The Start agent button is shown instead."
