---
status: draft
system: agents
requirements:
  - REQ-AGENTS-CLAUDE-SESSION-LIMIT-001
---

# Claude Session-Limit Classification System Design

## Purpose and boundaries

Agents owns this provider signature and the shared reset parser.
Platform recovery and dynamic routing consume the existing classification contract.
This design adds no recovery owner, event field, or orchestration branch.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-AGENTS-CLAUDE-SESSION-LIMIT-001` | [Classification](#classification), [Reset clock](#reset-clock), [Consumer integration](#consumer-integration), [Compatibility](#compatibility) |

## Components and responsibilities

- `internal/agent/runtime/routingerr` owns provider rules, `Classify`, reset parsing, and error invariants.
- `internal/agentctl/server/adapter/transport/acp` projects terminal ACP errors through `ProviderErrorFromError`.
- `internal/orchestrator.classifyKanbanFailure` passes the agent identity, projected message, and structured reset hint to the classifier.
- `internal/agent/runtime/dynamic.Engine` applies saved policies and shared credential circuits.

## Classification

Add the provider-scoped rule `claude.stderr.session_limit.v1` before the existing Claude rate and subscription rules.
Its bounded expression is `(?i)\b(?:you['’]ve|you\s+have)\s+hit\s+your\s+session\s+limit\b`.
It yields `CodeQuotaLimited` and `ConfHigh`.
Existing invariants assign `ClassHard`, enable fallback and the classification's retry flag, and leave user action false.
The retry flag alone does not authorize replay.

Keep the period word `session` explicit.
A generic period-word expression also matches `you've hit your rate limit`, incorrectly changing a transient throttle to a hard quota error.
Other period signatures require captured evidence before expansion.
Structured HTTP status continues to run before provider rules.

## Reset clock

Extend `parseResetHintAt(text, now)` with a separate clock-only parsing branch.
Keep the existing dated `try again at` branch and its year-resolution behavior.
Do not broaden the existing offset parser to load arbitrary zone strings.

The new branch accepts `resets 11:10am (Europe/Helsinki)` and `resets 11am (Europe/Helsinki)`.
It requires an AM/PM suffix, validates hours 1 through 12 and minutes 0 through 59, and requires balanced zone parentheses.
Preserve timezone-name case when calling `time.LoadLocation`.
Accept UTC and IANA names, including names with multiple slash-separated components.
Reject `Local`, abbreviations such as `EET`, unknown names, absolute paths, and malformed names.
Use the standard `time/tzdata` fallback so packaged binaries do not depend on host timezone files.

Convert `now` into the explicit location before selecting the calendar date.
Use calendar dates rather than adding twenty-four hours for rollover.
Select today's requested wall time when it is future, otherwise select the next calendar day's wall time.
At exact equality, select the next day.
Validate the selected wall time and reject daylight-saving gaps or repeated wall times instead of choosing an arbitrary offset.
Compare the resulting instant with `now` before returning it.

For example, at `2026-10-01T08:03:00Z`, Helsinki's `11:10am` resolves to `2026-10-01T08:10:00Z`.
At `2026-10-01T08:11:00Z`, it resolves to `2026-10-02T08:10:00Z`.
The parser receives a clock argument for deterministic tests.
No new public clock API is needed.

`Classify` already parses text only for quota or rate errors with no structured `ResetHint`.
It passes input text to the parser, while rules consume sanitized text.
Retain this separation and do not persist raw text.

## Consumer integration

Claude's reported terminal error already contains the complete notice.
Unlike the Codex message-only failure, it does not require attaching a separate assistant chunk to `Internal error`.
Verify the existing terminal projection and `classifyKanbanFailure` with the exact issue message.

Dynamic routing consumes the hard quota classification through its existing saved policy.
`Engine.openCircuitForFailure` sets the shared binding deadline to the later of its minimum backoff and the reset hint.
Sibling candidates on that binding remain ineligible while a distinct healthy binding can be selected.
At the deadline, the existing exclusive-probe path admits one candidate; the circuit closes only after that probe succeeds.
Invocation correlation and output/tool safety remain prerequisites for automatic recovery.

`routingerr.Decide(ContextKanban, ...)` keeps fixed-profile quota failures in manual recovery.
The specialized rendered quota metadata currently serves OpenCode diagnostics.
This package does not expand that card or promise an automatic wake at Claude's reset time.

## Compatibility

| Input boundary | Behavior | Unsupported-shape fallback |
| --- | --- | --- |
| Claude terminal notice, Helsinki zone | High-confidence quota with reset hint | Quota without hint if clock is invalid |
| Claude notice after existing ACP sanitization | Same result when the explicit zone survives | Quota without hint if sanitization removed the zone |
| Raw clock with a nested IANA name | Shared parser loads the complete zone name | No hint for an unknown name |
| Codex dated notice | Existing date and explicit-offset parser | Existing no-hint behavior for an unzoned date |
| Structured HTTP status or reset hint | Existing structured precedence | No text override |
| Other provider, same session prose | Existing provider classification | Unknown post-start error unless another existing rule matches |

The existing path sanitizer removes part of `America/Argentina/Buenos_Aires` in projected messages.
Direct parser support does not imply that this name survives every adapter boundary.
Retain redaction guarantees and fail closed when only the sanitized text remains.

## Observability and verification

Existing lifecycle logs expose the semantic code, confidence, rule ID, and recovery flags.
The new rule ID distinguishes this notice without adding metric labels or logging raw diagnostics.
Classifier, fixed-clock parser, ACP projection, orchestrator, and dynamic-engine regressions cover the contract.
No browser test is required because no rendered component changes.

## Related decisions and contracts

- [Provider-neutral error recovery](../../../decisions/2026-08-08-provider-neutral-agent-error-recovery.md).
- [Provider classes and saved policies](../../../decisions/2026-08-17-provider-error-classes-and-policies.md).
- [Platform recovery requirements](../../platform/requirements/provider-error-recovery.md).
- [Codex reset parser design](codex-usage-limit-classification.md).
- [Fix package](../../../plans/claude-session-limit-classification/plan.md).
