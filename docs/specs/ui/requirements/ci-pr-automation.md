---
status: active
system: ui
created: 2026-06-18
updated: 2026-09-11
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

Decision: [ADR-2026-09-06](../../../decisions/2026-09-06-explicit-pr-auto-fix-outcomes.md)
(auto-fix prompts remain retryable until their bound turn records an explicit
outcome or provider progress changes the feedback generation).

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
- **AC-UI-CI-PR-AUTOMATION-001.9:** While one auto-fix prompt for a linked PR is queued or its bound agent turn is running, later observations of the same actionable snapshot shall coalesce without starting another round.
- **AC-UI-CI-PR-AUTOMATION-001.10:** An auto-fix agent turn shall report one explicit outcome for its bound PR feedback: action taken, non-actionable feedback, or blocked work. The report shall be accepted only from the task session and turn that received the current auto-fix prompt.
- **AC-UI-CI-PR-AUTOMATION-001.11:** When an auto-fix turn ends successfully or with a recoverable agent failure without a valid outcome, the same actionable snapshot shall become eligible for another round on a later settled PR evaluation.
- **AC-UI-CI-PR-AUTOMATION-001.12:** When the agent reports action taken, Kandev shall wait for provider-visible progress. If the PR head, check execution identity, review-thread state, conflict state, or queue-removal state does not change within two PR watch intervals, the same snapshot shall become eligible for another round.
- **AC-UI-CI-PR-AUTOMATION-001.13:** A non-actionable or blocked outcome shall acknowledge the current snapshot without repeatedly prompting. New or materially changed provider feedback shall rearm auto-fix, and blocked outcomes shall remain visible through the existing per-PR automation error surface.
- **AC-UI-CI-PR-AUTOMATION-001.14:** Every retry shall consume the existing per-PR 10-round budget. Kandev shall never create an 11th accepted round, and disabling then re-enabling auto-fix shall reset the retry state with the existing checkpoint and round state.
- **AC-UI-CI-PR-AUTOMATION-001.15:** Existing installations whose built-in `ci-auto-fix` prompt still matches a shipped, untouched legacy revision shall receive the current default content on startup. Kandev shall preserve edited prompt rows, and the server-owned outcome instructions shall apply even when a task or global prompt is customized.

### Outcome scope clarification

The following criteria clarify the existing turn-bound outcome contract.
The [scope repair plan](../../../plans/pr-auto-fix-outcome-scope/plan.md) records the implementation and regression coverage.

- **AC-UI-CI-PR-AUTOMATION-001.16:** Agent guidance shall restrict outcome reporting to the current Kandev-dispatched auto-fix turn. Manual PR fixup, sibling review messages, and historical auto-fix instructions shall not establish that obligation. Tool availability and enabled automation settings alone shall not establish that obligation.
- **AC-UI-CI-PR-AUTOMATION-001.17:** When no matching unresolved attempt exists, Kandev shall reject the report with a scope explanation. The explanation shall direct the agent to finish ordinary work without retrying the report or enabling automation. The rejected report shall not create an attempt, consume a round, or change automation settings.

## Implementation plans

- [Outcome retries](../../../plans/pr-auto-fix-outcome-retries/plan.md): existing implementation.
- [Outcome scope repair](../../../plans/pr-auto-fix-outcome-scope/plan.md): implementation record.

## System-design references

The migrated technical source is split into [part 1](../system-design/ci-pr-automation-01.md), [part 2](../system-design/ci-pr-automation-02.md), [part 3](../system-design/ci-pr-automation-03.md).
