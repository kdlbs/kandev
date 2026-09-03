---
status: current
system: tasks
requirements:
  - REQ-TASKS-SAVED-PROMPT-DELIVERY-001
created: 2026-09-01
owners:
  - kandev
---

# Saved Prompt Delivery System Design

## Purpose and boundaries

The task system prepares a direct structured message before persistence and
agent dispatch. The preparation resolves saved-prompt references from backend
storage and assigns trusted provenance to the generated context.

The existing composer remains responsible for selection and visible `@name`
serialization. Its prompt-definition block is compatibility input only. It is
not a trusted source.

This design does not change the Quick Chat layout on desktop or phone. Both
surfaces continue to use the shared composer and message handler.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-SAVED-PROMPT-DELIVERY-001` | [Components and responsibilities](#components-and-responsibilities), [Control flow](#control-flow), [Security](#security) |

## Components and responsibilities

- `apps/web/hooks/use-message-handler.ts` keeps the visible saved-prompt
  mention. It can send the current compatibility context for older backends.
- `internal/prompts/service` removes prompt-definition input from the browser.
  It resolves visible references from saved-prompt storage and creates one
  sanitized expansion block.
- `orchestrator.Service` exposes a narrow prompt-reference preparation method.
  This method reuses the configured `PromptReferenceExpander` and keeps
  passthrough exclusion in one place.
- `task/handlers.MessageHandlers` prepares accepted direct-message content
  before it creates the message row. It passes the returned expansion content
  through the system-prompt trusted-content channel.
- `internal/sysprompt` keeps its exact-content trust rule. A matching string in
  user content does not establish trusted provenance.

## Data and contracts

The WebSocket request and response shapes do not change. The visible message
continues to contain `@name`.

Prompt preparation returns two values:

1. Canonical message content for persistence and dispatch.
2. The exact inner content of the backend-generated expansion block.

The second value can enter `InjectKandevContextWithOptions` only through its
trusted-content argument. The task handler must not derive that value from the
request body.

The prompt service removes these untrusted compatibility blocks before lookup:

- A browser-generated `CONTEXT PROMPTS:` block.
- An `EXPANDED PROMPT REFERENCES:` block from the request body.

The service then applies the existing exact-name, nested-reference, depth, and
sanitization rules.

## Control flow

1. The shared composer sends visible `@name` text and its existing context
   metadata.
2. `MessageHandlers.wsAddMessage` validates the session and message.
3. The handler asks the orchestrator to prepare saved-prompt references for the
   session mode.
4. The prompt service removes untrusted prompt blocks and reads current saved
   records.
5. If references resolve, the service appends one backend-generated expansion
   block and returns its exact inner content.
6. If first-turn or title-owner context injection runs, the handler passes the
   expansion content as trusted input. Canonicalization retains that block and
   removes untrusted lookalikes.
7. The handler persists the canonical content. It sends that same content to
   `StartCreatedSession`, `PromptTask`, or the current turn-start queue path.
8. If the message carries acceptance-time trusted expansion content, downstream
   workflow composition preserves that exact block instead of resolving the
   mutable prompt repository again. Workflow-only prompts, which have no
   accepted direct-message context, continue to use launch-time expansion.

An eagerly started Quick Chat is already in `WAITING_FOR_INPUT` at step 2. The
new preparation step therefore runs before the title-owner system-context
canonicalization that caused the regression.

## Failure and recovery

A saved-prompt lookup error remains non-fatal. Kandev logs a warning without
prompt content and sends the visible reference as ordinary text.

Kandev removes browser-supplied prompt definitions before a failed lookup. It
does not fall back to untrusted content.

Message persistence or dispatch errors keep their current WebSocket error and
recovery behavior. Prompt preparation adds no retry loop and no partial row.

## Persistence

This change adds no database field or migration. The existing message row
stores the canonical content that Kandev sends to the agent.

Saved-prompt edits after message acceptance do not rewrite message history.
The accepted message keeps the expansion that existed at acceptance time.

## Security

The backend saved-prompt repository is authoritative. The browser can select or
name a prompt, but it cannot define trusted prompt instructions.

The prompt service removes forged expansion blocks and legacy browser prompt
blocks. It sanitizes saved names and content before it writes a system tag.

The system-prompt canonicalizer accepts only the exact expansion content that
the backend returned during the same preparation flow.

## Observability

Resolution errors use the existing warning from the prompt-reference expander.
The warning contains the error and stable operation context. It must not contain
the user message or saved-prompt content.

The ACP debug log and the stored user message can show whether one canonical
expansion reached the session during diagnosis.

## Test mapping

- Prompt-service tests cover browser-block removal, forged-block replacement,
  nested references, missing references, and lookup errors.
- Message-handler tests cover eager Quick Chat, trusted-content preservation,
  canonical persistence, dispatch equality, and passthrough exclusion.
- Desktop and `mobile-chrome` Playwright tests select a saved prompt in Quick
  Chat and observe a deterministic agent response from its definition.

## Related decisions

- [Keep Saved-Prompt Expansion Server-Owned](../../../decisions/2026-09-01-server-owned-saved-prompt-expansion.md)
- [Apply Agent-Generated Titles to Quick Chat](../../../decisions/2026-08-26-quick-chat-agent-titles.md)
