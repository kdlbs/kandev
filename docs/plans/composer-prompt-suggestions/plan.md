---
created: 2026-10-06
status: in_progress
requirements:
  - REQ-UI-PROMPT-SUGGEST-001
  - REQ-UI-PROMPT-SUGGEST-002
  - REQ-UI-PROMPT-SUGGEST-003
  - REQ-UI-PROMPT-SUGGEST-004
system_design:
  - ../../specs/ui/system-design/composer-prompt-suggestions.md
legacy_specs: []
---

# Implementation Plan: Suggest the Next Prompt in the Chat Composer

## Overview

The feature landed in PR #4306 on 2026-10-28. The
[native capability follow-up](../native-prompt-suggestion-capability/plan.md)
replaces its namespace-only detection contract through a separate draft package.
The original work orders and verification results below remain historical evidence.
The follow-up's Agents design owns its proposed evidence and compatibility rules.

The feature shows the user's likely next prompt as ghost text in the empty
composer. Tab fills it in for editing, Enter sends it, and Escape dismisses it.

Suggestions are native only. Claude sessions use Claude Code's own suggestion:
Claude Code forks the conversation with the prompt cache warm, and measured
locally the suggestion arrives about 1.7 seconds after the turn result. Agents
without native support keep the composer's normal behavior, and Kandev never
generates a replacement.

The preference is opt-in per user.

The first version also ran a utility-agent fallback for other agents. The
maintainer review of kdlbs/kandev#4306 asked for native suggestions only, and
Task 04 removed the fallback.

### Evidence gathered during design

- Claude Code 2.1.285 forks the main conversation with a `[SUGGESTION MODE]`
  prompt, denies all tools, and skips the transcript and cache writes.
- It suppresses suggestions when:
  - the conversation is early (fewer than 2 assistant messages);
  - the last response was an API error;
  - a permission or elicitation is pending;
  - the session is in plan mode or rate-limited;
  - the cache is cold (more than 10,000 uncached tokens).
- Its client filters reject meta text, more than 12 words, 100 or more
  characters, several sentences, formatting, evaluative text, and agent voice.
- In UI, Tab accepts, Enter sends, and suggestions back off after unused
  streaks.
- `claude-agent-acp` 0.81.2, the version Kandev pins, accepts
  `_meta.claudeCode.options.promptSuggestions`. Its
  `_meta.claudeCode.emitRawSDKMessages` setting forwards the SDK message as the
  `_claude/sdkMessage` extension notification.
- The `kdlbs/acp-go-sdk` fork routes inbound extension notifications to
  `MethodNotFound`, so they are dropped. Task 02 fixes this.
- SDK stream-json probe results: the suggestion came 1.78 s and 1.68 s after
  `result`. The suggestions were "sim, corre a migration e os testes" and "sim,
  faz commit".

## Scope

### In scope

- The opt-in preference, plus docs.
- An ACP SDK fork change so extension notifications reach the client.
- Native delivery for Claude sessions:
  - Claude `_meta` on session new and load;
  - the `_claude/sdkMessage` handler and stale drop;
  - the `prompt_suggestion` stream event;
  - the orchestrator's state-guarded metadata write;
  - the `session.prompt_suggestion` WebSocket event;
  - mock agent support.
- No suggestion, and no generated replacement, for sessions without native
  support or turns without a native suggestion.
- Ghost text, the Tab, Enter, and Escape keys, a tap-to-accept chip, and both
  task chat and Quick Chat on desktop and phone.

### Out of scope

- Model-generated or utility-agent suggestions.
- Native sources other than Claude.
- Pausing native generation when no browser shows the session.
- Kandev-side back-off, and multiple suggestions.
- Passthrough, run-transcript, and task-creation composers.
- Runtime flags.

## Work orders and dependency order

| Order | Work order | Wave | Depends on |
| --- | --- | --- | --- |
| 1 | [Task 01: Prompt suggestion preference](task-01-suggestion-agent-and-preference.md) | 1 | none |
| 2 | [Task 02: Native Claude suggestion delivery](task-02-native-suggestion-delivery.md) | 2 | 01 |
| 3 | [Task 03: Composer suggestion UI](task-03-composer-prompt-suggestion.md) | 3 | 01, 02 |
| 4 | [Task 04: Native-only scope](task-04-native-only-scope.md) | 4 | 03 |
| 5 | [Task 05: Review fixes](task-05-review-fixes.md) | 5 | 04 |

The order is strictly sequential. Task 02's launch path reads the preference
that Task 01 adds. Task 03 consumes both the preference and the native event.
The `kdlbs/acp-go-sdk` fork change inside Task 02 can start in parallel with
Task 01, because it is in another repository.

- [x] Task 01
- [x] Task 02
- [x] Task 03
- [x] Task 04
- [x] Task 05

