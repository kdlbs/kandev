---
status: draft
system: ui
created: 2026-09-02
owners:
  - kandev
---

# Session Start Composer Readiness Requirements

## Overview

The chat composer has separate editing and submission readiness. An operator
can prepare a draft while a session starts or resumes. The operator cannot
submit the draft before the session can accept it.

The UI system owns this interaction contract. The task system continues to own
the session lifecycle and message admission rules.

## Terminology

- **Session startup:** A new or resumed session is starting, or its executor
  environment is in the prepare phase.
- **Editable:** The composer accepts focus and draft text.
- **Submittable:** The composer can send its current content to the session.

## Requirements

### REQ-UI-SESSION-START-COMPOSER-READINESS-001: Early draft entry

**Intent:** Operators can use session startup time to prepare the next message
without creating an invalid message request.

**User story:** As an operator, I want to type during session startup, so that
I can prepare my message before the session is ready.

#### Acceptance criteria

- **AC-UI-SESSION-START-COMPOSER-READINESS-001.1:** When a visible chat session
  starts or resumes, the composer shall accept focus and draft text during the
  startup state.
- **AC-UI-SESSION-START-COMPOSER-READINESS-001.2:** When regular message
  submission is unsafe during startup, the send action and submission shortcut
  shall remain disabled. These actions shall not create a message request.
- **AC-UI-SESSION-START-COMPOSER-READINESS-001.3:** When the session becomes
  ready, the composer shall keep the draft. The composer shall enable
  submission without requiring the operator to enter the text again.
- **AC-UI-SESSION-START-COMPOSER-READINESS-001.4:** If startup fails or enters
  recovery, the composer shall keep the draft and shall apply the existing
  failure or recovery gate.
- **AC-UI-SESSION-START-COMPOSER-READINESS-001.5:** An interactive
  clarification shall keep its existing submission behavior when lifecycle
  data still reports startup.
- **AC-UI-SESSION-START-COMPOSER-READINESS-001.6:** Task chat and Quick Chat
  shall provide the same editing and submission behavior on desktop and mobile
  surfaces.

## Out of scope

- Changing session lifecycle states or message admission rules.
- Queuing a regular message before the session is ready.
- Changing failure, recovery, movement, upload, or executor-availability gates.
- Changing composer layout, touch targets, copy, or responsive composition.
