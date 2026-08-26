---
status: draft
system: integrations
created: 2026-08-24
owners:
  - Kandev
---
# GitHub PR Merge Queue Recovery Requirements

## Overview

GitHub can remove a pull request from a merge queue after a merge-group check
fails or a conflict appears. Kandev shows active queue membership, but it does
not retain the removal event or repair the pull request.

Queue-aware automation extends the existing per-PR auto-fix and auto-merge
controls. It does not add a separate queue-recovery preference.

## Terminology

- **Queue attempt:** One queue entry for one pull-request head commit.
- **Queue removal:** A GitHub `RemovedFromMergeQueueEvent` for an open pull
  request.
- **Actionable removal:** A queue removal with a failed check, a timed-out
  check, or a merge conflict.

## Requirements

### REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-001: Queue removal observation

**Intent:** Kandev must retain a queue removal after the active queue entry
disappears. This record lets automation respond after a short queue attempt.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-001.1:** When GitHub reports
  a queue removal, Kandev shall store its provider event ID, time, reason, and
  available commit identity.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-001.2:** When Kandev misses
  the active queue entry, a later poll shall still detect the latest queue
  removal from the pull-request timeline.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-001.3:** When Kandev
  restarts, the last observed queue attempt and removal shall remain available
  for deduplication and status display.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-001.4:** When a queue-aware
  read fails, Kandev shall preserve the last complete queue-recovery state.

### REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002: Queue failure auto-fix

**Intent:** Auto-fix must send actionable merge-queue failures to the linked
task agent without sending the same removal more than once.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002.1:** When auto-fix is
  enabled and a new actionable removal appears, Kandev shall send or queue one
  auto-fix prompt for that removal.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002.2:** The prompt shall
  include the queue-removal reason and conflict state. It shall include failed
  check names and links only when Kandev has an exact commit identity for those
  checks.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002.3:** The removal event ID
  shall participate in the auto-fix checkpoint. Repeated polls shall not send
  the same removal again.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002.4:** A queue-recovery
  prompt shall consume one of the existing 10 auto-fix rounds for that linked
  pull request.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002.5:** A manual or unknown
  removal without failed-check or conflict evidence shall not start auto-fix.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002.6:** When the task is
  busy, the queue-recovery prompt shall use the existing durable auto-fix queue
  and coalescing rules.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002.7:** When a user enables
  auto-fix after Kandev retained an unhandled actionable removal for the
  current head, the next evaluation shall send or queue one repair prompt.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002.8:** Kandev shall not
  treat the removal event's commit as a merge-group commit unless GitHub
  identifies it as such.

### REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003: Safe automatic requeue

**Intent:** Auto-merge must requeue a repaired pull request without creating a
same-head retry loop or reversing an intentional queue removal.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003.1:** When GitHub accepts
  a queue attempt, Kandev shall record the attempted pull-request head commit.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003.2:** When GitHub removes
  that queue attempt, auto-merge shall not requeue the same head commit.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003.3:** When the head commit
  changes and the existing merge gates pass, auto-merge shall submit one new
  queue-aware merge request.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003.4:** A user or agent can
  repair the pull request when auto-fix is disabled. A later eligible head
  commit shall still re-arm auto-merge.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003.5:** When both controls
  are enabled, each actionable removal can start one repair round. A new
  eligible head can then start one queue attempt.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003.6:** Kandev shall use
  GitHub's queue-aware default merge action. It shall not bypass repository
  rules or choose direct merge instead of the required queue.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003.7:** When a user enables
  auto-merge while the pull request already has an active queue entry, Kandev
  shall adopt that entry and head as the current attempt. It shall not submit a
  duplicate merge request.
- **AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003.8:** When a user enables
  auto-merge after a queue removal, Kandev shall keep the same-head guard. It
  shall wait for a new eligible head instead of reversing the removal.

## Related requirements

- [GitHub PR Merge Queue](github-pr-merge-queue.md)
- [Task PR Automation Controls](../../ui/requirements/ci-pr-automation.md)
- [Merge Queue Recovery Controls](../../ui/requirements/ci-pr-merge-queue-recovery-controls.md)

## Out of scope

- Webhook ingestion for merge-group events.
- Automatic retry of a flaky queue failure without a new pull-request head.
- Editing branch protection, rulesets, or workflow files from the controls.
- Removing a pull request from the queue.
- Coordinated recovery and queue ordering for stacked pull requests.
- GitLab merge-request behavior.
