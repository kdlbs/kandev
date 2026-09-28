---
status: draft
system: agents
requirements:
  - REQ-AGENTS-CODEX-USAGE-LIMIT-001
---

# Codex Usage-Limit Classification System Design

## Purpose and boundaries

The agent system owns provider error classification and manual recovery. This
design covers how a codex-acp usage-limit notice is classified, how its retry
time is derived, and how manual recovery recognizes the notice. It does not
change candidate ordering, the shared credential binding circuit, or the
classification rules of other providers.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-AGENTS-CODEX-USAGE-LIMIT-001` | [Classification](#classification), [Reset time](#reset-time), [Manual recovery](#manual-recovery) |

## Components and responsibilities

- **`internal/agent/runtime/routingerr` rules** own the per-provider regex
  table. The codex quota rule additionally matches the plain usage-limit notice
  with either apostrophe form.
- **`routingerr.Classify`** owns the final classification result. After the
  rule pass, it fills a missing reset hint for quota and rate errors from the
  notice text.
- **`routingerr.parseResetHint`** owns text parsing of the retry time. It is
  independent of any provider rule and returns no hint when the timestamp is
  invalid or has no explicit timezone.
- **The ACP adapter** owns correlation between a Codex usage-limit message and
  its active prompt. It projects the sanitized message onto that prompt's
  generic ACP error only after the notification queue is drained.
- **`internal/orchestrator` manual recovery** owns recognition of the failure
  that must retain a queued prompt. It normalizes the typographic apostrophe
  before matching.
- **`internal/agent/runtime/dynamic`** reuses the resulting quota
  classification unchanged. A quota failure opens the shared credential
  binding circuit, so every candidate on the same account is skipped as
  described by
  [dynamic-agent-routing-rollout-blockers](../../requirements/dynamic-agent-routing-rollout-blockers.md).

## Classification

Codex ACP emits the usage-limit notice as a plain agent message and the
terminal ACP error is only `Internal error`, so the provider rule must match
the notice text rather than only the machine tokens. The codex quota rule
accepts both `you've hit your usage limit` (U+0027) and `you’ve hit your usage
limit` (U+2019), alongside the existing machine tokens. A match yields a
high-confidence `quota_limited` classification whose fallback is allowed, which
is the signal dynamic routing needs to advance.

The ACP adapter marks the assistant chunk as provider diagnostic evidence. It
stores the sanitized notice on the active prompt turn. After `session/prompt`
returns, `sendPrompt` drains the ACP notification queue before it inspects that
evidence. It attaches the notice only when the same turn then returns the
generic JSON-RPC error with code `-32603` and message `Internal error`. The
adapter returns a typed `codex_acp` provider error and keeps the original ACP
error unwrap-able for allowlisted metadata extraction. Other terminal errors
remain unchanged.

The orchestrator classifies both the streaming diagnostic and the terminal
provider error with the provider ID. It clears the generated-output fence only
when the high-confidence diagnostic code matches the terminal error code and
the sanitized notice text is contained in the terminal provider message. This
lets dynamic routing apply its existing quota policy. The wrapped error text
also gives manual recovery the usage-limit notice, so a queued prompt remains
available to the user.

## Reset time

Some providers state the retry time only in the human notice, not in a
structured field. `Classify` derives a hint from the notice only when the
incoming `ResetHint` is nil and the classification is `quota_limited` or
`rate_limited`. The timestamp must include `UTC`, `GMT`, `Z`, or a numeric UTC
offset. The parser uses that explicit zone to create an absolute timestamp. It
does not use the backend's local timezone because it may differ from the
provider's timezone. The observed Codex notice has no timezone, so it yields no
text-derived hint and the existing routing policy applies.

A structured `ResetHint` supplied by the adapter always wins: text parsing runs
only when the field is absent. The parser rejects invalid month names,
nonexistent dates, invalid clock values, and offsets outside `±14:00`. For a
yearless date, it selects the next future occurrence in the stated timezone,
including the next year near New Year's Day. When parsing fails, no hint is
produced and the classification is unchanged.

## Manual recovery

Manual recovery inspects the prompt error text to decide whether a queued
prompt must be retained. Because codex renders the notice with a typographic
apostrophe, recovery normalizes U+2019 to U+0027 before matching, so both
`usageLimitExceeded` and `you've hit your usage limit` are recognized
regardless of the apostrophe form.

## Failure and recovery

Text parsing fails closed: an unparseable month, out-of-range day or clock
value, invalid calendar date, invalid offset, or absent explicit timezone
produces no hint rather than an incorrect one. Classification never depends on
a successfully parsed hint, and dynamic routing selection is unchanged when
no hint is available. Classification rules for other providers are untouched.
