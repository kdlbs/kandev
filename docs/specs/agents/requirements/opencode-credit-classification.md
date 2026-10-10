---
status: active
system: agents
created: 2026-09-30
owners:
  - kandev
---

# OpenCode Credit-Exhaustion Classification Requirements

## Overview

OpenCode can report a depleted credit allowance as plain diagnostic text. The
agent system owns provider-error classification. It must recognize exhausted
credit allowances so dynamic routing can try another configured provider.

Payment errors also arrive as text without an HTTP status. These errors require
user action when the text does not show credit exhaustion.

## Terminology

- **Credit-exhaustion diagnostic:** OpenCode text that states a credit limit
  has been reached, credits are exhausted, or the balance is insufficient.
- **Payment diagnostic:** OpenCode text that states payment is required but
  does not state that a credit allowance is exhausted.

## Requirements

### REQ-AGENTS-OPENCODE-CREDIT-001: Classify OpenCode billing diagnostics

**Intent:** Provider routing must distinguish an exhausted credit allowance
from a billing condition that requires user action.

**User story:** As an operator, I want an exhausted credit route skipped and a
billing problem surfaced, so that other configured routes can handle safe work
and users can repair inactive billing.

#### Acceptance criteria

- **AC-AGENTS-OPENCODE-CREDIT-001.1:** When `opencode-acp` reports one of the
  bounded credit-exhaustion phrases, the system shall classify it as
  high-confidence `quota_limited` and allow fallback.
- **AC-AGENTS-OPENCODE-CREDIT-001.2:** When `opencode-acp` reports only
  `payment required`, the system shall classify it as high-confidence
  `subscription_required`, set user action, and disable automatic retry.
- **AC-AGENTS-OPENCODE-CREDIT-001.3:** When the text reports credit exhaustion
  and payment requirement, the system shall give credit-exhaustion text priority.
- **AC-AGENTS-OPENCODE-CREDIT-001.4:** When structured HTTP status and text
  disagree, the system shall give the structured status priority.
- **AC-AGENTS-OPENCODE-CREDIT-001.5:** Text that only mentions credits without
  an exhaustion phrase shall keep its existing classification.
- **AC-AGENTS-OPENCODE-CREDIT-001.6:** Credit classification shall not depend
  on a renewal URL or date that sanitization removes.

## Out of scope

- Changing rules for providers other than `opencode-acp`.
- Changing candidate ordering, credential circuits, or provider selection.
- Parsing reset times from these billing diagnostics.
