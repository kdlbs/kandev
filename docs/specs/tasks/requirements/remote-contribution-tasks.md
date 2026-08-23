---
status: active
system: tasks
created: 2026-08-04
updated: 2026-08-19
owners:
  - product
---
# Remote Contribution Tasks Requirements

## Overview

Maintainers can create a task from an existing GitHub pull request or GitLab merge request and keep
the task checkout synchronized with the contributor's published change without losing local work.

## Requirements

### REQ-TASKS-REMOTE-CONTRIBUTION-TASKS-001: Remote Contribution Tasks

**Intent:** Prepare and operate on a remote contribution using provider-validated identity, explicit
user intent for destructive replacement, and evidence-based version comparison.

#### Acceptance criteria

- **AC-TASKS-REMOTE-CONTRIBUTION-TASKS-001.1:** When task creation receives a supported repository, pull-request, or merge-request URL, the system shall validate the provider identity, source branch, head SHA, target branch, and collaboration permission instead of trusting caller-supplied repository or branch metadata.
- **AC-TASKS-REMOTE-CONTRIBUTION-TASKS-001.2:** When a contribution is accepted, the system shall attach the task to the target repository, prepare the checkout at the provider-reported head SHA, configure a dedicated source remote for contributor pushes, and associate the existing provider change before agent launch.
- **AC-TASKS-REMOTE-CONTRIBUTION-TASKS-001.3:** When provider content is used to prepare a task, the system shall keep provider title and body outside trusted task text and prompts, and shall preserve ordinary repository URLs and ordinary task-created pull requests with their existing behavior.
- **AC-TASKS-REMOTE-CONTRIBUTION-TASKS-001.4:** When the checkout and provider history diverge, the system shall classify versions by repository, branch, commit identity, and ancestry evidence; it shall preserve the local task version and expose distinct provider/local actions without treating message or patch similarity as equality.
- **AC-TASKS-REMOTE-CONTRIBUTION-TASKS-001.5:** When a user chooses to replace the provider branch, the system shall require explicit confirmation and an exact provider-head lease; if the provider head changed, it shall leave both versions unchanged and request a fresh review.
- **AC-TASKS-REMOTE-CONTRIBUTION-TASKS-001.6:** When a user chooses the provider version, the system shall require a clean working tree, create a local recovery branch at the current task head, and reset to the confirmed provider head while reporting the recovery branch.
