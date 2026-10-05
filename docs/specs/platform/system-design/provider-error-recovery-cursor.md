---
status: draft
system: platform
created: 2026-10-05
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
owners:
  - Kandev
---

# Cursor provider error recovery system design

## Purpose and boundaries

This is the Cursor-specific part of [provider error recovery](provider-error-recovery.md).
Platform owns normalized provider failure meaning and shared recovery permission.
The ACP dialect extracts ordered evidence; orchestration consumes semantic codes.
Native post-output recovery follows [continuation](provider-interruption-continuation.md).

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| `REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001` | Cursor normal-completion failure projection (`.12`-`.14`, `.30`, `.31`); Cursor retry-safety semantics (`.8`, `.15`) |

#### Cursor normal-completion failure projection

`cursor-agent` can report an upstream transient failure as an ordinary
`agent_message_chunk`; the ACP transport therefore owns a Cursor-specific evidence
projection that mirrors the existing Codex capacity projection. It does not add
generic content scanning to orchestration.

The observer runs in the ordered ACP notification worker and applies these
checks in order:

1. The adapter identity is `cursor-acp`.
2. The normalized event is a non-empty assistant message chunk for a non-zero
   prompt generation that matches the active turn.
3. After trimming leading and trailing whitespace, the chunk begins with the
   case-insensitive prefix `Error: RetriableError:`.
4. The text after that prefix contains a non-empty suffix of at most 256 bytes
   after Unicode whitespace trimming. It need not describe an HTTP/2 reset;
   `[unavailable] PING timed out` and `Connection stalled` are valid examples.
   Context cancellation, deadline, and retry escalation are vetoed.

Identity, event type, and prefix checks precede the suffix check. Both layers
share the case-insensitive prefix, Unicode trim, and byte bound. Prose before
the prefix, an empty suffix, cancellation signatures, stale generations, and
other adapters do not match.

A match stores bounded sanitized diagnostic evidence and its observed category
on the active `promptTurnState` under the existing evidence mutex. Keep the
occurrence time with that diagnostic. Suppress the control chunk; a later
non-empty assistant/thought chunk or new tool call clears its pending identity.
A later matching marker can re-arm it. In-flight tool updates do not clear it.
Do not store only a boolean and later reconstruct a transport failure.

After `session/prompt` returns, drain the ordered notification queue. If the
marker remains pending, retire the async completion owner and emit one
`EventTypeError`, with valid `ProviderError` source `cursor_acp`, provider ID,
occurrence time, and sanitized observed message. Use `SanitizeProviderMessage`
before retaining or exporting diagnostic text. Bound the source suffix to the
existing 256 UTF-8 bytes. If detail cannot survive sanitization, render a neutral
provider error instead of inventing a cause. Do not leak raw input into generic
logs, persistence, or the UI. Preserve complete-diagnostic identity only under
its existing exact-sanitization rules.

Use narrow catalogue rules for the actual category:

| Bounded Cursor terminal envelope | Semantic code | Short same-provider recovery |
| --- | --- | --- |
| `RetriableError: [resource_exhausted] Error` | `provider_resource_exhausted` | Yes; no reset hint or inferred account quota |
| `RetriableError: [unavailable] ...` | `provider_unavailable` | Yes |
| Exact observed HTTP/2 CANCEL stream reset, including leading `[canceled]` | `agent_transport_lost` | Yes |
| Explicit observed `Connection stalled` | `network_unavailable` | Yes |
| Unknown `RetriableError` suffix | Unclassified | Manual |
| Context cancellation, deadline, or retry escalation | Existing cancellation/manual policy | No |

The stream-reset rule keeps `cursor.retriable_stream_reset.v1` for actual reset
signatures, rather than rewriting historic records. Add independent stable
resource-exhaustion and availability fingerprints with fixtures. The resource
code is transient, high-confidence, same-provider retryable, and grants no new
fallback or reset permission. Include it in the shared short-retry policy and
runtime-usability recognition. Generic resource-exhaustion text without the
explicit Cursor retryable control envelope does not gain this policy.
Explicit hard quota/authentication evidence retains precedence. Unknown suffixes
can become a terminal diagnostic, but their prefix alone does not authorize
automatic recovery. Do not change the generic ACP `RequestError.Data` boundary.

The [hidden continuation decision](../../../decisions/2026-10-05-hidden-completed-tool-continuation.md)
corrects the former fixed HTTP/2 diagnostic and all-suffix transport classification.
The actual incident supplied resource exhaustion after completed tools; no
upstream TCP drop was established. The delivery package contains sanitized
fixtures, not the user's transcript or tool results.

#### Cursor retry-safety semantics

The Cursor label `RetriableError` is evidence about the upstream transport. It
is not permission for Kandev to repeat a turn. The following rules define the
Cursor recovery choices. They preserve the provider-neutral safety boundary in
[ADR-2026-08-08-provider-neutral-agent-error-recovery](../../../decisions/2026-08-08-provider-neutral-agent-error-recovery.md):

1. **Safe point.** A terminal marker is automatically replayable only when its
   prompt-generation-correlated evidence is known and records neither assistant
   output nor tool activity. Thoughts, message output, a pending or completed
   tool call, and missing evidence all fail closed. The observed incident had
   thoughts and a `Read File` call in flight, so it enters manual recovery even
   though its classification is transient.
2. **Replay mode.** Eligible retries retain the execution profile and native
   identity before sending the cached original prompt at the safe point above.
   Separate opt-in native continuation follows its distinct contract, admitting
   output or all-successful completed foreground tools while refusing uncertain
   work. Completion-based tool evidence uses a separate versioned contract. Otherwise
   the existing composer or manual recovery choices remain available according
   to the runtime-continuity contract.
3. **Cursor-owned retry.** Kandev does not schedule while the original
   `session/prompt` RPC remains open. Provider progress after a marker clears
   the pending marker. Only a later terminal marker can re-arm it. The prompt
   barrier then proves that Cursor's internal retry has either resumed or
   finished before Kandev chooses recovery. This prevents overlapping Cursor
   and Kandev retry loops.
4. **Budget and delay.** Eligible concrete-profile recovery reuses the single
   orchestrator-owned retry entry, `transientMaxAttempts`, and
   `transientRetryDelayFor`. There is no Cursor-specific nested counter, timer,
   or backoff. Exhaustion uses the existing manual recovery path.

The orchestrator records replay evidence for every interactive prompt, not only
dynamic route attempts. The evidence remains scoped by session, execution, and
prompt generation. Lifecycle snapshots the current prompt's evidence before it
marks a terminal completion as activity and carries that immutable snapshot on
`agent.failed`. This prevents separate NATS subscriptions for `agent.stream.*`
and `agent.failed` from changing the replay decision based on delivery order.
The concrete-profile `handleTransientFailure` path requires the same known,
no-output, no-tool condition before scheduling. A missing record is unsafe.
Dynamic profiles retain `dynamicPreResultSafe` and their configured policy
owner. A model-switch restart reserves the cached prompt and replay identity
before `StartAgentProcess` can dispatch the replacement prompt, then binds the
identity to the replacement execution. `agent_transport_lost` remains
same-provider recovery evidence and does not gain permission to switch
candidates merely because Cursor supplied the new fingerprint. No orchestration
branch inspects `cursor-acp`, `cursor_acp`, or the raw diagnostic.

## Delivery

[Hidden continuation and truthful Cursor errors](../../../plans/cursor-hidden-continuation/plan.md)
owns the correction and its fixtures. This split keeps the shared design within
its context limit; existing main-design anchors retain links to these sections.
