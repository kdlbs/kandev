---
status: active
system: tasks
created: 2026-09-05
owners:
  - kandev
---

# Clarification response reliability Requirements

## Overview

Users must receive a bounded, unambiguous result when they answer or skip an
active clarification. The response path must remain responsive as unrelated
task-session history grows, and a transient failure must leave the user's
choices recoverable.

The task system owns this contract because it owns clarification authority,
the durable response claim, and the response endpoint. The UI presents the
result on desktop and phone but does not decide whether a bundle was accepted.

## Terminology

- **Pre-claim work:** The durable bundle lookup, authority check, validation,
  and atomic transition that occur before response delivery begins.
- **Retryable failure:** A response attempt that did not establish a durable
  accepted outcome and can be attempted again without duplicate delivery.
- **Ambiguous transport result:** A client-side timeout or connection failure
  where the client did not receive the server's authoritative result.

## Requirements

### REQ-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001: Bounded clarification responses

**Intent:** A clarification action must not leave the user watching an
indefinite progress state or force them to re-enter an answer after a transient
failure.

**User story:** As a user answering or skipping an agent question, I want a
bounded and retryable result, so that I can recover without guessing whether my
response reached the agent.

#### Acceptance criteria

- **AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001.1:** When a user submits or
  skips an active clarification, the client shall leave the submitting state
  within 40 seconds. It shall show the accepted outcome, the expired outcome,
  or a retryable error.
- **AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001.2:** When pre-claim work
  cannot complete within five seconds, the endpoint shall return HTTP 503 with
  machine code `temporarily_unavailable`. It shall not deliver the response.
  It shall leave the still-current bundle answerable.
- **AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001.3:** When a response attempt
  has an ambiguous transport result, retrying the same answer or rejection
  shall return the established outcome or perform one safe delivery. It shall
  not deliver the response twice.
- **AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001.4:** When a response attempt
  fails retryably on desktop or phone, the interface shall preserve the user's
  selected answers and stop the progress indicator. It shall expose Retry and
  restore the existing local dismiss and Skip actions.
- **AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001.5:** On supported SQLite and
  PostgreSQL installations, pending-ID response lookup and claim shall use a
  pending-ID-leading index after fresh initialization or an existing-database
  startup. Unrelated message history shall not require a full message-table
  scan.

## Out of scope

- Changing current-turn authority, detached-resume semantics, or the durable
  at-most-once boundary defined by the active clarification lifecycle.
- Introducing a dedicated clarification table or rewriting existing message
  rows.
- Supporting more than one Kandev backend replica against PostgreSQL.
- Making local dismiss resolve or reject a clarification. Skip remains the
  explicit rejection action.
