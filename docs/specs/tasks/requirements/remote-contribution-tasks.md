---
status: active
system: tasks
created: 2026-08-04
updated: 2026-09-12
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

## Amendment: branch history explanations

Requirement 003 is implemented. Requirements 001 and 002 retain their current status.
The task system owns this amendment because it owns checkout preservation and
published-version choices across repositories and executors.

### REQ-TASKS-REMOTE-CONTRIBUTION-TASKS-003: Branch history explanations

**Intent:** Explain differing histories without falsely attributing a change to
the published branch or implying that a local rebase lost work.

#### Acceptance criteria

- **AC-TASKS-REMOTE-CONTRIBUTION-TASKS-003.1:** When histories diverge, the system shall show “Task and PR histories differ” without assuming which side changed.
- **AC-TASKS-REMOTE-CONTRIBUTION-TASKS-003.2:** When current evidence identifies a completed local rebase from the published head, the explanation shall identify that local rebase. Missing, stale, ambiguous, or incomplete evidence shall retain neutral wording.
- **AC-TASKS-REMOTE-CONTRIBUTION-TASKS-003.3:** When exact counts are available, the explanation shall distinguish task commits, published commits, and newer base commits. Counts shall not imply content equivalence or lost work. Unavailable counts shall be omitted.
- **AC-TASKS-REMOTE-CONTRIBUTION-TASKS-003.4:** The first action shall be “Compare versions”. It shall reveal both version histories without changing Git state. Secondary actions shall be “Publish task version...” and “Restore published PR version...”.
- **AC-TASKS-REMOTE-CONTRIBUTION-TASKS-003.5:** Publication and restoration shall retain criteria 001.5 and 001.6. Confirmations shall explain replacement effects. A local-rebase explanation shall not relax authorization or claim that validation passed.
- **AC-TASKS-REMOTE-CONTRIBUTION-TASKS-003.6:** Desktop and phone users shall receive the same explanation, comparison, and resolution choices. Phone explanations shall be touch-accessible without hover. Dismissal shall restore focus. Long content shall remain inside the viewport.
- **AC-TASKS-REMOTE-CONTRIBUTION-TASKS-003.7:** An explanation shall belong to one selected repository, checkout branch, and PR. Changes to that identity or either head shall invalidate it. Explanations shall not fetch, rewrite, or publish Git history automatically.
- **AC-TASKS-REMOTE-CONTRIBUTION-TASKS-003.8:** All changed copy shall be localized. Loading or failed explanation requests shall preserve the usable Changes view and existing action policy. Aligned and linear histories shall retain their behavior.

#### Exclusions

Automatic reconciliation, validation orchestration, agent intent inference,
content-based commit deduplication, and new provider support are excluded.
Existing GitLab presentation exclusions remain in force.

#### Design and delivery

- [Branch history explanations design](../system-design/branch-history-explanations.md)
- [Branch history explanations package](../../../plans/branch-history-explanations/plan.md)

## Delivery

Requirement 002 is implemented. See the
[contribution resume recovery package](../../../plans/contribution-resume-recovery/plan.md)
for the admission tests and verification evidence.

Requirement 003 is implemented. See the
[branch history explanations package](../../../plans/branch-history-explanations/plan.md)
for the bounded local-rebase observation, comparison-first UI, mobile parity, and rendered-flow
verification evidence.
