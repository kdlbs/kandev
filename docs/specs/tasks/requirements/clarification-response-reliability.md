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

Unsubmitted answers also remain recoverable during navigation within the same
browser page. The [system design](../system-design/clarification-response-reliability.md)
defines their lifecycle separately from an accepted response.

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

### REQ-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-002: Unsubmitted answer continuity

**Intent:** Users can inspect other tasks and return to an unfinished question
without entering their answers again.

#### Acceptance criteria

- **AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-002.1:** On desktop and phone,
  task or session navigation shall preserve selected options, custom text,
  and the current question for a retained pending bundle. This includes text
  that the user has not committed with Enter or Send.
- **AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-002.2:** A restored draft shall
  belong to the same task, source session, pending bundle, and form mode.
  Another bundle shall start without those answers, including bundles that
  reuse question IDs.
- **AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-002.3:** Navigation and draft
  restoration shall neither submit nor reject a bundle. Existing single-question
  selection and explicit multi-question Submit behavior shall remain unchanged.
- **AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-002.4:** An accepted answer,
  accepted Skip, or authoritative terminal outcome shall prevent restoration
  of that pending draft. A retryable response failure shall preserve its answers.
- **AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-002.5:** Drafts shall remain
  available during client navigation, including a task-page unmount, within
  one browser page. Reload, logout, or a change of authenticated identity or
  authority shall clear them. Returning to an earlier identity shall not
  restore its cleared drafts, including changes while no task page is open.
- **AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-002.6:** The browser shall retain
  at most eight pending-bundle drafts per application instance. Access or edit
  shall refresh recency. A ninth draft shall evict the least recently used
  draft without changing the remaining drafts.
- **AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-002.7:** A desktop/phone layout
  change shall preserve the same draft and question position. Answer retention
  shall not reset unrelated task panels or their pending focus state.

## Out of scope

- Changing current-turn authority, detached-resume semantics, or the durable
  at-most-once boundary defined by the active clarification lifecycle.
- Introducing a dedicated clarification table or rewriting existing message
  rows.
- Supporting more than one Kandev backend replica against PostgreSQL.
- Making local dismiss resolve or reject a clarification. Skip remains the
  explicit rejection action.
- Preserving unsubmitted answers across reloads, browser tabs, devices, or
  backend installations.
- Extending historical late-answer admission, Quick Chat, or Inbox draft
  retention beyond their existing contracts.
