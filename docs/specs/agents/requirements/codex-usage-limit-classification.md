---
status: draft
system: agents
created: 2026-09-28
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

## Out of scope

- Changing classification rules for providers other than codex-acp.
- Changing the shared credential binding circuit or candidate selection order.
