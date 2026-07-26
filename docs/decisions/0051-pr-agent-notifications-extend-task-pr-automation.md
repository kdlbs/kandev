# 0051: PR Agent Notifications Extend Task PR Automation

**Status:** accepted
**Date:** 2026-07-23
**Area:** backend, frontend, workflow, GitHub, MCP

## Context

A PR Review task goes idle after its agent submits a review. A later review
request, merge, or close must be able to wake that same task. Kandev already
has task-level PR automation options, per-PR checkpoints, linked-PR records,
the one-minute PR watch poller, agent prompt queueing, prompt overrides, and
desktop/mobile controls.

## Decision

Extend the existing GitHub task PR automation subsystem:

- add task-level `prompt_on_review_requested`, `prompt_on_merged`, and
  `prompt_on_closed` options beside auto-fix and auto-merge;
- keep transition and dedupe checkpoints per linked PR in
  `github_task_ci_pr_state`;
- continue using `github_pr_watches` and its one-minute batched poller as the
  source of PR facts;
- use the authenticated GitHub login to classify review requests;
- deliver prompts through the orchestrator's existing single CI automation pass
  (same goroutine and in-flight map as auto-fix and auto-merge);
- expose current-task get/update tools to task-mode MCP agents;
- render the same switches in the existing desktop popover and mobile drawer.

Schema evolution is additive. The existing table and API names retain `ci` for
compatibility even though their product meaning broadens to task PR automation.

The PR Review workflow enables these options through MCP after its initial
review. Review-requested is silently baselined and fires on a later
false-to-true request edge. Merged and closed fire once per observed terminal
entry. Stable states remain quiet, and an observed open state rearms a later
close.

## Consequences

- PR automation has one task-level control plane for users and agents.
- Lifecycle evaluation runs inside the single CI automation pass — one goroutine
  and one in-flight map handle auto-fix, auto-merge, and lifecycle for a PR.
- Existing auto-fix, auto-merge, PR watch, and linked-PR behavior remains
  compatible.
- Review tasks remain retained under `cleanup_policy=auto` while PR-agent
  prompt options express ongoing intent.
- Reviewer-specific detection depends on the authenticated user-level review
  request. Team-level matching remains future work.
