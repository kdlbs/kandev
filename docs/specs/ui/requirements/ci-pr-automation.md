---
status: active
system: ui
created: 2026-06-18
owners:
  - tbd
---
# Task PR Automation Controls Requirements

## Overview

Users can already see pull request CI/review status above the task chat input,
but acting on a red PR still requires repeatedly noticing the failure, prompting
the agent, and deciding when it is safe to merge. A review task can also go
idle after submitting a review and miss a later re-review request, merge, or
close. Users and task agents need automation controls that keep a linked PR
moving throughout its lifecycle, configured independently per linked PR so a
task with several open PRs does not force the same setting onto all of them.

Task-to-PR associations survive restarts and archive/unarchive. Hard deletion
removes task-owned associations and refresh watches; it is not contribution
history. Decision: ADR-2026-08-13-hard-delete-task-contribution-links.

Decision: [ADR-0051](../../../decisions/0051-pr-agent-notifications-extend-task-pr-automation.md)
(the task-level control plane for the five switches was superseded by per-PR
scoping; see that ADR's Consequences section).

## Requirements

### REQ-UI-CI-PR-AUTOMATION-001: Task PR Automation Controls

**Intent:** Users can already see pull request CI/review status above the task chat input, but
acting on a red PR still requires repeatedly noticing the failure, prompting the agent, and deciding
when it is safe to merge. A review task can also go idle after submitting a review and miss a later
re-review request, merge, or close. Users and task agents need automation controls that keep a
linked PR moving throughout its lifecycle, configured independently per linked PR so a task with
several open PRs does not force the same setting onto all of them. Task-to-PR associations survive
restarts and archive/unarchive. Hard deletion removes task-owned associations and refresh watches;
it is not contribution history. Decision: ADR-2026-08-13-hard-delete-task-contribution-links.
Decision: [ADR-0051](../../../decisions/0051-pr-agent-notifications-extend-task-pr-automation.md) (the
task-level control plane for the five switches was superseded by per-PR scoping; see that ADR's
Consequences section).

#### Acceptance criteria

- **AC-UI-CI-PR-AUTOMATION-001.1:** The PR CI popover above the chat input shows five automation controls, scoped to the selected linked PR:
- **AC-UI-CI-PR-AUTOMATION-001.2:** `Auto-fix CI & address comments`
- **AC-UI-CI-PR-AUTOMATION-001.3:** `Auto-merge when ready`
- **AC-UI-CI-PR-AUTOMATION-001.4:** `Your review is requested`
- **AC-UI-CI-PR-AUTOMATION-001.5:** `PR merged`
- **AC-UI-CI-PR-AUTOMATION-001.6:** `PR closed without merging`
- **AC-UI-CI-PR-AUTOMATION-001.7:** The automation section states which PR the controls apply to (the PR number, interpolated into localized copy), since a task can have several linked PRs each with independent settings.
- **AC-UI-CI-PR-AUTOMATION-001.8:** The automation section includes an info icon or equivalent help affordance that explains what each control watches, how often Kandev checks watched PRs, how feedback snapshots prevent duplicate prompts, and how auto-merge decides readiness.

## System design

The migrated technical source is split into [part 1](../system-design/ci-pr-automation-01.md), [part 2](../system-design/ci-pr-automation-02.md), [part 3](../system-design/ci-pr-automation-03.md).