## Technical approach

See the system design for contracts. Highlights:

1. **SDK fork.** Add `ExtensionNotificationHandler` to `kdlbs/acp-go-sdk`, tag
   or pin a revision, and bump `replace` in `apps/backend/go.mod`.
2. **agentctl.**
   - Add `prompt_suggestions` to the instance creation request and carry it
     to `shared.Config`.
   - Add a `promptSuggestions` capability to `acpDialect`, set only by the
     Claude and mock dialects, and attach the `_meta` to session new, load,
     and resume.
   - Implement `HandleExtensionNotification` in `internal/agentctl/server/acp/client.go`.
   - Emit `EventTypePromptSuggestion`, and drop stale suggestions using the
     adapter's prompt generation.
3. **Backend.**
   - Negotiate native suggestions from the `initialize` `_meta.claudeCode`
     advertisement, report `supports_prompt_suggestions` on `agent_capabilities`,
     and record it as session metadata `prompt_suggestion_source`.
   - In launch and recreate, set the instance flag only for interactive task
     and Quick Chat sessions.
   - Add `handlePromptSuggestionEvent`, which writes `prompt_suggestion` with
     `SetSessionMetadataKeyIfState` and publishes `session.prompt_suggestion`.
4. **Frontend.**
   - Add the WebSocket handler that writes session metadata.
   - Add the pure helpers and the hook. They select a native suggestion only
     for a native session and its own turn, and never request one.
   - Add the placeholder decoration, the keymap, the hint chip, and the
     settings switch.

## ASCII UI preview

Structural choices are requirements:

- ghost text occupies the placeholder slot;
- the hint chip sits in the composer's trailing hint slot;
- on desktop the chip shows `Tab`, and Enter sends;
- on phones the chip is a "Use reply" button at least 44 px tall;
- the settings row lives in the Conversation tab of Task behavior.

Exact copy and spacing are illustrative. Use existing tokens and localized
strings.

### UI-01: Composer after an agent turn with a suggestion (desktop)

Entry point: task chat or Quick Chat. The session is waiting for input and the
draft is empty. Maps to `AC-UI-PROMPT-SUGGEST-004.1`, `.2`, `.3`, and `.9`.

```text
+--------------------------------------------------------------------+
| Agent: The migration 0042_tags.sql is ready. Should I run it and   |
|        the test suite?                                             |
+--------------------------------------------------------------------+
+--------------------------------------------------------------------+
| sim, corre a migration e os testes              (muted ghost text) |
|                                         [Tab] Accept · Enter sends |
| [Agent v] [Model v] [Plan]                    [Enhance] [Send >]   |
+--------------------------------------------------------------------+
```

### UI-02: After Tab (desktop)

Maps to `AC-UI-PROMPT-SUGGEST-004.2` and `.5`.

```text
+--------------------------------------------------------------------+
| sim, corre a migration e os testes|             (normal draft text) |
|                                                                    |
| [Agent v] [Model v] [Plan]                    [Enhance] [Send >]   |
+--------------------------------------------------------------------+
```

### UI-03: Phone composer with a suggestion

Same composition. The ghost text wraps, and the chip is a touch button. Maps to
`AC-UI-PROMPT-SUGGEST-004.4` and `.8`.

```text
+--------------------------------+
| sim, corre a migration e os    |
| testes              (ghost)    |
|                 [ Use reply ]  |  <- >= 44 CSS px, fills for editing
| [+] [Agent v]        [Send >]  |  <- Send with empty draft sends suggestion
+--------------------------------+
```

### UI-04: Settings > Task behavior > Conversation

Maps to `AC-UI-PROMPT-SUGGEST-001.1` and `.4`.

```text
+--------------------------------------------------------------------+
| Suggest next prompt                                   (i)  [ on  ] |
| Show a suggested reply after each agent turn.                      |
|   (i) Only agents with native suggestions, such as Claude Code,    |
|       suggest replies, from their next start.                      |
+--------------------------------------------------------------------+
```

