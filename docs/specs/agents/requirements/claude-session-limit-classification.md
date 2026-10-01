---
status: draft
system: agents
created: 2026-10-01
owners:
  - kandev
---

# Claude Session-Limit Classification Requirements

## Overview

Claude ACP reports subscription exhaustion with a session-limit notice and a zoned reset clock.
The Agents system owns provider classification and timing hints consumed by recovery policies.
Recognizing this notice lets existing policies distinguish exhausted quota from an unknown runtime error.

## Terminology

- **Session-limit notice:** Claude's statement that the account has hit its session limit.
- **Reset clock:** A twelve-hour time after `resets`, with an explicit timezone in parentheses.
- **Reset hint:** An absolute timestamp available to existing recovery consumers.

## Requirements

### REQ-AGENTS-CLAUDE-SESSION-LIMIT-001: Recognize Claude session exhaustion

**Intent:** Claude session exhaustion must expose a quota classification and a trustworthy reset hint when the notice supplies one.

#### Acceptance criteria

- **AC-AGENTS-CLAUDE-SESSION-LIMIT-001.1:** Claude session-limit notices shall classify as high-confidence `quota_limited`, in the hard policy class, with fallback allowed.
  Straight and typographic apostrophes, `you have`, case differences, and whitespace differences shall be accepted.
- **AC-AGENTS-CLAUDE-SESSION-LIMIT-001.2:** Without a structured hint, a valid `resets <h>[:<mm>]<am|pm> (<zone>)` shall produce an absolute reset hint.
  Supported zones are UTC and slash-separated IANA location names.
  Abbreviations such as `GMT` and `EET` are not supported.
  Minutes omitted from the notice mean zero minutes.
- **AC-AGENTS-CLAUDE-SESSION-LIMIT-001.3:** An unambiguous reset clock shall resolve to its next occurrence strictly after the provider diagnostic's observation time in the stated zone.
  If no observation time is supplied, classification time shall be used.
  The backend's local timezone shall not affect the result.
  Calendar-day rollover shall respect the stated zone's offset changes.
- **AC-AGENTS-CLAUDE-SESSION-LIMIT-001.4:** Missing or unknown zones and invalid clocks shall produce no text-derived hint.
  A selected future wall time in a daylight-saving gap or repeated interval shall produce no hint.
  When today's requested wall time has elapsed, including a nonexistent gap time, selection shall advance to the next calendar date and validate that date's occurrence.
  An invalid hint shall not change a recognized notice's quota classification.
- **AC-AGENTS-CLAUDE-SESSION-LIMIT-001.5:** A supplied structured reset hint shall take precedence over notice text.
  Existing dated Codex reset formats and structured HTTP precedence shall retain their behavior.
- **AC-AGENTS-CLAUDE-SESSION-LIMIT-001.6:** Classification shall retain existing recovery policy and replay-safety boundaries.
  A fixed-profile Kanban quota failure shall remain manual recovery.
  An eligible dynamic route shall apply its saved hard-error policy and credential circuit rules.
- **AC-AGENTS-CLAUDE-SESSION-LIMIT-001.7:** Unrelated limit text and other providers shall not acquire Claude's session-limit classification.
  A rate-limit notice shall retain its rate classification.

## Out of scope

- Automatic resume or workflow deferral for fixed profiles after reset.
- Additional Claude limit-period signatures without observed evidence.
- Changes to rendered recovery cards, candidate order, or replay authorization.
- Reconstruction of timing text removed by upstream sanitization.

## Implementation plans

- [Claude session-limit fix](../../../plans/claude-session-limit-classification/plan.md).
