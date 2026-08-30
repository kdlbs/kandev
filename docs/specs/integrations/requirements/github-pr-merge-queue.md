---
status: active
system: integrations
created: 2026-08-17
owners:
  - Kandev
---

# GitHub PR Merge Queue Requirements

## Overview

The integration system owns GitHub merge actions and merge-queue state. Users
must be able to merge an eligible pull request without leaving Kandev. Kandev
must show whether GitHub merged the pull request or added it to a merge queue.

The same provider contract owns automatic merge attempts from linked tasks.
Kandev must bind each attempt to the pull-request state that passed its readiness gates.

## Terminology

- **Readiness signature:** A stable identity for one pull-request head and its
  observed merge gates.
- **Automatic attempt:** A merge request that Kandev starts after auto-merge
  observes a ready pull request.
- **Explicit retry:** One user-requested evaluation of a prior failed automatic
  attempt for one linked pull request.

## Requirements

### REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001: GitHub PR Merge Queue

**Intent:** Users can complete an eligible GitHub pull request through the
repository's configured merge path. They can then observe its current queue
state from the existing pull-request surfaces.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.1:** When GitHub requires a merge queue for an eligible pull request, Kandev shall expose the existing merge action.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.2:** When the user activates the merge action, Kandev shall let GitHub select a direct merge or the configured queue.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.3:** When GitHub accepts the request, Kandev shall report whether GitHub merged or queued the pull request.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.4:** After GitHub accepts the request, Kandev shall prevent repeated submission until the pull-request state refreshes.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.5:** When a pull request has an active queue entry, Kandev shall use GitHub's merge-queue color, `#966600`.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.6:** When states compete, Kandev shall prioritize terminal states, then queue state, then other non-terminal states.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.7:** When GitHub supplies queue metadata, Kandev shall show localized state, position, and available estimate data.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.8:** When an estimate is less than one minute, Kandev shall use a localized sub-minute label. Larger estimates shall round up.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.9:** When GitHub omits an estimate, Kandev shall show state and position without invented estimate data.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.10:** When GitHub supplies an unknown non-empty state, Kandev shall show generic localized copy instead of the raw value.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.11:** When a refresh cannot observe queue membership, Kandev shall preserve the last confirmed entry. An authoritative empty or terminal state shall clear it.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.12:** When a pull request is a draft, conflicted, failing checks, awaiting checks, missing reviews, or has requested changes, Kandev shall hide the merge action.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.13:** When GitHub rejects a merge request, Kandev shall show the error, preserve state, and leave the action available for retry.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.14:** When users view queue state on desktop or mobile, Kandev shall provide the same outcome without hover-dependent controls or horizontal page overflow.

### REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002: Reliable automatic merge attempts

**Intent:** Automatic merge must act on the reviewed pull-request head without
repeating an unchanged side effect or leaving obsolete errors after GitHub accepts it.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.1:** When Kandev starts an
  automatic merge, the request shall require the non-empty head SHA from the
  fresh readiness snapshot. A different current head shall not merge.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.2:** Before Kandev calls GitHub,
  it shall durably reserve the per-PR readiness signature and attempted head.
  A restart or unchanged later poll shall not repeat that automatic request.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.3:** When a readiness gate or the
  pull-request head changes, Kandev shall permit one new automatic attempt only
  after every current gate passes.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.4:** When the user retries a
  failed automatic merge, Kandev shall reevaluate only the named linked pull
  request. The retry shall not bypass readiness, head binding, repository
  scope, permissions, or GitHub policy.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.5:** When GitHub reports an
  active queue entry or a merged pull request, Kandev shall reconcile the
  attempt as accepted and clear only its obsolete auto-merge error.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.6:** When Kandev cannot persist
  the automatic attempt reservation, it shall not call GitHub and shall show a
  retryable per-PR error.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.7:** While GitHub reports a
  pending asynchronous merge, Kandev shall request status no more than once per
  second and shall stop when the bounded operation context ends.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.8:** When an upgrade finds a
  recognized legacy auto-merge error, Kandev shall classify it so a later queue
  or merged observation can clear it.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.9:** When users view an
  auto-merge error on desktop or mobile, they shall have the same explicit
  retry action. A load error shall keep a separate state-refresh action.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.10:** When auto-fix is enabled
  for at least one active linked pull request, the task-row pull-request icon
  shall show a yellow indicator at its top-left. When auto-merge is enabled,
  it shall show a purple indicator at its top-right. Both indicators can
  appear together.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.11:** When a user examines the
  task-row pull-request icon with a pointer, keyboard, or touch input, Kandev
  shall identify the enabled automation settings and the active pull requests
  that use them. Merged and closed pull requests shall not produce active
  automation indicators.

## Out of scope

- Displaying or navigating the complete merge queue.
- Removing a pull request from the merge queue.
- Selecting between direct merge and queue entry when GitHub permits both.
- Changing the stored value or default of Kandev's independent CI auto-merge setting.
- Changing automation settings from the task-row pull-request indicator.
- Adding merge-queue behavior for GitLab merge requests.

## System design

- [GitHub PR Merge Queue System Design](../system-design/github-pr-merge-queue.md)

## Implementation plans

- [Automatic merge reliability plan](../../../plans/github-auto-merge-reliability/plan.md)
- [Sidebar automation indicators plan](../../../plans/github-sidebar-automation-indicators/plan.md)
- [Queue-status visibility plan](../../../plans/github-pr-merge-queue-status/plan.md)
- [Original queue-aware merge action plan](../../../plans/github-pr-merge-queue/plan.md)
