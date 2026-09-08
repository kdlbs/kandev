---
status: active
system: ui
created: 2026-09-02
owners:
  - kandev
---

# Plan Comment Drafts Requirements

## Overview

Task plans are shared by every session in a task, while pending plan comments
are browser-local prompt context owned by one task session. The UI system owns
the projection between those session-scoped drafts and the shared Plan editor.
Changing the selected Agent session can change which annotations are visible,
but it must never turn that presentation change into destructive user intent.

## Terminology

- **Owning session:** The task session selected when a pending plan comment is
  created and whose composer can submit that comment.
- **Comment projection:** The owning session's pending plan comments currently
  rendered as Plan highlights, badges, and composer context.
- **Projection reconciliation:** A UI-driven change to rendered comment marks
  after a session switch, hydration, or editor remount.
- **Destructive edit:** An explicit delete action or a user edit that removes a
  pending comment's marked plan range.

## Requirements

### REQ-UI-PLAN-COMMENT-DRAFTS-001: Preserve session-scoped plan comment drafts

**Intent:** Users can move between Agent sessions without losing pending
feedback they prepared against the shared task plan.

**User story:** As a user reviewing a task plan, I want pending comments to
survive Agent-session navigation, so that inspecting another session cannot
erase feedback I have not sent.

#### Acceptance criteria

- **AC-UI-PLAN-COMMENT-DRAFTS-001.1:** When a user adds a pending plan comment,
  the UI shall persist it in browser `sessionStorage` for the owning session and
  show its highlight, badge, and composer context while that session is
  selected.
- **AC-UI-PLAN-COMMENT-DRAFTS-001.2:** When the user selects a different session
  in the same task, the UI shall show that session's comment projection without
  deleting, modifying, or moving pending plan comments owned by the previous
  session.
- **AC-UI-PLAN-COMMENT-DRAFTS-001.3:** When the user returns to a session that
  owns pending plan comments, the UI shall restore their highlights, badges,
  feedback text, and composer context without requiring a reload or another
  user action.
- **AC-UI-PLAN-COMMENT-DRAFTS-001.4:** Projection reconciliation shall not count
  as comment deletion. Pending plan comments shall be removed only by an
  explicit delete action, a destructive edit of their marked plan range, or a
  successful delivery path. Failed delivery shall preserve them.
- **AC-UI-PLAN-COMMENT-DRAFTS-001.5:** When the selected session changes while a
  plan comment editor is open, the UI shall close that transient editor and
  clear its selection state. A later action in the new session shall not update,
  delete, or deliver the previous session's comment.
- **AC-UI-PLAN-COMMENT-DRAFTS-001.6:** Desktop and mobile task surfaces shall
  apply the same draft-preservation and session-ownership behavior.

## Out of scope

- Changing pending plan comments from session-scoped to task-scoped data.
- Persisting pending comments in the backend or synchronizing them across
  browsers or tabs.
- Changing Plan comment popover, Drawer, toolbar, or task-navigation layout.
- Recovering comments already removed by versions that predate this contract.
