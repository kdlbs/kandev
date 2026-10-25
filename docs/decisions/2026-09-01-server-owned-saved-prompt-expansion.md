# ADR-2026-09-01-server-owned-saved-prompt-expansion: Keep Saved-Prompt Expansion Server-Owned

**Status:** accepted
**Date:** 2026-09-01
**Area:** backend, frontend, protocol, security

## Context

The chat composer shows a saved prompt as `@name`. It also adds the selected
definition to a hidden browser-generated block.

An eager Quick Chat starts ACP before its first user request. The first message
can then pass through backend system-context canonicalization. That process
removes the browser block because it has no server provenance. The running
agent receives only `@name` and cannot apply the saved definition.

The backend already owns saved-prompt storage, reference resolution, expansion
sanitization, and exact-content trust. Direct chat delivery did not use that
complete path before message persistence.

## Decision

The backend owns saved-prompt expansion for direct structured chat messages.
The browser supplies a visible reference, not a trusted definition.

Kandev removes browser-supplied prompt-definition and expansion blocks. It then
resolves `@name` against the current saved-prompt repository. The backend passes
the generated expansion through the exact trusted-content channel.

Kandev prepares the canonical message before persistence and dispatch. The
stored user message and the ACP prompt therefore contain the same expansion.

Passthrough sessions remain excluded because hidden system blocks become
visible terminal input.

## Consequences

- Eager Quick Chat can use a saved prompt after ACP initialization.
- A typed reference and a selected reference have the same backend behavior.
- Browser content cannot replace a saved definition or forge expansion
  provenance.
- A missing prompt or lookup error leaves visible `@name` text and does not
  trust the browser fallback.
- Older clients can keep sending their compatibility block. The backend removes
  it and creates current context.
- The direct-message path gains one saved-prompt lookup when the message
  contains `@`.
- The eager created-session path carries the exact acceptance-time expansion
  through workflow composition and launch. It does not re-read direct saved
  prompts after the message is accepted.

## Alternatives Considered

1. **Trust the browser-generated definition.** Rejected because the definition
   can be stale or modified before submission.
2. **Expand only immediately before ACP dispatch.** Rejected because this makes
   stored message content differ from the prompt that the agent receives.
3. **Delay Quick Chat launch until the first message.** Rejected because eager
   launch provides commands, models, and modes to the composer.
4. **Replace visible `@name` with the full definition.** Rejected because this
   removes concise transcript text and exposes hidden instructions.
