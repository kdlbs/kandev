---
id: "02-slash-command-feedback"
title: "Configuration slash-command feedback"
status: done
wave: 2
depends_on:
  - "01-picker-outcomes"
plan: "plan.md"
requirements:
  - REQ-AGENTS-SESSION-CONFIG-FEEDBACK-001
acceptance_criteria:
  - AC-AGENTS-SESSION-CONFIG-FEEDBACK-001.4
  - AC-AGENTS-SESSION-CONFIG-FEEDBACK-001.5
  - AC-AGENTS-SESSION-CONFIG-FEEDBACK-001.6
system_design:
  - ../../specs/agents/system-design/session-configuration-feedback.md
---

# Task 02 Configuration slash-command feedback

This work order records the original slash receipt implementation. [Task 03](task-03-muted-transcript-feedback.md)
owns the revised presentation.

## Scope

Recognize the supported ACP configuration commands on the existing prompt
path. Send the result with the live completion for the same turn. Use the
empty-turn notice path to show a temporary system message under the command.
Ordinary agent replies and failure text stay in the chat.
[Command completion design](../../specs/agents/system-design/session-configuration-feedback.md#slash-command-completion)

## Acceptance

1. A fresh, attributable update gives one system message under the command.
   A missing or ambiguous update gives a dismissible unconfirmed result
   in the same place. Existing guidance applies to other silent turns.
2. Another session, older turn, replay, or duplicate cannot create a
   confirmation. The outcome stays out of saved conversation history.
3. Desktop and phone show the receipt and its expiry, preserve the draft
   and scroll position, and allow dismissal of an unconfirmed result.

## ASCII UI preview

### UI-02 Silent-command feedback

```text
[ Conversation scrolls ]
You
  /model auto hydrafusion

System
  Copilot reported Auto.
  Fusion High requested.

[ Composer ] [ Existing controls ]
```

The system message renders with the command in the transcript on both
surfaces. This covers ACs .4 through .6. [Full UI-02 preview](plan.md#ui-02-silent-command-feedback)

## Verification

Start with `TestConfigurationCommandFeedbackPrompt` in the ACP adapter.
Use a plain `/model <id>` for an advertised model on a non-Copilot fixture.
Also exercise the reported Copilot wire shape. A target change produces an
observation. A bare `/model` uses the ordinary prompt path. An unchanged,
competing, or missing update stays unconfirmed. Add
`TestConfigurationCommandFeedbackReady` for the numbered Ready path and
a case for Core projection. Exercise duplicate completion, replay, and
settlement of the correct turn. Cover provider-specific model option IDs and
both directions of the Codex `/plan` toggle. Check that a recognized config
command cannot be retried by the existing empty-turn classifier.

Use the plain model form in a mock ACP fixture that emits a configuration
update and an empty `end_turn`. Submit it from the composer in both specs
from Task 01. Check one temporary system message under the command and
suppression of the generic hint. Run the generic empty-turn specs in both
projects. Run the desktop build before the no-build phone run.

```bash
(cd apps/backend && go test ./cmd/mock-agent ./internal/agentctl/server/adapter/transport/acp \
  ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/task/service \
  ./internal/gateway/websocket \
  -run 'Test(ConfigurationCommandFeedback.*|ConfigurationCommandAcceptsProviderModelConfigID|CodexPlanCommandResolvesBothToggleDirections|HandleAgentReady_DelayedOldGenerationDoesNotCompleteReplacementTurn|CompleteTurn_PublishesHadOutput|ConversationDeliveryProjectsSafeCoreTurnMetadataAndCompletion)$' -count=1)
(cd apps/web && pnpm run test lib/ws/client.test.ts \
  lib/ws/handlers/empty-turn-notice.test.ts \
  lib/state/slices/session-runtime/session-configuration-feedback.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --project chromium \
  tests/chat/session-configuration-feedback.spec.ts tests/chat/empty-turn.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome \
  tests/chat/mobile-session-configuration-feedback.spec.ts tests/chat/mobile-empty-turn.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- ACP `adapter_prompt.go`, `adapter_updates.go`, the Copilot dialect, and
  the mock ACP command fixture.
- Lifecycle `manager_events.go` and the existing event payloads. In the
  orchestrator, use the guarded completion in `event_handlers_agent.go`
  and the existing stream completion helper.
- `service_turns.go`, `conversation_source.go`, and the Core projection
  in `conversation_delivery.go`.
- Web `lib/ws/client.ts`, `lib/ws/handlers/turns.ts`,
  `lib/ws/handlers/empty-turn-notice.ts`, `components/task/chat/messages/status-message.tsx`,
  chat locales, and the listed tests.
- The empty-turn requirement and design, and the composer command section in
  `docs/public/tasks-and-workflows.md`, when the behavior ships.

## Dependencies

[Task 01](task-01-picker-outcomes.md) creates the desktop and phone
acceptance specs. Add the command cases to those files.

## Risk and results

Ready can settle the turn before the later stream event arrives. The outcome is
attached to the owned completion before settlement. Focused ACP, lifecycle,
orchestrator, service, Core delivery, and frontend tests passed. Desktop and
phone acceptance checks verified one temporary receipt beneath an owned
command, with replay and duplicate-completion cases covered.
