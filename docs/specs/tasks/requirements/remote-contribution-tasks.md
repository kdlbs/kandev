---
status: active
system: tasks
created: 2026-08-04
updated: 2026-09-11
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
- **AC-TASKS-REMOTE-CONTRIBUTION-TASKS-001.7:** When Kandev refreshes provider history for the same contribution, the Changes panel shall keep the previous confirmed commit provenance visible until refreshed evidence replaces it. A pending or failed refresh shall not show those commits as newly unpushed. Retained evidence shall not authorize a remote mutation.

### REQ-TASKS-REMOTE-CONTRIBUTION-TASKS-002: Contribution resume after remote updates

**Intent:** Let the agent resume local work when the published contribution has changed, without selecting which history wins.

#### Acceptance criteria

- **AC-TASKS-REMOTE-CONTRIBUTION-TASKS-002.1:** When an existing contribution session resumes and its push check reports only a non-fast-forward rejection for the configured source branch, that rejection shall not prevent agent startup.
- **AC-TASKS-REMOTE-CONTRIBUTION-TASKS-002.2:** Resume shall preserve local commits, uncommitted files, the source branch, and the provider conversation identity. It shall not merge, reset, rebase, pull, or push to resolve remote updates automatically.
- **AC-TASKS-REMOTE-CONTRIBUTION-TASKS-002.3:** Authentication, permission, invalid destination, missing source branch, network, timeout, and unclassified preflight failures shall remain distinct from confirmed history rejection. History rejection shall not be represented as proof of write permission.
- **AC-TASKS-REMOTE-CONTRIBUTION-TASKS-002.4:** A task with multiple repositories shall start only when every required preflight passes or qualifies for the history-only resume exception. One qualifying repository shall not hide a blocking failure in another.
- **AC-TASKS-REMOTE-CONTRIBUTION-TASKS-002.5:** After resume, the existing Changes surface shall retain its provider/local version choices and their authorization conditions. A history-only rejection shall not create an agent failure or a recovery banner.

## Delivery

Requirement 002 is specified but not implemented. See the
[contribution resume recovery package](../../../plans/contribution-resume-recovery/plan.md).
