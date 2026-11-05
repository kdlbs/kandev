# ADR-2026-10-29-acp-prompt-error-runtime-lifetime: Preserve usable ACP sessions after prompt errors

**Status:** accepted
**Date:** 2026-10-29
**Area:** backend, protocol, frontend

## Context

An OpenCode 2.2.0 turn failed after a SQLite lock timeout. Kandev stopped the runtime through its general execution-failure path.
The user requested follow-up prompts after errors across ACP agents, whenever the session remains usable.

The [existing decision](2026-10-03-transient-turn-runtime-lifetime.md) separates transient turn failure from process lifetime.
Its error and provider allowlists still send generic internal errors through teardown.
These allowlists couple runtime preservation to automatic recovery classification.

The pinned ACP SDK also creates `RequestError` values for local send, decode, cancellation, and disconnect failures.
The error type alone cannot prove that the agent returned a terminal prompt response.

## Decision

Preserve an initialized, connected ACP conversation after its current prompt returns a proven error response.
Apply this rule across providers in concrete-profile interactive task sessions, including task-backed Quick Chat.
Fail and explain the turn. After durable settlement, allow later user input on the same runtime.
Keep task attention until explicit dismissal or newly accepted manual input acknowledges its client-observed same-session stamp.
Backend admission cannot invent observation. Previously queued input cannot acknowledge a later failure through automatic drain.
Quick Chat provides inline attention and Dismiss without adding normal sidebar rows or task menus.
Focus and read state do not acknowledge turn errors. Acknowledgement preserves transcript history and cannot clear a newer failure.

Classify errors only for diagnostic and automatic recovery policy. Classification does not decide manual runtime preservation.
Keep provider-specific terminal-stream evidence under its existing tested contracts.

Add typed response-origin evidence in the existing ACP SDK fork at its response-correlation boundary.
Kandev consumes that evidence with current prompt ownership, notification-drain, cancellation, and runtime checks.
Do not infer origin from error text, raw data, debug logs, or a probe prompt.

Real runtime loss, failed initialization, explicit lifecycle operations, and uncertain settlement retain their existing authority.
Autonomous owners and non-ACP transports retain their existing policies.
This decision extends the October 3 decision without changing retry safety or budgets.

## Consequences

Generic prompt failures can leave one historical error, a task attention badge, and a usable composer.
Task-owned attention remains separate from session recovery and runtime authority.
Its latest error pointer keeps acknowledgement bounded without reading every session transcript.
Quota and authentication errors can still require external action before another prompt succeeds.
Retained processes continue to occupy runtime resources.

The SDK prerequisite needs a reviewed immutable revision before the host removes its application-error allowlists.
Older agentctl components retain conservative behavior when they do not supply the required attestation.
Native compatibility tests supplement shared protocol tests. Preserving a conversation does not guarantee that every later prompt succeeds.

## Alternatives considered

- Keep a growing error allowlist. This leaves valid new errors dependent on provider-specific additions.
- Preserve every `RequestError`. This can treat local transport or decode failures as settled remote errors.
- Add a second protocol parser in Kandev. This duplicates SDK request correlation and notification ordering.
- Stop and restore every failure. This discards a usable connection and makes follow-up input require recovery.
- Retry every failed prompt. This can repeat completed effects and changes automatic recovery policy.

## Related contract

- [Requirements](../specs/platform/requirements/transient-turn-runtime-continuity.md)
- [Prompt-error design](../specs/platform/system-design/acp-prompt-error-continuity.md)
- [Implementation package](../plans/acp-prompt-error-continuity/plan.md)
