---
id: "01-suggestion-agent-and-preference"
title: "Add the prompt suggestion preference and fallback utility agent"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-PROMPT-SUGGEST-001
  - REQ-UI-PROMPT-SUGGEST-003
acceptance_criteria:
  - AC-UI-PROMPT-SUGGEST-001.1
  - AC-UI-PROMPT-SUGGEST-001.3
  - AC-UI-PROMPT-SUGGEST-001.4
  - AC-UI-PROMPT-SUGGEST-001.5
  - AC-UI-PROMPT-SUGGEST-001.6
  - AC-UI-PROMPT-SUGGEST-003.5
  - AC-UI-PROMPT-SUGGEST-003.8
system_design:
  - ../../specs/ui/system-design/composer-prompt-suggestions.md
---

# Task 01: Add the Prompt Suggestion Preference and Fallback Utility Agent

## Summary

Add the per-user `prompt_suggestions` and `prompt_suggestions_fallback`
preferences end to end, both default off. Add the Settings > Task behavior >
Conversation controls:

- the main switch;
- the fallback switch, shown only when the main switch is on;
- an agent profile selector, shown only when the fallback switch is on. It is
  bound to the `suggest-next-prompt` built-in's profile binding. Seed the
`builtin-suggest-next-prompt` utility agent that the composer fallback uses.

## In scope

- `prompt_suggestions` and `prompt_suggestions_fallback` through:
  - user models, DTO, service, and controller;
  - the JSON settings store;
  - settings catalog defaults;
  - the boot payload;
  - the frontend `promptSuggestions` settings slice and SSR mapping.
- `PromptSuggestionSettings`:
  - a main Switch row;
  - a nested fallback Switch row, rendered only when the main switch is on;
  - a nested `UtilityAgentProfilePicker`, rendered only when the fallback switch
    is on. It reads and saves the `builtin-suggest-next-prompt` binding
    (`inherit` or an explicit profile) through the existing utility agent API.
    When the binding is unresolvable, it shows a localized hint.

  Each row has a save contributor and a discovery target. The info popovers
  say:
  - main switch: Claude sessions use Claude's native suggestions from their
    next start;
  - fallback switch: each turn costs one extra agent call and is slower than a
    native suggestion.
- `apps/backend/config/utilityagents/suggest-next-prompt.md`. This is an
  original Kandev prompt, not a verbatim copy of Claude Code's. It encodes:
  - predict what the user would type, not what the agent recommends;
  - 2 to 12 words, in the user's language and style;
  - never an evaluation, a question, agent voice, a new idea, or several
    sentences;
  - stay silent when the next step is not obvious, after an error, or when the
    suggestion could be unsafe;
  - reply with the suggestion only, or nothing.

  It uses `{{TaskTitle}}` and `{{ConversationHistory}}`.
- A `builtinDefs` entry: ID `builtin-suggest-next-prompt`, name
  `suggest-next-prompt`.
- Copy in every locale catalog. Use `pnpm run i18n:zh-hant` for the Traditional
  Chinese pair. No em dashes.
- `docs/public/developer-tools.md`: the built-in list, plus the first version of
  a "Prompt suggestions" section covering the switch, native versus fallback,
  and the next-session-start note.

## Out of scope

- Agent launch, agentctl, orchestrator, and WebSocket changes (Task 02).
- Composer, hook, and editor changes (Task 03).

## Acceptance

- A fresh database and an existing database both list `suggest-next-prompt` in
  Settings > Utility Agents. A Go test asserts that the template contains the
  rule phrases and both template variables.
- `prompt_suggestions` defaults to `false`, round-trips through user settings
  `PATCH`, and appears in the boot payload. An absent stored value reads as
  `false`.
- Settings > Task behavior shows the main switch, the conditional fallback switch,
  and the conditional profile selector (UI-04). Both switches persist across a
  reload. A profile chosen in General appears for `suggest-next-prompt` in
  Settings > Utility Agents.

