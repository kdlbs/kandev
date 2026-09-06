---
status: draft
system: integrations
created: 2026-08-16
owners:
  - nova28
---
# External Question Answering — authorized, discoverable, idempotent clarification resolution Requirements

## Overview

When a Kandev agent calls `ask_user_question_kandev`, the resulting question is answerable in exactly one place: a popup rendered over the chat input of that task's session. Everything behind that popup — the durable record, the REST endpoints, the resume path — already exists, but nothing outside the browser can discover a question or answer it.

## Requirements

### REQ-INTEGRATIONS-EXTERNAL-QUESTION-ANSWERING-001: External Question Answering — authorized, discoverable, idempotent clarification resolution

**Intent:** When a Kandev agent calls `ask_user_question_kandev`, the resulting question is answerable in exactly one place: a popup rendered over the chat input of that task's session. Everything behind that popup — the durable record, the REST endpoints, the resume path — already exists, but nothing outside the browser can discover a question or answer it.

#### Acceptance criteria

- **AC-INTEGRATIONS-EXTERNAL-QUESTION-ANSWERING-001.1:** **R1.** When a caller submits a resolution for an **active** bundle and `CompleteActiveClarificationBundle` returns `claimed == true`, that caller SHALL be treated as the winner and SHALL be the only caller that performs step 5.
- **AC-INTEGRATIONS-EXTERNAL-QUESTION-ANSWERING-001.2:** **R2. A losing caller SHALL be told what the winner decided, not merely that it lost.** When the claim returns `claimed == false`, the system SHALL re-read the bundle's `clarification_request` messages and branch on their durable status:
- **AC-INTEGRATIONS-EXTERNAL-QUESTION-ANSWERING-001.3:** **At least one message carries `answered` or `rejected`** — a winner exists. The system SHALL return a **success** response carrying `claimed: false` plus the winner's `status` and reconstructed `response` (R2b), SHALL NOT modify any message, and SHALL NOT publish any event.
- **AC-INTEGRATIONS-EXTERNAL-QUESTION-ANSWERING-001.4:** **No message carries `answered` or `rejected`** — there is no winner; the bundle simply is not active (superseded turn, terminal session, `expired`, `cancelled`, or malformed). The system SHALL return **409** with `{"error": "clarification request is no longer active", "code": "not_active"}`. The `code` field is additive; the existing human-readable `error` value stays unchanged. This branch is the load-bearing addition, because `CompleteActiveClarificationBundle` returns the same `claimed == false` for both states and a caller cannot distinguish them. Collapsing them — in either direction — produces a concrete lie: reporting 409 for a duplicate answer hides the winner's payload, which is the whole point of idempotent replay; reporting "success, here is the winner's answer" for a superseded bundle invents a winner that does not exist and would have the MCP tool tell an agent its answer was superseded by an answer nobody gave.
- **AC-INTEGRATIONS-EXTERNAL-QUESTION-ANSWERING-001.5:** **R2a. The re-read is not required to agree with the failed claim, and SHALL NOT be retried.** The bundle can change again between the claim and the re-read. Whatever the re-read observes is what the caller is told; a single read is the contract. Retrying to obtain a "more settled" answer would race the same transitions indefinitely, and every outcome of the re-read is already a truthful statement about a real moment.
- **AC-INTEGRATIONS-EXTERNAL-QUESTION-ANSWERING-001.6:** **R2b. Reconstructing the winner's `response`.** `status` is the status of the **first message in D2 order** whose status is `answered` or `rejected`; a bundle claimed by upstream is uniform, so this tiebreak only ever binds on a legacy mixed bundle, where it is still total and reproducible. `answers` is built by walking the bundle's messages in D2 order and taking each message's `metadata.response` where present, in that order; a message without one contributes no entry, so a rejection yields `[]`. `rejected` is `true` when `status` is `rejected`. **`reject_reason` SHALL be the empty string on every replay.** Upstream stores no reject reason on the message rows — `completeClaimedClarificationMessages` writes only `status`, `response_delivery_pending` and, for `answered`, `response` — so there is nowhere to read it from. This is stated rather than repaired: repairing it means adding a metadata key to a shape the *Upstream baseline* freezes, which belongs to the active-lifecycle card, not this one. A loser therefore learns *that* the bundle was declined and *by what outcome*, but not the declining caller's prose.
- **AC-INTEGRATIONS-EXTERNAL-QUESTION-ANSWERING-001.7:** **R2c. Validation runs before the loser branch.** A malformed submission against an already-resolved bundle SHALL receive the same validation error it would receive against an active one, and SHALL NOT receive the winner's payload. Returning success for a request the system could not parse would tell a caller its malformed answer had been accepted. This follows from N8c ordering validation ahead of step 4, and is restated here because R2 is where it is observable.
- **AC-INTEGRATIONS-EXTERNAL-QUESTION-ANSWERING-001.8:** **R3.** While two callers submit resolutions for the same `pending_id` concurrently, exactly one SHALL observe `claimed == true` and the other SHALL observe R2's winner branch. This SHALL hold when the two callers are served by different HTTP requests, and — the case no existing test can cover, because the MCP tool did not exist when upstream's tests were written — **when one is the web UI over REST and the other is `answer_question_kandev` over MCP**.

## System design

The migrated technical source is split into [part 1](../system-design/external-question-answering-01.md), [part 2](../system-design/external-question-answering-02.md), [part 3](../system-design/external-question-answering-03.md), [part 4](../system-design/external-question-answering-04.md).
