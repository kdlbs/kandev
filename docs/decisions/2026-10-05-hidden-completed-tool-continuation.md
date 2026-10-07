# ADR-2026-10-05-hidden-completed-tool-continuation: Continue completed work with a hidden prompt

**Status:** accepted
**Date:** 2026-10-05
**Area:** backend, frontend, protocol

## Context

An inspected Cursor session reported `RetriableError: [resource_exhausted] Error`
after its foreground PR-read shell commands reported completion. The ACP adapter
replaced the diagnostic with a fixed HTTP/2 CANCEL message. The runtime remained
usable, but original-prompt replay was refused and experimental continuation was
disabled. The enabled continuation contract would also refuse every shell tool.

The user requested a generic internal `continue` message, absent from chat history,
using the existing exponential retry mechanism. They identified same-conversation
continuation as the desired recovery after tools have already completed.

The [October 2 decision](2026-10-02-safe-interrupted-conversation-continuation.md)
separates original replay from continuation but limits continuation to read-only
work. This decision amends that admission boundary and prompt wording. It keeps
the native identity, ownership, cancellation, and original-replay constraints.

## Decision

After a supported short-retryable provider failure settles, an enabled concrete
task conversation may receive the exact internal prompt `continue` when every
observed foreground tool has reported unambiguous successful completion. Completed
shell, write, and MCP tools do not by themselves forbid this new turn. Pending,
failed, cancelled, conflicting, or incomplete tool outcomes, unresolved permissions,
active background/subagent work, and uncertain prompt acceptance remain manual.

Keep the same provider-native conversation, selected settings, and workspace.
Preserve a proven usable runtime; restore that exact native identity only when
runtime loss requires it. Never replay the original request after tool activity,
append original attachments, or silently create another conversation.

Use the existing dispatch-only internal continuation path and single retry owner.
Do not create or filter a user-message row. Keep attempts and recovery status
visible, and keep actual user-authored `continue` messages visible. Reuse the
five-attempt 5/10/20/40/60-second schedule and experimental default-off toggle.

Retain a sanitized actual provider condition. A retryable resource-exhaustion
envelope receives its own semantic category, not a fabricated transport cause or
an inferred account quota. Unknown conditions do not authorize automatic recovery.

Version the completed-tool evidence contract so an older read-only snapshot cannot
authorize the broader behavior and older consumers refuse unknown support.

## Consequences

Completed shell workflows can recover without a user manually typing `continue`.
The recovery prompt is independent of the original task's subject. The transcript
stays focused on user requests, actual work, and visible recovery status.

Successful tool completion is an admission signal, not an exactly-once guarantee.
A newly prompted model may choose to inspect or repeat an action; Kandev promises
that its dispatcher does not resend the original request or completed tool calls.
This is an intentional broader continuation policy, not a provider idempotency
claim. Native compatibility proof and controlled effectful-tool regressions are
required before declaring the versioned support ready.

The error correction is unconditional; automatic continuation still requires the
existing operator opt-in and restart. Promotion is a separate release decision.
Completed historical plans retain their results and are not relabelled as evidence
for the new completed-tool contract.

## Alternatives considered

- Task-specific continuation instructions: couple recovery to request content and
  are unnecessary for an intact native conversation.
- Continue only after read-only tools: rejects already completed shell workflows.
- Resend the original request: permits original work to be repeated by the host.
- Hide messages by matching the word `continue`: incorrectly hides real user input
  and exposes internal instructions through reload or alternate projections.
- Treat every `RetriableError` as transport loss: destroys diagnostic meaning.
- Add another timer or persistent retry queue: duplicates existing ownership.

## Related contracts

- [Continuation requirements](../specs/platform/requirements/provider-interruption-continuation.md)
- [Continuation design](../specs/platform/system-design/provider-interruption-continuation.md)
- [Provider error requirements](../specs/platform/requirements/provider-error-recovery.md)
- [Delivery plan](../plans/cursor-hidden-continuation/plan.md)
