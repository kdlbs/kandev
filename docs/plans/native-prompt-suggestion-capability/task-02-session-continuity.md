---
id: "02-session-continuity"
title: "Preserve session configuration and origin continuity"
status: done
wave: 2
depends_on:
  - "01-normalize-capability"
plan: "plan.md"
requirements:
  - REQ-AGENTS-NATIVE-SUGGEST-002
  - REQ-AGENTS-NATIVE-SUGGEST-003
  - REQ-UI-PROMPT-SUGGEST-002
acceptance_criteria:
  - AC-AGENTS-NATIVE-SUGGEST-002.2
  - AC-AGENTS-NATIVE-SUGGEST-002.3
  - AC-AGENTS-NATIVE-SUGGEST-002.4
  - AC-AGENTS-NATIVE-SUGGEST-003.1
  - AC-AGENTS-NATIVE-SUGGEST-003.2
  - AC-AGENTS-NATIVE-SUGGEST-003.3
  - AC-AGENTS-NATIVE-SUGGEST-003.4
  - AC-AGENTS-NATIVE-SUGGEST-003.5
  - AC-UI-PROMPT-SUGGEST-002.2
  - AC-UI-PROMPT-SUGGEST-002.4
  - AC-UI-PROMPT-SUGGEST-002.5
  - AC-UI-PROMPT-SUGGEST-002.6
system_design:
  - ../../specs/agents/system-design/native-prompt-suggestion-capability.md
  - ../../specs/ui/system-design/composer-prompt-suggestions.md
---

# Task 02: Preserve session configuration and origin continuity

## Summary

Apply normalized support and requested enablement consistently across session
creation and restoration. Prove capability loss clears effective source while
the landed origin and conditional-write guarantees remain intact.

## In scope

- New/load/resume request assertions with all support/request combinations.
  Supported but unrequested sessions send explicit false without a raw filter.
  Unsupported sessions receive no suggestion metadata.
- Advertised resume, method-not-found fallback to load, reset, and recreation.
  Keep existing saved-session identity and error policies.
- Lifecycle fresh-launch and recreate preference/scope propagation, using existing
  executor fakes. Cover excluded utility/passthrough request paths without
  enabling host capability probes.
- New connection support replaces prior support. Current-attempt effective false
  replaces an earlier native source. Obsolete attempt events stay rejected.
- Session boundary rejection of old origin windows and notifications.
  Add only the minimal transition invalidation needed by a failing regression.
  Preserve `sessionTransitionMu`, prompt ownership guards, and lock ordering.
- Existing generation-to-turn propagation, unknown/foreign/stale rejection,
  no-write/no-publish assertions, and waiting-for-input conditional writes.

## Out of scope

Origin algorithm redesign, recovery/adoption protocol changes, new persistence,
model fallback, provider identity logic in lifecycle, and UI layout changes.

## Acceptance

1. Real adapter fake-connection tests inspect metadata on new/load/resume and
   fallback load. Fresh/recreated execution tests prove scope and preference
   reach configuration; support alone never admits a notification.
2. Replacing a supported connection with unsupported evidence, or resuming
   without the request, clears effective support/source. A delayed older
   attempt cannot restore it or reuse an old origin in the new session.
3. Existing origin-window and generation-to-turn regressions pass unchanged.
   Unknown, foreign, stale, or state-raced suggestions cause no write and no
   publication. Exact originating turns remain intact.

## Verification

