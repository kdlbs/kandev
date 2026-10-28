---
id: "01-suggestion-agent-and-preference"
title: "Add the prompt suggestion preference"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-PROMPT-SUGGEST-001
acceptance_criteria:
  - AC-UI-PROMPT-SUGGEST-001.1
  - AC-UI-PROMPT-SUGGEST-001.3
  - AC-UI-PROMPT-SUGGEST-001.4
system_design:
  - ../../specs/ui/system-design/composer-prompt-suggestions.md
---

# Task 01: Add the Prompt Suggestion Preference

## Summary

Add the per-user `prompt_suggestions` preference end to end, default off, and
the "Suggest next prompt" switch in Settings > Task behavior > Conversation.

This work order first also delivered a utility-agent fallback: a second switch,
a profile selector, and the `suggest-next-prompt` built-in. The maintainer
review asked for native suggestions only, so
[Task 04](task-04-native-only-scope.md) removed them. The file name keeps its
original ID so the dependency graph stays stable.

## In scope

- `prompt_suggestions` through:
  - user models, DTO, service, and controller;
  - the JSON settings store;
  - settings catalog defaults;
  - the boot payload;
  - the frontend `promptSuggestions` settings field and SSR mapping.
- `PromptSuggestionSettings`: one Switch row with a save contributor, a
  discovery target, and an info popover. The popover says that only agents with
  native support, such as Claude Code, produce suggestions, starting from the
  session's next launch or resume.
- Copy in every locale catalog. Use `pnpm run i18n:zh-hant` for the Traditional
  Chinese pair. No em dashes.
- `docs/public/developer-tools.md`: the first version of a "Prompt suggestions"
  section covering the switch and the next-session-start note.

## Out of scope

- Agent launch, agentctl, orchestrator, and WebSocket changes (Task 02).
- Composer, hook, and editor changes (Task 03).

## Acceptance

- `prompt_suggestions` defaults to `false`, round-trips through user settings
  `PATCH`, and appears in the boot payload. An absent stored value reads as
  `false`.
- Settings > Task behavior shows the switch (UI-04), and it persists across a
  reload.

## ASCII UI preview

See [plan UI-04](plan.md#ui-04-settings--task-behavior--conversation).

```text
| Suggest next prompt                                   (i)  [ off ] |
| Show a suggested reply after each agent turn.                      |
```

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/backend && go test ./internal/user/... ./internal/settingscatalog/... ./internal/backendapp/...
cd apps/web && pnpm test -- lib/ssr/user-settings.test.ts
cd apps/web && pnpm e2e:run tests/settings/prompt-suggestion-settings.spec.ts
cd apps/web && pnpm run typecheck && pnpm run i18n:check
cd apps && pnpm --filter @kandev/web lint
make -C apps/backend lint
```

## Files likely touched

- `apps/backend/internal/user/{models,dto,service,controller,store}/…`
- `apps/backend/internal/settingscatalog/defaults.go`
- `apps/backend/internal/backendapp/boot_state_routes.go`
- `apps/web/lib/state/slices/settings/types.ts`
- `apps/web/lib/ssr/user-settings.ts` and its test
- `apps/web/lib/settings-discovery/catalog/preferences.ts`
- `apps/web/components/settings/prompt-suggestion-settings.tsx`
- `apps/web/components/settings/task-behavior-settings.tsx` and
  `task-behavior-tabs.ts` (Conversation tab)
- `apps/web/e2e/tests/settings/prompt-suggestion-settings.spec.ts`
- `apps/web/src/locales/*/settings.json`
- `docs/public/developer-tools.md`

## Dependencies

None.

## Risks

- Keep the long search-string format in `settingscatalog/defaults.go`.
- A non-pointer zero value or a default mismatch must not turn the preference on
  for existing users.

## Parallelism

`sequential`

## Inputs

- `docs/specs/ui/requirements/composer-prompt-suggestions.md`
- `docs/specs/ui/system-design/composer-prompt-suggestions.md`
- `apps/backend/AGENTS.md`, `apps/web/AGENTS.md`, `docs/i18n.md`
- `agent_generated_task_titles` as the preference pattern

## Results

Implemented the preference end to end, the settings row, and the public docs.

The row lives in **Settings > Task behavior > Conversation**, not General. The
`TaskActionsSettings` card in `general-settings.tsx` is no longer mounted
anywhere, and the live page is `task-behavior-settings.tsx`.

Red-Green evidence:

- **Go.** The new tests in `dto_test`, `service_test`, `sqlite_test`, and
  `boot_state_user_settings_test` failed to compile before the field existed.
- **Vitest.** The new `user-settings.test.ts` cases failed before the mapping.
- **E2E.** The first run failed because the card was not on the live page, which
  exposed the dead `TaskActionsSettings` mount. It passed after moving the row.

Verification after Task 04 is recorded in that work order.
