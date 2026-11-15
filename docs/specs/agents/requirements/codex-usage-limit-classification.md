---
status: draft
system: agents
created: 2026-09-28
updated: 2026-11-10
owners:
  - kandev
---

# Codex Usage-Limit Classification Requirements

## Overview

Codex ACP reports an exhausted account usage limit as a plain agent message and
returns only a generic terminal error from `session/prompt`. The agent system
owns provider error classification and manual recovery, so it must recognize
that notice: otherwise dynamic routing stops instead of advancing to a healthy
provider, and manual recovery discards a queued prompt that should be retained.

## Terminology

- **Usage-limit notice:** The provider-authored plain-text message stating that
  the account has hit its usage limit, including a retry time when present.
- **Credit-exhaustion notice:** Codex's explicit statement that the workspace is out of credits.
- **Structured reset hint:** A retry timestamp parsed from a provider-native
  field rather than from free text.
- **Explicit timezone:** `UTC`, `GMT`, `Z`, or a numeric UTC offset in
  `±HHMM` or `±HH:MM` form. A backend's local timezone is not a provider hint.

## Requirements

### REQ-AGENTS-CODEX-USAGE-LIMIT-001: Recognize codex usage-limit notices

**Intent:** An exhausted codex account must yield a fallback-eligible quota
classification that carries the provider's retry time and is recognized during
manual recovery, so routing advances and the user is not forced to retry
against the same exhausted account.

#### Acceptance criteria

- **AC-AGENTS-CODEX-USAGE-LIMIT-001.1:** A codex-acp usage-limit notice written
  with either a straight or a typographic apostrophe shall classify as
  `quota_limited` with high confidence and shall allow fallback.
- **AC-AGENTS-CODEX-USAGE-LIMIT-001.2:** A structured reset hint shall take
  precedence over text parsing. Without a structured hint, classification shall
  derive a retry time only from a valid notice timestamp with an explicit
  timezone. An unzoned timestamp shall produce no text-derived hint.
- **AC-AGENTS-CODEX-USAGE-LIMIT-001.3:** Manual recovery shall recognize a
  usage-limit notice written with a typographic apostrophe and retain the
  queued prompt instead of discarding it as an unrelated failure.
- **AC-AGENTS-CODEX-USAGE-LIMIT-001.4:** Text parsing shall reject invalid
  calendar dates, invalid clock values, and invalid UTC offsets. A valid
  yearless timestamp shall resolve to its next future occurrence in its stated
  timezone.
- **AC-AGENTS-CODEX-USAGE-LIMIT-001.5:** A Codex diagnostic line that opens with `Your workspace is out of credits` shall classify as high-confidence hard `quota_limited`.
  The classification shall allow fallback and retain existing quota flags.
  Matching shall accept case differences, horizontal whitespace differences, and omission of `Your`.
  Established `AI_APICallError:`, `Error:`, `Internal error:`, and `API Error:` labels can precede the notice.
  Quoted prose, fenced examples, split-line phrases, unrelated providers, and incomplete phrases shall not match this addition.
  A notice without a reset time shall provide no inferred deadline.
  Explicit credit exhaustion shall outrank HTTP 429 metadata, while other structured statuses retain precedence.
- **AC-AGENTS-CODEX-USAGE-LIMIT-001.6:** A credit-exhaustion notice from the active ACP prompt shall remain available on that prompt's matching generic terminal failure.
  The terminal failure must have JSON-RPC code `-32603` and the exact message `Internal error`.
  A notice from another prompt, cancellation, successful completion, or a different terminal error shall not authorize this substitution.
  Without a matching notice or other recognized evidence, a bare `Internal error` shall retain its existing classification.
  Text available only in process logs shall not establish prompt correlation.
  Existing replay, continuation, ownership, and generation guards shall remain authoritative.
- **AC-AGENTS-CODEX-USAGE-LIMIT-001.7:** The new quota classification shall use existing configured routing and shared credential circuits.
  A quota failure handled by Office routing shall not increase the agent's consecutive-failure count or cause an automatic pause.
  A healthy candidate on another credential binding shall remain eligible under the configured route policy.
  A provider deadline shall enable the existing health check or circuit probe, rather than assert successful recovery.

## Out of scope

- Changing classification rules for providers other than codex-acp.
- Changing the shared credential binding circuit or candidate selection order.
- Inferring quota or provider availability from a generic JSON-RPC internal error without supporting evidence.
- Parsing uncorrelated process logs or changing manual recovery's queued-prompt rules.

## Implementation plans

- [Codex usage-limit classification](../../../plans/codex-usage-limit-classification/plan.md).
- [Office mid-run quota notices](../../../plans/office-midrun-quota-classification/plan.md) extends the notice catalogue for issue #5222.
