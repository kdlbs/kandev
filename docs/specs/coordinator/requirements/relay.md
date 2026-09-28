---
id: coordinator-relay
title: Answering in place and replying with a condition
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-29
---

# Answering in place and replying with a condition Requirements

## Overview

In phase 1 a question or permission item on Needs you only says "Answer it on
the task". Phase 3 lets a manager answer it on the item itself, through the
same clarification answer component and the same resolution paths the task
chat and the Inbox use, so the answer has one meaning wherever it is given.
Phase 3 also adds a third answer to a proposal besides approve and reject: a
reply with a condition, which settles the proposal and sends the condition to
the coordinator so it can propose again.

## Terminology

- **Clarification bundle:** all the questions of one `ask_user_question`
  call, sharing one pending id, answered or rejected as a whole. The
  [Needs-you Inbox requirements](../../ui/requirements/needs-you-inbox.md) and
  the integrations question-answering contract own its resolution semantics;
  this document consumes them unchanged.
- **Pending permission request:** a tool permission the task's agent is
  waiting on, with its title, action details and options.
- **Returned proposal:** a proposal settled by a reply with a condition. Its
  status is `returned`, which is settled and not open.
- Other terms are defined in the [system README](../README.md#terms).

## Mockup

The source mockup (`mockup-v2.1`, outside this repository) shows the question
card with numbered options, a free-text reply, "Send this answer" and a
Reassign action, and the proposal card with "Or reply with a condition". The
plan's ASCII previews UI-03 and UI-05 are the reference; Reassign is not
included.

## Requirements

### REQ-COORDINATOR-RELAY-001: Answering a question in place

**Intent:** A manager answers a waiting agent without leaving Needs you.

**User story:** As a workspace manager, I want to answer an agent's question
on the Needs you item, so that the task resumes without me opening it.

Mockup:

- Plan UI-03: the expanded question item.

#### Acceptance criteria

- **AC-COORDINATOR-RELAY-001.1:** While `features.coordinatorPhase3` is on, a
  question item whose task has an answerable clarification bundle shall offer
  **Answer here** to managers, in place of the phase 1 text "Answer it on the
  task."; **Open task** stays. This replaces
  `AC-COORDINATOR-NEEDS-YOU-002.5` for questions while the flag is on.
- **AC-COORDINATOR-RELAY-001.2:** **Answer here** shall expand the item to show
  the bundle's questions, options and shared context through the same
  clarification answer component the task chat and the Inbox render.
- **AC-COORDINATOR-RELAY-001.3:** Submitting shall resolve the bundle through
  the same resolver the Inbox uses and resume the task, with the outcomes that
  component reports: recorded (the item leaves Needs you when the task's
  pending action clears); lost to another caller (the item closes with a
  notice naming the winning outcome, no error); no longer active (the item
  closes with a notice); failed (the item stays expanded with the entered
  answer kept and Try again).
- **AC-COORDINATOR-RELAY-001.4:** When the task has no answerable bundle, or
  the bundle cannot be read, the item shall show the phase 1 text and
  **Open task** only.
- **AC-COORDINATOR-RELAY-001.5:** A reader shall see the question item without
  **Answer here**.

### REQ-COORDINATOR-RELAY-002: Answering a permission in place

**Intent:** A manager resolves a waiting permission the way the task chat
does.

Mockup:

- Plan UI-03: the expanded permission item.

#### Acceptance criteria

- **AC-COORDINATOR-RELAY-002.1:** While `features.coordinatorPhase3` is on, a
  permission item whose task's primary session has a pending permission
  request shall offer **Answer here** to managers; it replaces
  `AC-COORDINATOR-NEEDS-YOU-002.5` for permissions while the flag is on.
- **AC-COORDINATOR-RELAY-002.2:** **Answer here** shall show the request's
  title, action details and one button per option, and choosing one shall
  resolve the request through the same permission response path the task chat
  uses, recorded with source `web` and the manager as actor.
- **AC-COORDINATOR-RELAY-002.3:** When the request was already resolved or is
  no longer pending, the item shall close with a notice; any other failure
  shall keep the item expanded with Try again.
- **AC-COORDINATOR-RELAY-002.4:** Answering in place shall change neither the
  Inbox's rows nor its count; the Inbox keeps its own row set (D14).

### REQ-COORDINATOR-RELAY-003: Replying with a condition

**Intent:** A manager can say "yes, if" without editing the proposal
themselves.

**User story:** As a workspace manager, I want to reply to a proposal with a
condition, so that the coordinator revises it rather than me.

Mockup:

- Plan UI-05: the reply field on a proposal card.

#### Acceptance criteria

- **AC-COORDINATOR-RELAY-003.1:** While `features.coordinatorPhase3` is on, a
  `pending` or `failed` proposal card shall offer **Reply with a condition** to
  managers on both surfaces. Submitting 1 to 2,000 characters after trimming
  shall set the proposal `returned` with the reply text, the deciding user and
  the time, create nothing, and emit `coordinator.updated`. Empty or longer
  text shall be refused in place and change nothing.
- **AC-COORDINATOR-RELAY-003.2:** A reply shall follow the same status rules as
  reject: any status other than `pending` or `failed` returns 409 with the
  current proposal, and a reply racing an approve or reject leaves exactly one
  winner.
- **AC-COORDINATOR-RELAY-003.3:** After the proposal is `returned`, the system
  shall send the coordinator's conversation a message from the replying
  manager that quotes the proposal's title and the reply, opening the
  conversation first when there is none. When sending fails, the proposal
  shall stay `returned`, the card shall say "Reply saved, not delivered" with
  **Send again**, and Send again shall send the same message without changing
  the proposal.
- **AC-COORDINATOR-RELAY-003.4:** `propose_task_kandev` shall accept an
  optional `in_reply_to` naming a `returned` proposal of the same coordinator;
  any other value shall be refused naming the field. A proposal with
  `in_reply_to` shall show "Revised after your reply" with the reply text.
- **AC-COORDINATOR-RELAY-003.5:** A coordinator principal shall be refused the
  reply route, as for approve and reject.

## Out of scope

- Reassigning a question to another person; clarifications have no addressee.
- Recording who answered a clarification. The clarification contract stores
  no answerer today; changing it is a change to that contract, not to this
  system. Permissions keep their existing resolution audit.
- Answering from the copilot chat, and any answer tool on the coordinator's
  surface (D5): the coordinator never answers for a person.
- Re-asking a question.
