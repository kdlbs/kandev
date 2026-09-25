---
status: active
system: tasks
created: 2026-09-15
owners:
  - kandev
---

# Remote repository options

## Overview

Users configure downloads and included folders independently for each remote
repository in a new task. Tasks own this contract because the choices belong
to a task's repository attachments and survive that task's retry and resume.
Workspace repository defaults and other tasks are unaffected.

The user confirmed a gear beside each remote repository and selected task-only
settings rather than remembered repository defaults. The controls and defaults
below describe the implemented task-owned behavior.

## Requirements

### REQ-TASKS-REMOTE-OPTIONS-001: Per-repository advanced controls

**Intent:** Keep ordinary task creation compact while making advanced options discoverable.

#### Acceptance criteria

- **AC-TASKS-REMOTE-OPTIONS-001.1:** Each selected remote repository shall expose a visible gear with an accessible name identifying the repository. Opening it shall not change repository or branch selection or remove the row. Rows without a selected or submitted repository URL shall not show the gear.
- **AC-TASKS-REMOTE-OPTIONS-001.2:** The settings surface shall expose download mode (Standard or Download on demand) and included folders (All folders or Selected folders). Standard and All folders shall be the initial choices. Standard shall preserve the executor's existing download behavior; Download on demand shall preserve commit history while fetching file contents as needed.
- **AC-TASKS-REMOTE-OPTIONS-001.3:** Selected folders shall accept multiple repository-relative directory paths with per-entry errors. Apply shall update only that row; Cancel or dismissal shall discard uncommitted changes. Reset shall restore the initial choices. Applied non-default settings shall have a visible summary beside or below that repository.
- **AC-TASKS-REMOTE-OPTIONS-001.4:** Desktop shall use a compact settings panel; phone shall use an inset bottom drawer with repository context, one scrolling form region, and reachable Apply and Cancel actions. Gear and action hit targets shall be at least 44 pixels on phone/coarse pointers. Keyboard dismissal shall close only the settings surface and return focus to its opener. All copy shall be localized.

### REQ-TASKS-REMOTE-OPTIONS-002: Task-only ownership

**Intent:** Preserve task choices without changing another task's files or future defaults.

#### Acceptance criteria

- **AC-TASKS-REMOTE-OPTIONS-002.1:** Applied options shall belong to the specific task-repository attachment, remain stable across task submission, restart, retry, and session resume, and never become workspace or repository defaults.
- **AC-TASKS-REMOTE-OPTIONS-002.2:** Within the creation form, row removal shall remove its options and changing to a different repository shall reset them. Reordering, branch changes, and toggling away from and back to Remote shall preserve applied choices for the same row. A new independent task shall start with initial choices.
- **AC-TASKS-REMOTE-OPTIONS-002.3:** Options shall remain distinct for multiple repository attachments, including different branches of the same repository. Editing one task or preparing one attachment shall not alter another task's checkout scope. Changing options on an already materialized environment is excluded from this release.
- **AC-TASKS-REMOTE-OPTIONS-002.4:** API callers omitting options and existing tasks without options shall retain current behavior. Invalid options shall be rejected before launch, including attempts to apply remote clone options to a user-managed local repository. An unsupported executor/provider combination shall have an explicit reason and shall never silently ignore selected options.

### REQ-TASKS-REMOTE-OPTIONS-003: Effective preparation

**Intent:** Make the controls reduce actual preparation work while retaining normal development operations.

#### Acceptance criteria

- **AC-TASKS-REMOTE-OPTIONS-003.1:** Download on demand shall avoid eagerly downloading historical file contents. If Selected folders is also chosen, preparation shall avoid first populating the entire working tree. Selected directories and root/ancestor files needed by directory-based sparse checkout shall be available before agent startup.
- **AC-TASKS-REMOTE-OPTIONS-003.2:** Missing directories at the resolved revision, absolute paths, parent traversal, control characters, and unsupported path expressions shall produce actionable errors rather than silently falling back to All folders. An empty Selected folders list shall block Apply and submission.
- **AC-TASKS-REMOTE-OPTIONS-003.3:** After initial clone credentials are released, checkout, diff, historical file reads, and fetch shall still obtain missing objects using the execution's authorized credentials. Credential expiry shall retain existing renewal behavior; revocation shall fail without another account fallback. Secrets shall not appear in options, repository URLs, or errors.
- **AC-TASKS-REMOTE-OPTIONS-003.4:** Reused repositories shall retain valid downloaded objects. Reuse shall check repository identity, selected revision, and option compatibility; interrupted preparation shall not be accepted merely because a Git directory exists. Retries shall preserve user work and never overwrite another task's checkout.
- **AC-TASKS-REMOTE-OPTIONS-003.5:** Selected folders shall remain scoped to the task's isolated working tree. Agent tools shall see the selected folders, with omitted files treated as outside the checkout rather than deleted. Git may materialize additional files during conflict handling; sparse checkout is not a security sandbox.

## Out of scope

- Remembered defaults, shallow-depth controls, arbitrary Git flags, automatic dependency discovery, and a remote directory-browser API.
- Editing scope after materialization, converting user-owned local repositories, or changing contribution head/branch authority.
- The separate preparation progress, cancellation, and timeout redesign from the large-repository investigation. These remain recommended follow-up work; this package alone does not remove the five-minute timeout.

## Implementation plans

- [Remote repository options](../../../plans/remote-repository-options/plan.md)