Use TDD for each changed configuration or continuity condition.
The race run is justified by session-transition, notification, and source races.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -p=1 -count=1 ./internal/agentctl/server/adapter/transport/acp -run 'PromptSuggestionSessionConfiguration|PromptSuggestionSessionOriginReset|PromptSuggestionOriginSurvives')
(cd apps/backend && go test -trimpath -tags fts5 -race -p=1 -count=1 ./internal/agentctl/server/adapter/transport/acp ./internal/agent/runtime/lifecycle ./internal/orchestrator -run 'PromptSuggestion|DelayedSuggestionAfterNewerTurn|HandedOffPromptMakesSuccessorOriginAmbiguous')
mapfile -t suggestion_changed_go < <(git ls-files --modified --others --exclude-standard -- 'apps/backend/**/*.go')
if ((${#suggestion_changed_go[@]})); then
  bash scripts/lint-go-changed "${suggestion_changed_go[@]}"
fi
git diff --check
```

Add `TestPromptSuggestionSessionConfiguration` and
`TestPromptSuggestionSessionOriginReset` beside the existing adapter tests.
Add `TestPromptSuggestionOriginSurvivesFailedTransition` and
`TestPromptSuggestionOriginSurvivesSameSessionRestore` for failed and same-ID
transitions. Add `TestPromptSuggestionCapabilityRecreation` using lifecycle
executor fakes.
Extend `TestPromptSuggestionsForLaunch` and
`TestHandleAgentCapabilitiesEvent_RecordsNativePromptSuggestions`.

Retain `TestPromptSuggestionUsesItsPromptGenerationTurnID`,
`TestPromptSuggestionWithoutGenerationGetsNoTurn`, and
`TestHandlePromptSuggestionEvent_RejectsSuggestionWithoutLatestOrigin`.
Add a foreign-turn case only if absent. Assert metadata and publication together.

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session_resume.go` (only if existing metadata forwarding needs correction)
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt_suggestion.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session_test.go` and `adapter_session_resume_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/prompt_suggestion_origin_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/prompt_suggestions_test.go` and launch/recreate tests
- `apps/backend/internal/agent/runtime/lifecycle/prompt_suggestion_turn_test.go`
- `apps/backend/internal/orchestrator/event_handlers_prompt_suggestion_test.go`
- Existing source-recording and projection code only when a regression requires it

## Dependencies

Task 01. Its generic capability and provider integration must exist first.

## Risks

Session transitions and prompt completion use different locks. A reset must not
invalidate a successor prompt or introduce inverted lock acquisition.
A current-turn fallback would conceal missing origin evidence.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/native-prompt-suggestion-capability.md)
- [Agents design](../../specs/agents/system-design/native-prompt-suggestion-capability.md#connection-and-session-continuity)
- [UI design](../../specs/ui/system-design/composer-prompt-suggestions.md#suggestion-origin)
- Original Task 05 and existing reset/resume tests
- Backend and agentctl `AGENTS.md`; TDD backend test reference

## Results

Implemented. `session/new`, `session/load`, advertised `session/resume` (with
the existing method-not-found fallback), live `session/resume`, and reset all
use the same support/request builder. Supported but unrequested sessions send
an explicit disable with no raw filter; unsupported sessions send no suggestion
metadata. A proven successful transition to a different provider session ID
discards the prior suggestion window; failed transitions and same-ID restores
keep it so the next prompt remains ambiguous. The orchestrator ignores
capability events from stale resume attempts.

Verification on 2026-10-28 from repository root:

- `go test ... -run 'PromptSuggestionSessionConfiguration|PromptSuggestionSessionOriginReset'` passed.
- New failed new/load/reset and same-ID new/load/resume regressions passed;
  they failed RED against the pre-transition reset.
- `go test -race ... -run 'PromptSuggestion|DelayedSuggestionAfterNewerTurn|HandedOffPromptMakesSuccessorOriginAmbiguous'`
  passed for ACP, lifecycle, and orchestrator.
- The new recreation tests passed in the lifecycle focused run.
- `bash scripts/lint-go-changed <changed-go-files>` reported 0 issues.

The origin-reset regression was proven RED by temporarily removing the
transition reset: the delayed notification claimed the prior window and emitted
a suggestion with the old generation. The failed/same-ID regressions were
proven RED by temporarily restoring the pre-transition reset.

