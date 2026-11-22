---
id: "04-native-only-scope"
title: "Narrow prompt suggestions to native support only"
status: done
wave: 4
depends_on:
  - "03-composer-prompt-suggestion"
plan: "plan.md"
requirements:
  - REQ-UI-PROMPT-SUGGEST-001
  - REQ-UI-PROMPT-SUGGEST-002
  - REQ-UI-PROMPT-SUGGEST-003
acceptance_criteria:
  - AC-UI-PROMPT-SUGGEST-001.2
  - AC-UI-PROMPT-SUGGEST-001.4
  - AC-UI-PROMPT-SUGGEST-002.2
  - AC-UI-PROMPT-SUGGEST-003.1
  - AC-UI-PROMPT-SUGGEST-003.2
  - AC-UI-PROMPT-SUGGEST-003.3
system_design:
  - ../../specs/ui/system-design/composer-prompt-suggestions.md
---

# Task 04: Narrow Prompt Suggestions to Native Support Only

## Summary

The maintainer review of kdlbs/kandev#4306 asked for native prompt suggestions
only. Kandev must show a suggestion only when the active agent advertises and
delivers native support, and must never generate a replacement. Remove the
utility-agent fallback and everything that exists only for it. Keep capability
negotiation, native delivery, and the desktop and phone composer behavior, and
add targeted coverage for unsupported agents and stale suggestions.

## In scope

- **Backend removal.**
  - The `suggest-next-prompt` built-in: its template, its `builtinDefs` entry,
    and the ID constant.
  - The utility execute request's `fallback_agent_profile_id` and the service
    path that resolved the session's profile as a last resort.
  - The `prompt_suggestions_fallback` user setting in models, DTO, service,
    controller, store, settings catalog, and boot payload.
- **Frontend removal.**
  - The fallback request, timeout, transcript builder, response filters, and
    result cache in `lib/prompt-suggestion.ts` and
    `hooks/use-prompt-suggestion.ts`.
  - The fallback switch and the profile selector in `PromptSuggestionSettings`,
    their settings-discovery entries, the store and SSR field, and their copy in
    every locale catalog.
  - The `fallback_agent_profile_id` field in the utility API client.
- **Selection.** A pure `selectVisibleSuggestion` helper that shows a native
  suggestion only for a native session, in its own turn, while the composer is
  idle, and not dismissed. Dismissals keep a bounded browser store of
  `{ sessionId, turnKey }`.
- **Tests.**
  - Unit: a non-native session never shows a suggestion, even with a
    `prompt_suggestion` payload in its metadata; a stale suggestion for an
    older turn stays hidden; dismissal storage.
  - E2E: a non-native agent shows the normal placeholder, and Tab keeps its
    existing behavior, after a turn. The fallback E2E case is removed.
- **Docs.** Update the requirements, the system design, this plan, and
  `docs/public/developer-tools.md`.

## Out of scope

- Replacing the `replace` directive for `github.com/coder/acp-go-sdk`. It waited
  on kdlbs/acp-go-sdk#6 and is tracked in [Task 02](task-02-native-suggestion-delivery.md).
- Native sources other than Claude Code.

## Acceptance

- No code path, setting, copy, or test of the utility fallback remains.
- A non-native session shows no ghost text and no hint chip, and Tab, Enter,
  and Escape behave as before.
- A native suggestion for an older turn never renders.
- The native desktop, Quick Chat, and phone E2E cases still pass.

## Verification

```bash
cd apps/backend && go test ./internal/utility/... ./internal/user/... ./internal/settingscatalog/... \
  ./internal/backendapp/... ./internal/orchestrator/... ./cmd/mock-agent/...
cd apps/web && pnpm vitest run lib/prompt-suggestion.test.ts hooks/use-prompt-suggestion.test.ts \
  components/settings lib/ssr/user-settings.test.ts
cd apps/web && pnpm e2e:run tests/chat/prompt-suggestions.spec.ts tests/settings/prompt-suggestion-settings.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-prompt-suggestions.spec.ts
cd apps/web && pnpm run typecheck && pnpm run i18n:check
cd apps && pnpm --filter @kandev/web lint
make -C apps/backend lint
```

