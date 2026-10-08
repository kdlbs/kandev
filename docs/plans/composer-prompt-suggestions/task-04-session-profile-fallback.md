---
id: "04-session-profile-fallback"
title: "Use the session profile when no suggestion profile is configured"
status: done
wave: 4
depends_on:
  - "03-composer-prompt-suggestion"
plan: "plan.md"
requirements:
  - REQ-UI-PROMPT-SUGGEST-001
  - REQ-UI-PROMPT-SUGGEST-003
acceptance_criteria:
  - AC-UI-PROMPT-SUGGEST-001.6
  - AC-UI-PROMPT-SUGGEST-003.1
  - AC-UI-PROMPT-SUGGEST-003.9
system_design:
  - ../../specs/ui/system-design/composer-prompt-suggestions.md
---

# Task 04: Use the Session Profile When No Suggestion Profile Is Configured

## Summary

During the test phase, the fallback failed on an install with no default
utility profile ("utility agent profile is required"). The user decided that
the suggestion fallback then uses the session's own agent profile. The order is
the `suggest-next-prompt` binding, then the default utility profile, then the
session profile. Other utility actions are unchanged.

## In scope

- Utility execute DTO and service: optional `fallback_agent_profile_id`, used
  only for an inheriting binding without a default. It is validated by the
  existing profile resolver.
- Suggestion hook: sends `session.agent_profile_id`.
- Settings: replace the "fallback cannot run" error with a hint that the
  session's profile is used. Copy goes in all locales.
- Tests: service unit tests, hook payload test, settings model test, and an E2E
  run of the fallback with no default profile.

## Out of scope

- Changing resolution for review, commit, PR, summary, or enhance actions.
  That needs a separate decision record.

## Acceptance

- With no binding and no default, the fallback runs on the session's profile.
  With a binding or a default, that profile wins.
- An ineligible or missing fallback profile still fails closed. Requests without
  the field behave exactly as before.
- Settings no longer shows an error for the no-default case.

## Verification

```bash
cd apps/backend && go test ./internal/utility/...
cd apps/web && pnpm vitest run hooks/use-prompt-suggestion.test.ts components/settings/prompt-suggestion-settings-model.test.ts
cd apps/web && pnpm e2e:run tests/chat/prompt-suggestions.spec.ts
cd apps/web && pnpm run typecheck && pnpm run i18n:check
make lint
```

## Results

Implemented.

- **Backend.** `ExecutePromptRequest.fallback_agent_profile_id` reaches
  `DefaultUtilitySettings.FallbackProfileID` through
  `controller.withFallbackProfile`, which copies and never mutates the caller's
  defaults. The service uses it only in the inheriting branch when the default
  profile is empty. The same `profileResolver` validates it.
- **Frontend.** The suggestion hook sends `session.agent_profile_id`. Settings
  shows a muted hint (`promptSuggestionsSessionProfileHint`) instead of the
  error, with copy in all locales.
- **Docs.** The public docs describe the resolution order.

Red-Green evidence:

- `TestPreparePromptRequest_FallbackProfileOrder` and `TestWithFallbackProfile`
  failed to compile before the field and helper existed.
- The hook test "sends the session's agent profile" failed before the payload
  change.
- All passed after implementation.

Verification:

- `go test ./internal/utility/...` passed.
- The hook and settings unit tests passed.
- `pnpm e2e:run tests/chat/prompt-suggestions.spec.ts`: 4 passed. The fallback
  test now asserts `fallback_agent_profile_id`.
- `pnpm e2e:run tests/settings/prompt-suggestion-settings.spec.ts`: 1 passed.
- `pnpm run i18n:check` passed.

Follow-up round (manual-test feedback):

- The profile help moved into a `SettingsInfo` popover with the new
  `promptSuggestionsProfileSpeed` copy (fast model, up to 15 seconds).
- A new E2E proves a fallback that arrives while the user is typing leaves the
  draft intact and appears once the draft is cleared.
- That E2E exposed a cache bug: enabling the preference started a request that
  a reload aborted, and the abort was cached as `null`, hiding the suggestion
  for the turn. Transport rejections are no longer cached
  (`does not cache a request that never reached the server` failed first).
- `pnpm e2e:run` for the chat, settings, and mobile prompt-suggestion specs:
  7 passed.
- Manual testing showed the model sometimes stayed silent after a
  non-coding answer, because the prompt forbade questions. The prompt now
  allows a natural follow-up question (AC-UI-PROMPT-SUGGEST-003.5);
  `TestSeedBuiltinAgents_IncludesSuggestNextPrompt` failed first. The speed
  hint now says that a slower answer shows no suggestion for that turn.
- Manual testing found that the send button ignored a visible suggestion; only
  the shortcut sent it. Both now go through `submitWithPromptSuggestion`
  (AC-UI-PROMPT-SUGGEST-004.3); the new E2E failed first.
