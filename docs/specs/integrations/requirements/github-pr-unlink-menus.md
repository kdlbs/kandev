---
status: draft
system: integrations
created: 2026-09-26
owners:
  - Kandev
---

# GitHub pull request unlink menus Requirements

## Overview

The integration system owns the task-to-GitHub-pull-request association and its
unlink operation. Users need a direct way to remove a linked PR from the task
top bar, sidebar task row, or Kanban card, including when it is the task's only
PR. Every surface must target the same association and show the resulting
linked set.

## Requirements

### REQ-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001: Unlink a task PR from contextual menus

**Intent:** A user can remove a specific GitHub PR association from task
surfaces without navigating to a review panel.

**User story:** As a task owner, I want to unlink a PR from the top bar or a
task menu, so that a task no longer points to the wrong PR.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001.1:** When a task has a linked
  GitHub PR, right-clicking its PR control in the desktop task top bar shall
  open a contextual `Edit` menu with an action to unlink that PR. Normal click
  and hover behavior shall remain available.
- **AC-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001.2:** When a sidebar task row or
  Kanban card has a linked GitHub PR, its desktop right-click menu shall offer
  the same unlink action under `Edit`. Their visible actions controls shall
  expose the action on a phone without requiring right-click or long press.
- **AC-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001.3:** When a task has multiple
  linked GitHub PRs, each menu shall identify the repository and PR number of
  every unlinkable association and remove only the selected association.
  Equal PR numbers in different repositories shall remain distinguishable.
- **AC-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001.4:** When the unlink succeeds,
  the selected association shall disappear from the top bar, card indicator,
  and other connected task surfaces. If it was the last association, the GitHub
  PR control and indicator shall disappear. The task and remote PR shall remain.
- **AC-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001.5:** While an unlink is in
  progress, repeated activation of the same action shall not start another
  request. If it fails, the association shall remain visible, an error shall be
  shown, and the user shall be able to retry.
- **AC-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001.6:** The menus shall omit unlink
  actions when no GitHub PR is linked. They shall not infer an association ID
  from a compact status summary alone; if detailed links are still loading, a
  task menu shall show a non-actionable loading state until exact choices exist.
- **AC-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001.7:** Every action shall have a
  localized, association-specific accessible name and be operable by keyboard.
  Phone menu entries shall have touch targets of at least 44 pixels and remain
  within the viewport without horizontal page overflow.

## Out of scope

- Unlinking GitLab merge requests or registered provider change requests from
  these GitHub PR menus.
- Deleting or closing a PR on GitHub, deleting the task, or changing its branch.
- Bulk unlink of all associations or all selected task cards.
- A new confirmation step for an operation already directly available in the
  existing multi-PR status surface.

## Related contracts

- [Manage task change requests](task-change-link-mcp.md) describes the active
  association and provider identity semantics used by agent tools.
- [Task PR synchronization](github-task-pr-sync-coordination.md) describes
  frontend freshness and stale-response protection.

## Implementation plan

[Implementation plan](../../../plans/github-pr-unlink-menus/plan.md)
