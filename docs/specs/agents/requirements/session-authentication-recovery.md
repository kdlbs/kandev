---
status: draft
system: agents
created: 2026-11-12
owners:
  - Kandev
---

# Session authentication recovery requirements

## Overview

Users need to identify provider authentication failures before they retry an agent session.
The agent system owns provider authentication evidence and recovery eligibility.
Tasks retain durable error storage and session ownership.

The existing [recovery presentation contract](session-recovery-failures.md) remains authoritative for placement, history, and attempt isolation.
This capability defines the missing authentication outcome, including failures before a native conversation exists.
[Remote authentication admission](remote-auth-admission.md) concerns configuration eligibility before launch and remains separate.

## Terminology

- **Established authentication failure:** A provider authentication rejection identified at its supported protocol boundary.
- **Usable login method:** Supported login guidance that remains safe and executable in the current execution context.
- **Native conversation:** The conversation identity returned by the agent provider, distinct from the Kandev task session.

## Requirements

### REQ-AGENTS-SESSION-AUTHENTICATION-001: Actionable provider authentication recovery

**Intent:** Preserve an established authentication cause and show useful login guidance throughout session recovery.

#### Acceptance criteria

- **AC-AGENTS-SESSION-AUTHENTICATION-001.1:** When a provider rejects session creation or supported restoration because authentication is required, Kandev shall retain that authentication cause.
  Safe error formatting shall not replace the established cause with an unknown failure.
- **AC-AGENTS-SESSION-AUTHENTICATION-001.2:** Recovery shall identify the configured provider and explain that login is required before retry.
  Usable login methods shall remain visible. Missing, empty, unsafe, or unavailable methods shall retain generic authentication guidance.
  A terminal command may launch only from a terminal validated for the same executor environment and live owner.
  When that context is unavailable, the command shall remain copyable with execution-host instructions and no runnable login control.
- **AC-AGENTS-SESSION-AUTHENTICATION-001.3:** The failed startup entry, current recovery card, technical cause, and transcript shall agree on authentication.
  Reload, navigation, reconnect, and backend restart shall preserve that cause without the original error object or a live failed execution.
- **AC-AGENTS-SESSION-AUTHENTICATION-001.4:** An authentication failure shall remain recoverable through an explicit retry after user-driven authentication.
  Retry shall reuse the prepared workspace. Supported restoration shall preserve an existing native conversation until the user explicitly chooses a reset.
  A failed creation with no native conversation shall offer restart without implying that a conversation can be resumed.
  A startup authentication failure remains eligible for explicit restart when its session is waiting for input and has no native conversation identity.
  A retained native conversation identity from the current session shall preserve Resume when transcript metadata is absent.
  Unknown identity eligibility shall not default to a context-resetting restart. A current session snapshot with no native identity, or an explicit negative resume-token projection when that snapshot is unavailable, confirms that Resume is unavailable. Positive native identity or resume-token evidence takes precedence.
  Request admission alone shall not establish successful login or recovery.
- **AC-AGENTS-SESSION-AUTHENTICATION-001.5:** Unrelated repository, MCP setup, permission, transport, cancellation, timeout, and unknown failures shall retain their classifications.
  A provider authentication rejection before native session creation shall not produce MCP attachment failure evidence for that rejected session operation.
  Stale or foreign attempts shall not change the current session.
  New records, displays, copies, and diagnostic fields shall exclude secrets, private paths, and arbitrary nested provider payloads.
- **AC-AGENTS-SESSION-AUTHENTICATION-001.6:** Desktop and phone shall expose the same cause, login guidance, eligible retry, pending feedback, and safe details.
  The active recovery card shall remain the single control owner.
  Phone and coarse-pointer controls shall have touch targets of at least 44 pixels without horizontal document overflow.
  New copy shall use all shipped locales and update with the selected locale.

## Compatibility and exclusions

Older unknown records retain their safe fallback; Kandev shall not invent authentication history from generic text.
This capability does not diagnose why a credential failed or claim that a token expired.
It does not introduce automatic login, credential copying, provider switching, new retry budgets, or weaker delivery ownership checks.
Repository and MCP credentials remain separate from provider account authentication.

## Design and delivery

- [Session authentication recovery design](../system-design/session-authentication-recovery.md)
- [Implementation package](../../../plans/cursor-startup-authentication-recovery/plan.md)