## ASCII UI preview

See [plan UI-04](plan.md#ui-04-settings--general--task-actions).

```text
| Suggest next prompt                                   (i)  [ off ] |
| Show a suggested reply after each agent turn.                      |
```

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/backend && go test ./internal/utility/... ./internal/user/... ./internal/settingscatalog/... \
  ./internal/backendapp/... ./config/...
cd apps/web && pnpm test -- lib/ssr/user-settings.test.ts
cd apps/web && pnpm e2e:run tests/settings/utility-agents.spec.ts tests/settings/prompt-suggestion-settings.spec.ts
cd apps/web && pnpm run typecheck && pnpm run i18n:check
cd apps && pnpm --filter @kandev/web lint
make -C apps/backend lint
```

## Files likely touched

- `apps/backend/config/utilityagents/suggest-next-prompt.md`
- `apps/backend/internal/utility/store/builtins.go` and its test
- `apps/backend/internal/user/{models,dto,service,controller,store}/…`
- `apps/backend/internal/settingscatalog/defaults.go`
- `apps/backend/internal/backendapp/boot_state_routes.go`
- `apps/web/lib/state/slices/settings/types.ts`
- `apps/web/lib/ssr/user-settings.ts` and its test
- `apps/web/lib/settings-discovery/catalog/preferences.ts`
- `apps/web/components/settings/prompt-suggestion-settings.tsx`
- `apps/web/components/settings/task-behavior-settings.tsx` and
  `task-behavior-tabs.ts` (Conversation tab)
- `apps/web/components/settings/utility-agent-profile-picker.tsx` (reuse, no
  behavior change)
- `apps/web/e2e/tests/settings/prompt-suggestion-settings.spec.ts`
- `apps/web/src/locales/*/settings.json`
- `apps/web/e2e/tests/settings/utility-agents.spec.ts`
- `docs/public/developer-tools.md`

## Dependencies

None.

## Risks

- Tests that count built-in utility agents must be updated, not bypassed.
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
- `apps/backend/config/utilityagents/summarize-session.md` as the template
  pattern

## Results

Implemented both preferences end to end, the `suggest-next-prompt` built-in
(`SuggestNextPromptAgentID`) with an original Kandev prompt, the settings rows,
and the public docs.

The rows live in **Settings > Task behavior > Conversation**, not General. The
`TaskActionsSettings` card in `general-settings.tsx` is no longer mounted
anywhere, and the live page is `task-behavior-settings.tsx`. The specs and plan
were updated to match.

The profile selector reuses `UtilityAgentProfilePicker`. It saves the built-in's
binding through `PATCH /api/v1/utility/agents/builtin-suggest-next-prompt` with
its own save contributor.

Red-Green evidence:

- **Go.** The new tests in `dto_test`, `service_test`, `sqlite_test`,
  `boot_state_user_settings_test`, and `utility/store/builtins_test` failed to
  compile before the fields and constant existed.
- **Vitest.** The two new `user-settings.test.ts` cases failed (2 failed) before
  the mapping. `prompt-suggestion-settings-model.test.ts` failed (no module)
  before the helper.
- **E2E.** The first run failed because the card was not on the live page, which
  exposed the dead `TaskActionsSettings` mount. It passed after moving the rows.

Verification:

- `go test ./internal/utility/... ./internal/user/... ./internal/settingscatalog/... ./internal/backendapp/ ./config/...`
  passed.
- `go run ./cmd/settings-catalog` regenerated both contract snapshots, and
  `--check` reports them fresh.
- `pnpm vitest run lib/ssr/user-settings.test.ts components/settings/prompt-suggestion-settings-model.test.ts components/settings/task-behavior*`
  passed.
- `pnpm e2e:run tests/settings/prompt-suggestion-settings.spec.ts` passed
  (1 test).
- `tsc --noEmit`, targeted eslint, and `pnpm run i18n:check` passed.