Phone: the row stacks vertically, as other settings rows do. The switch keeps
its touch target.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-UI-PROMPT-SUGGEST-001.1` | Go settings default test. Settings E2E. |
| `AC-UI-PROMPT-SUGGEST-001.2` | Launch-flag Go test with the preference off. Selection unit test. E2E with the preference off. |
| `AC-UI-PROMPT-SUGGEST-001.3` | Go settings round-trip test. Settings E2E reload. |
| `AC-UI-PROMPT-SUGGEST-001.4` | Hook unit test of immediate rendering after a toggle. Help-text keys. |
| `AC-UI-PROMPT-SUGGEST-002.1` | Launch-flag Go tests (Claude, mock, others). Adapter `_meta` unit test. |
| `AC-UI-PROMPT-SUGGEST-002.2` | Capability-record Go test. Selection unit test: a non-native session with a suggestion payload shows nothing. |
| `AC-UI-PROMPT-SUGGEST-002.3` | Orchestrator handler Go test (metadata and event). WebSocket handler unit test. E2E with the mock agent, including reload. |
| `AC-UI-PROMPT-SUGGEST-002.4` | agentctl stale-drop unit test. State-guarded write Go test. |
| `AC-UI-PROMPT-SUGGEST-002.5` | Launch-flag Go tests for Office, automation, utility, and passthrough sessions. |
| `AC-UI-PROMPT-SUGGEST-003.1` | Selection unit test. E2E: a non-native agent keeps the normal placeholder and Tab behavior. |
| `AC-UI-PROMPT-SUGGEST-003.2` | Selection unit test with no native payload. |
| `AC-UI-PROMPT-SUGGEST-003.3` | Selection and hook unit tests for a suggestion bound to an older turn. |
| `AC-UI-PROMPT-SUGGEST-004.1`–`.9` | Decoration and keymap unit tests. Desktop, Quick Chat, and phone E2E. `i18n:check`. |

## E2E tests

- `apps/web/e2e/tests/chat/prompt-suggestions.spec.ts` (desktop):
  - **Native (mock agent).**
    1. Enable the preference and start a task.
    2. Complete one turn, and assert the ghost text and chip.
    3. Press Tab, assert the draft, then clear it.
    4. Assert the ghost text returns. Press Enter and assert the suggestion was
       sent as a user message.
    5. After the next turn, press Escape and assert it is dismissed.
    6. Reload and assert the latest native suggestion is restored.
  - **Unsupported agent.** With the preference on, a mock agent that does not
    advertise Claude Code completes a turn. Assert no ghost text and no chip,
    and that Tab keeps its existing behavior.
  - **Quick Chat.** Escape dismisses the suggestion and keeps the dialog open.
- `apps/web/e2e/tests/chat/mobile-prompt-suggestions.spec.ts` (`mobile-chrome`):
  the "Use reply" chip is at least 44 px and fills the draft.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/backend && go test ./internal/agentctl/server/acp/... ./internal/agentctl/server/adapter/transport/acp/... \
  ./internal/orchestrator/... ./internal/agent/agents/... ./internal/user/... \
  ./internal/settingscatalog/... ./cmd/mock-agent/...
cd apps/web && pnpm test -- lib/prompt-suggestion.test.ts hooks/use-prompt-suggestion.test.ts \
  components/task/chat/tiptap-dynamic-placeholder.test.ts lib/ws/handlers/prompt-suggestions.test.ts
cd apps/web && pnpm e2e:run tests/chat/prompt-suggestions.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-prompt-suggestions.spec.ts
cd apps/web && pnpm run typecheck && pnpm run i18n:check
cd apps && pnpm --filter @kandev/web lint
make -C apps/backend lint
```

Manual evidence for Task 02 uses `/acp-debug` against the real
`claude-agent-acp@0.81.2`. Over two turns with the Claude `_meta`, it confirms
that a `_claude/sdkMessage` `prompt_suggestion` frame follows the second turn's
`session/prompt` response. Record the frame and its timing in the Task 02
results.

## Risks

- **Cross-repo SDK change.** `kdlbs/acp-go-sdk` must ship first, and the
  `replace` bump must not pull unrelated breaking changes. Diff the fork range
  before bumping.
- **Adapter upgrades.** A future `claude-agent-acp` could rename the raw
  forwarding. A dialect unit test pins the `_meta` shape, and the mock agent
  mirrors the frame. Re-run the `/acp-debug` probe on every adapter bump.
- **Native silence.** Claude Code suppresses early turns, cold-cache turns, plan
  mode, and unused streaks. Native sessions then show nothing, by design.
- **Unviewed sessions.** Native generation still runs when nobody is watching.
  Its cost is a cache read plus a short suffix. Office and automation sessions
  never request it.
- **Enter sends.** Enter on an empty draft sends the visible suggestion, which
  the user chose explicitly. The ghost text and the "Enter sends" hint make it
  visible. Enter with a menu open is unchanged.
- **Tab and Escape regressions.** Claim these keys only when ghost text is
  visible.
- **Coverage limited to Claude Code.** Other agents show no suggestion until
  they expose a native source.

## Public documentation

Update `docs/public/developer-tools.md` with a "Prompt suggestions" section
covering the switch, which agents support it, Tab, Enter, Escape, and tap, and
the next-session-start note. Run
`/docs-maintainer` in Task 01 and again in Task 03 for the key behavior.

## Decisions

No ADR. The design reuses ACP `_meta`, extension notifications, and session
metadata.
