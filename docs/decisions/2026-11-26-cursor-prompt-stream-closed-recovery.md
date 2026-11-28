# ADR-2026-11-26-cursor-prompt-stream-closed-recovery: Recognize Cursor prompt-stream loss from an exact error envelope

**Status:** accepted
**Date:** 2026-11-26
**Area:** backend, protocol

## Context

[Issue #5541](https://github.com/kdlbs/kandev/issues/5541) reports a Cursor
prompt that streamed output and then failed with `-32603 Internal error`.
Kandev kept the session and offered only manual recovery. A controlled
[reproduction](https://github.com/kdlbs/kandev/issues/5541#issuecomment-7197605216)
on Cursor CLI `2026.11.19-f005e9d` captured the terminal reply. Cursor's model
stream closed while a `postToolUse` hook was still running:

```json
{"error":{"code":-32603,"data":{"details":"WritableIterable is closed"},"message":"Internal error"},"id":5,"jsonrpc":"2.0"}
```

Cursor sent no ACP completion or stop signal for that prompt. A control run,
in which the hook finished before the stream closed, returned `end_turn`.

Criterion `AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.24` keeps every transient
signature carried only in `RequestError.Data` on manual recovery. This keeps
unreviewed provider detail out of classification and display. The generic
projection classifies on `Message`, and here that is only `Internal error`.

## Decision

The Cursor ACP dialect recognizes this one verified envelope by exact
structural match: a peer-returned `-32603`, the message `Internal error`, and
`data` equal to `{"details":"WritableIterable is closed"}` with no other
fields. The adapter projects a constant Kandev-authored diagnostic, classified
by the provider-scoped rule `cursor.prompt_stream_closed.v1` as
`agent_transport_lost`, and attaches the prompt's continuation evidence.
Recovery follows the existing retry-safety rules on the retained runtime. A
proven pre-result attempt with no assistant output or tool activity replays the
cached original prompt. Eligible output or successful foreground tool evidence
uses the existing interruption continuation contract.

Every other data-only envelope keeps criterion `.24`. Raw `data` never crosses
the provider-diagnostic boundary.

## Consequences

Eligible interrupted Cursor work with output or successful foreground tool
evidence continues with the internal `continue` instruction under the existing
five-attempt budget and cancellation controls. A proven pre-result attempt with
no assistant output or tool activity instead replays the cached original prompt
at most once per admitted attempt under that same budget. Background work,
subagents, unresolved permissions, and failed tools remain manual. ACP does not
distinguish a reply Cursor had already finished from one it never finished. In
the finished case, replay or continuation can cost one extra model request.
Kandev does not read Cursor's internal `turnEnded` log event. Other `details`
values need their own verified capture before they gain recovery.

## Alternatives Considered

- Keep manual recovery: safe but leaves the reported interruption unrecovered.
- Generic data-only classification: reopens the boundary `.24` closed for all
  providers without per-envelope evidence.
- Match any `details` containing "closed": unverified variants would gain
  automatic recovery.
- Treat the error as completion: work interrupted mid-turn would silently stop.

## Related contracts

- [Requirement `001.10`](../specs/platform/requirements/provider-interruption-continuation.md)
- [Provider error requirements](../specs/platform/requirements/provider-error-recovery.md)
- [Cursor design](../specs/platform/system-design/provider-error-recovery-cursor.md)
- [Plan](../plans/cursor-prompt-stream-closed/plan.md)
