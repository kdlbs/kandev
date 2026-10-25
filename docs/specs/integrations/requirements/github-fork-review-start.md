---
status: active
system: integrations
created: 2026-09-23
updated: 2026-09-23
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
- **AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.2:** When the watched pull
  request head belongs to another repository, or its head repository identity
  is unavailable, the review task shall be created and associated without an
  unattended agent start.
- **AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.3:** A user shall be able to
  start a fork review task explicitly. The manual start shall not make later
  background auto-starts eligible.
- **AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.4:** User-submitted PR-link
  task creation shall keep its existing launch behavior.

## Exclusions

- Adding a fork-approval UI, trust label, or provider setting.
- Sandboxing setup scripts or changing their environment.
- Changing repository identity validation, contribution authorization, or push
  routing.
