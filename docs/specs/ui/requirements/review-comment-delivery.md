---
status: active
system: ui
created: 2026-10-08
owners:
  - kandev
---

# Review comment delivery requirements

## Overview

Reviewers accumulate file and line notes before selecting **Fix comments**.
An unsuccessful delivery must leave that feedback available for correction and
retry. UI owns this browser-held Review feedback lifecycle, as established by
[review file comments](review-file-comments.md). Task session message admission
and execution remain separate contracts.

## Terminology

- **Submitted notes:** The pending file and line comments selected for one
  deliberate Fix comments attempt, with their original session, repository,
  file, anchor, metadata, and exact authored text.
- **Acknowledged delivery:** Successful confirmation of that attempt by the
  existing session message transport. This does not mean the agent has acted
  on the feedback.
- **Current notes:** Feedback presently available in the originating Review
  session, including edits and additions made after submission.

## Requirements

### REQ-UI-REVIEW-COMMENT-DELIVERY-001: Retain review feedback until acknowledged

**Intent:** Keep actionable feedback available until delivery is confirmed,
without erasing work authored while a send is pending.

#### Acceptance criteria

- **AC-UI-REVIEW-COMMENT-DELIVERY-001.1:** When a delivery is explicitly rejected,
  the current notes shall remain exactly available in Review and its existing
  same-tab persistence. Existing error feedback shall appear; failure shall not
  close an otherwise open Review as if delivery succeeded.
- **AC-UI-REVIEW-COMMENT-DELIVERY-001.2:** When no message transport is available,
  Fix comments shall preserve current notes and their same-tab persistence,
  issue no send, show failure feedback, and retain the retry path.
- **AC-UI-REVIEW-COMMENT-DELIVERY-001.3:** While an attempt awaits confirmation,
  its notes shall remain pending and persisted. The existing Fix comments
  control shall prevent another admitted send from that mounted Review owner,
  including rapid repeated activation and dismissal/reopening while pending.
- **AC-UI-REVIEW-COMMENT-DELIVERY-001.4:** A confirmed delivery shall remove only
  submitted notes that remain unchanged and pending in their original session.
  Edits to text, anchors, or repository/file metadata, new notes, and unrelated
  notes shall remain intact. A deleted note shall not be resurrected.
- **AC-UI-REVIEW-COMMENT-DELIVERY-001.5:** An admitted attempt shall deliver the
  submitted file and line feedback to the originally selected task session,
  retaining existing repository-qualified Markdown, source context, ordering,
  message identity, and normal busy-session handling. Subsequent edits shall
  not change that admitted request.
- **AC-UI-REVIEW-COMMENT-DELIVERY-001.6:** After a rejected or unavailable send,
  the user shall be able to deliberately retry the retained current feedback.
  A successful retry shall apply its own acknowledgement and clearing decision.
  No failure shall automatically resend feedback.
- **AC-UI-REVIEW-COMMENT-DELIVERY-001.7:** After confirmed delivery, Review shall
  close automatically only when no pending file or line notes remain for the
  same currently displayed task session. Delivery for a previous session shall
  not dismiss a different session's Review. Explicit user dismissal shall
  remain available, and settlement shall never reopen a dismissed Review.
- **AC-UI-REVIEW-COMMENT-DELIVERY-001.8:** Desktop and phone shall provide the
  same retention, pending admission, acknowledgement, and retry outcomes through
  their existing Review entry points. Current labels, composition, touch
  activation, scrolling, and navigation shall be retained.
- **AC-UI-REVIEW-COMMENT-DELIVERY-001.9:** A transport failure without a definitive
  delivery result shall retain current notes without reporting success or
  claiming the server rolled back. It shall not automatically retry or imply
  exactly-once delivery; the user can inspect the conversation before choosing
  whether to resend.

## Out of scope

Provider-hosted reviews, other composers' send contracts, backend or protocol
changes, cross-tab or server draft persistence, automatic transport recovery,
global send coordination, review navigation redesign, and new telemetry.
Existing browser storage availability and lifetime limits remain applicable.

## Related artifacts

- [System design](../system-design/review-comment-delivery.md)
- [Implementation plan](../../../plans/preserve-review-comments/plan.md)