## Files likely touched

- `apps/backend/config/utilityagents/suggest-next-prompt.md` (deleted)
- `apps/backend/internal/utility/{store,service,controller,dto}/…`
- `apps/backend/internal/user/{models,dto,service,controller,store}/…`
- `apps/backend/internal/settingscatalog/defaults.go`
- `apps/backend/internal/backendapp/boot_state_routes.go`
- `apps/web/lib/prompt-suggestion.ts` and its test
- `apps/web/hooks/use-prompt-suggestion.ts` and its test
- `apps/web/components/settings/prompt-suggestion-settings*.ts*`
- `apps/web/lib/settings-discovery/…`, `apps/web/lib/ssr/user-settings.ts`,
  `apps/web/lib/state/slices/settings/types.ts`, `apps/web/lib/types/…`
- `apps/web/lib/api/domains/utility-api.ts`
- `apps/web/src/locales/*/settings.json`
- `apps/web/e2e/tests/chat/prompt-suggestions.spec.ts`,
  `apps/web/e2e/tests/settings/prompt-suggestion-settings.spec.ts`
- `docs/public/developer-tools.md`

## Dependencies

- Task 03.

## Risks

- **Shared utility code.** The fallback profile path lives in the utility
  service. Removing it must leave every other utility action's fail-closed
  resolution unchanged.
- **Generated settings contracts.** Regenerate them with
  `go run ./cmd/settings-catalog` instead of editing by hand.

## Parallelism

`sequential`

## Inputs

- The maintainer comment on kdlbs/kandev#4306 (2026-10-12).
- `docs/specs/ui/requirements/composer-prompt-suggestions.md`
- `docs/specs/ui/system-design/composer-prompt-suggestions.md`

## Results

Implemented after rebasing the branch onto `main`.

- **Removed.** The `suggest-next-prompt` built-in and its template, the utility
  request's `fallback_agent_profile_id` and the service's last-resort profile
  path, the `prompt_suggestions_fallback` setting end to end, the fallback
  switch and profile selector, the settings model, and their copy in every
  catalog. The settings contracts were regenerated.
- **Selection.** `selectVisibleSuggestion` returns a suggestion only for a
  native session in its own turn. Without turn records, the turn key now comes
  from the newest agent message's `turn_id`, so a reload cannot resurrect a
  suggestion from an older turn. Only a message without a turn ID still trusts
  the backend binding.
- **Dismissals.** Stored as `{ sessionId, turnKey }` under
  `kandev.promptSuggestionDismissals.v1`.

Red-Green evidence:

- `lib/prompt-suggestion.test.ts` failed (17 failed) before the new helpers, and
  the stale-reload case failed before message turn IDs were used.

Verification:

- `go test` for utility, user, settingscatalog, backendapp, orchestrator,
  mock-agent, agentctl adapter, acp, process, and lifecycle packages passed.
  `make -C apps/backend lint` reports 0 issues.
- `pnpm vitest run` over the suggestion helpers and hook, settings, chat,
  ws handlers, settings slice, and SSR mapping: 494 files and 4,308 tests
  passed.
- `pnpm e2e:run tests/chat/prompt-suggestions.spec.ts tests/settings/prompt-suggestion-settings.spec.ts`:
  7 passed, including the stale suggestion and unsupported agent cases.
- `pnpm e2e:run --project mobile-chrome tests/chat/mobile-prompt-suggestions.spec.ts`:
  1 passed.
- `pnpm run typecheck`, targeted eslint, `pnpm run i18n:check`,
  `pnpm run i18n:ratchet`, `make lint-architecture lint-harness`, and the spec
  validators passed.

Manual test on 2026-10-13 in a secondary instance: a Claude session showed the
native suggestion; OpenCode with a Claude model and the custom Atomica Code ACP
agent kept the normal composer with no suggestion.

After kdlbs/acp-go-sdk#6 merged, the `go.mod` `replace` moved to
`kdlbs/acp-go-sdk` (see Task 02).
