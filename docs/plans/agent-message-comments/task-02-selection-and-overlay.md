---
id: agent-message-comments-02
title: Selection and overlay
status: complete
wave: 2
depends_on: agent-message-comments-01
plan: plan.md
---

## Acceptance

- Only eligible settled agent prose exposes selection actions; selection is message-local and cannot cross rows.
- Plan-style accent highlights and comment badges restore from stored offsets when a virtualized message remounts, without global DOM anchors.
- Selection first exposes the plan-style comment affordance. Desktop reuses the plan comment Popover; coarse-pointer/mobile uses an equivalent native Drawer with accessible Add/Run/edit/delete actions and safe-area/touch geometry.
- Activating a pending highlight or badge reopens it for update or deletion.

## Files

- `apps/web/components/task/chat/messages/chat-message.tsx`
- `apps/web/components/task/chat/messages/message-comment-surface.tsx`
- `apps/web/lib/chat/agent-message-comments.ts`
- focused component/unit tests

## Verification

- focused Vitest selection/highlight tests
- desktop and mobile rendered checks

## Output

Implemented message-local selection restoration, plan-style affordance/highlight/badge behavior, shared desktop plan Popover, and coarse-pointer/mobile Drawer interactions with Add/Run/update/delete behavior.

## Output contract

Summarize selection eligibility, anchor/highlight behavior, overlay composition, tests, and risks.
