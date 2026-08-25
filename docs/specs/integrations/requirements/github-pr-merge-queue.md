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

## Out of scope

- Displaying or navigating the complete merge queue.
- Removing a pull request from the merge queue.
- Selecting between direct merge and queue entry when GitHub permits both.
- Changing Kandev's independent CI auto-merge setting.
- Adding merge-queue behavior for GitLab merge requests.

## System design

- [GitHub PR Merge Queue System Design](../system-design/github-pr-merge-queue.md)

## Implementation plans

- [Queue-status visibility plan](../../../plans/github-pr-merge-queue-status/plan.md)
- [Original queue-aware merge action plan](../../../plans/github-pr-merge-queue/plan.md)
