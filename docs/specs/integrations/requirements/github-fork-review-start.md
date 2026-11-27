---
status: active
system: integrations
created: 2026-09-23
updated: 2026-11-27
owners:
  - kandev
---

# GitHub fork review task start

## Overview

GitHub review watches can create tasks from pull requests whose code comes from
a fork. The linked task must remain available while unattended execution waits
for a user to start it.

## Requirements

### REQ-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001: Safe fork review launch

**Intent:** Preserve automatic review of same-repository pull requests while
requiring an explicit user start before a review watch runs fork-controlled
checkout content.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.1:** When provider data
  confirms that a watched pull request's head and target are in the same
  repository, the review task shall retain its configured auto-start behavior.
- **AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.2:** When provider data
  shows that the watched pull request head belongs to another repository, or
  that the provider has no head repository for it, the review task shall be
  created and associated without an unattended agent start.
- **AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.3:** A user shall be able to
  start a fork review task explicitly. The manual start shall not make later
  background auto-starts eligible.
- **AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.4:** User-submitted PR-link
  task creation shall keep its existing launch behavior.
- **AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.5:** When a review watch
  cannot read a new pull request's details, including throttling or a
  deferred background request, it shall not create that pull request's task
  during the poll. A later poll shall classify the pull request from complete
  provider data before it creates the task.
- **AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.6:** Opening a review task
  that waits for a manual start shall not report a session-start failure. The
  task shall stay without a session and without workspace preparation until
  the user starts it.
- **AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.7:** A task awaiting manual
  start shall show a warning and an explicit Start Task action in the empty
  chat. The warning shall explain that setup scripts from the PR may access
  executor credentials. The task edit dialog shall show the warning before
  the user starts the task.

## Exclusions

- Adding a fork-approval UI, trust label, or provider setting.
- Sandboxing setup scripts or changing their environment.
- Changing repository identity validation, contribution authorization, or push
  routing.
