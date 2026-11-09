---
status: active
system: integrations
created: 2026-10-27
owners:
  - Kandev
---

# Review thread context Requirements

## Overview

A reviewer can add a change-request comment thread to an agent's context. The
integration system owns the normalized external feedback and this shared
change-request detail outcome. The plugin host exposes the same detail surface
to compatible providers; its contribution contract remains plugin-owned.

## Terminology

- **Supplied thread:** A supplied root comment and the supplied comments whose
  parent identifies that root. When a parent is absent from the supplied
  comments, the orphan is treated as a root with its own supplied direct replies.
- **Whole-thread action:** The existing action that adds a comment thread to
  chat context, distinct from individual-comment and all-comments actions.

## Requirements

### REQ-INTEGRATIONS-REVIEW-THREAD-CONTEXT-001: Complete review thread context

**Intent:** A reviewer can give an agent the complete supplied conversation
when addressing a review thread.

#### Acceptance criteria

- **AC-INTEGRATIONS-REVIEW-THREAD-CONTEXT-001.1:** Activating the whole-thread
  action shall add the supplied root and every supplied direct reply exactly
  once, regardless of human or bot authorship, in root-first order followed by
  the supplied reply order, retaining the existing author, body, location, and
  change-request URL formatting.
- **AC-INTEGRATIONS-REVIEW-THREAD-CONTEXT-001.2:** A visible thread fragment
  with a supplied reply shall offer the whole-thread action even when its
  author-filtered display has no visible replies. If the fragment starts with a
  reply whose parent is supplied but belongs to the other author group, its
  action shall add the complete supplied parent thread.
- **AC-INTEGRATIONS-REVIEW-THREAD-CONTEXT-001.3:** If a comment's parent is
  absent, the orphan shall remain available as a root; its whole-thread action,
  when it has supplied direct replies, shall include that orphan and those
  replies without inventing a parent. A standalone comment shall not acquire a
  whole-thread action.
- **AC-INTEGRATIONS-REVIEW-THREAD-CONTEXT-001.4:** Human and bot comment
  visibility, the bot disclosure, individual and all-comments actions, reply
  action identity, and provider callback contracts shall retain their existing
  behavior.
- **AC-INTEGRATIONS-REVIEW-THREAD-CONTEXT-001.5:** Desktop keyboard and phone
  touch users shall be able to activate the whole-thread action and receive the
  same complete context. The phone action shall be visible without hover and
  have an active target of at least 44 by 44 pixels.

## Out of scope

- Fetching missing comments, changing provider data or transport, and changing
  the existing one-level thread relationship to support arbitrary nesting.
- Regrouping visible human and bot comments, changing the bot disclosure, or
  changing the content of individual or all-comments context.
- Plugin implementation changes or new user-facing copy.

## Implementation plan

[Review thread context plan](../../../plans/review-thread-context/plan.md)
