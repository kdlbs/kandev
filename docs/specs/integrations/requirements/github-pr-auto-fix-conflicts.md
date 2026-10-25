---
status: draft
system: integrations
created: 2026-09-05
owners:
  - Kandev
---
# GitHub PR Auto-Fix Conflicts Requirements

## Overview

GitHub reports a pull request with unresolved merge conflicts as `dirty`.
Kandev blocks auto-merge for this state, but auto-fix does not send the state
to the linked task agent.

The existing auto-fix option must treat a merge conflict as actionable pull
request feedback. Merge-queue removals remain under the merge-queue recovery
contract.

## Requirements

### REQ-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001: Conflict repair prompt

**Intent:** Auto-fix sends a current merge conflict to the linked task agent
without sending the same conflict state on every poll.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001.1:** When auto-fix is
  enabled, checks are settled, and an open linked pull request is `dirty`,
  Kandev shall send or queue one auto-fix prompt.
- **AC-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001.2:** When a user enables
  auto-fix while the linked pull request is already `dirty`, the next eligible
  evaluation shall send or queue the conflict prompt.
- **AC-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001.3:** The conflict state,
  head commit, head branch, and base branch shall participate in the auto-fix
  checkpoint. Repeated observations of the same snapshot shall not send a
  duplicate prompt.
- **AC-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001.4:** When an authoritative
  mergeability state shows that the conflict cleared, Kandev shall clear it
  from the checkpoint without using another auto-fix round.
- **AC-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001.5:** When a conflict
  reappears after it clears, or the head commit or target branch changes while
  the pull request remains `dirty`, Kandev shall allow one new auto-fix round.
- **AC-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001.6:** When the auto-fix
  prompt includes `{{pr.feedback}}`, the visible snapshot shall identify the
  merge conflict and its head and base branches.
- **AC-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001.7:** Conflict prompts shall
  use the existing session selection, durable queue, coalescing, and 10-round
  limit for that linked pull request.
- **AC-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001.8:** Mergeability states
  other than `dirty` shall not start a conflict repair prompt.
- **AC-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001.9:** The desktop popover
  and mobile drawer help shall state that auto-fix handles ordinary merge
  conflicts and actionable merge-queue removals.

## Related requirements

- [GitHub PR Merge Queue Recovery](github-pr-merge-queue-recovery.md)
- [Task PR Automation Controls](../../ui/requirements/ci-pr-automation.md)
- [Merge Queue Recovery Controls](../../ui/requirements/ci-pr-merge-queue-recovery-controls.md)

## Out of scope

- Changing the auto-fix switch label.
- Starting a conflict repair round while pull-request checks are unsettled.
- Selecting a merge strategy for the agent.
- Editing branch protection, rulesets, or workflow files.
- GitLab merge-request behavior.
