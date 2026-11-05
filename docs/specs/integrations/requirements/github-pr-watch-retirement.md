---
status: active
system: integrations
created: 2026-10-26
owners:
  - kandev
---

# GitHub PR watch retirement requirements

## Overview

An operator can stop one task's PR monitoring without archiving the task or removing its development resources.
Integrations owns this contract because it controls GitHub watch admission and provider requests.
The task-owned identity contract remains in Platform.

Retirement means an explicit, persistent stop for a canonical watch target.
Task completion, session completion, PR closure, and retirement are different conditions.
A retired target remains stopped until an authorized caller resumes it.

## Requirements

### REQ-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001: Persistent watch-only retirement

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.1:** Deleting a watch through the supported HTTP or WebSocket operation shall retire its canonical target. Repeated deletion shall succeed without additional effects.
- **AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.2:** Retirement shall survive restart, reconciliation, resumed sessions, task reopening, branch reuse, passive refresh, and explicit PR refresh. These operations shall neither recreate nor poll the retired target.
- **AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.3:** Retirement shall preserve tasks, sessions, environments, task-PR associations, CI state, and automation options. Retirement shall stop future watch-driven observations without changing stored automation configuration.
- **AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.4:** A retired watch shall not suppress an unrelated task, repository, or branch. Other eligible members of a shared provider target shall continue receiving observations.
- **AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.5:** An authorized caller shall be able to list retired watches and explicitly resume one. Resumption shall restore normal polling on the next eligible cycle, subject to authentication and rate limits.
- **AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.6:** Unauthorized callers shall neither inspect nor change retirement across workspace boundaries. A failed retirement write shall report failure and preserve the previous state.
- **AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.7:** After retirement commits, delayed provider results shall not change that watch's associations or publish watch-driven feedback. Resumption shall not make a pre-retirement result current again.
- **AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.8:** Existing watches shall remain active after upgrade. Archive and unarchive shall preserve explicit retirement. Permanent task deletion shall remove its retirement state.

## Compatibility and exclusions

This extends DELETE from removal of a poller row to persistent retirement.
There is no automatic retirement of every Done task or every terminal PR.
Eligible watches retain adaptive discovery, follow-up PR discovery, and terminal automation handling.
A genuinely different branch remains a separate target. A numbered retired watch also reserves its canonical PR identity.
Explicit association changes do not imply consent to resume a retired watch.

This package adds API controls only. It adds no rendered UI, native workflow action, bulk cleanup, request counter, or provider quota guarantee.
Already admitted remote requests can finish. Already queued agent actions are outside watch retirement.
Retirement does not prohibit independent, authorized PR reads by other features.

## Related contracts

- [Discovery and polling](github-pr-discovery-health.md)
- [Task-owned watch identity](../../platform/requirements/pr-watch-and-bounded-storage.md)
- [System design](../system-design/github-pr-watch-retirement.md)
- [Implementation plan](../../../plans/github-pr-watch-retirement/plan.md)
