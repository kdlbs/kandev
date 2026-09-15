---
status: active
system: tasks
created: 2026-07-22
owners:
  - kandev
---
# Attach Workspace Sources Requirements

## Overview

Tasks can add repositories and folders after creation without losing their existing conversation,
state, or repository attachments. The operation is validated and materialized as one task-workspace
change.

## Requirements

### REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001: Attach Workspace Sources

**Intent:** Let users and agents attach supported workspace sources while preserving task state and
providing atomic validation and materialization.

#### Acceptance criteria

- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.1:** When an idle task opens workspace actions, the system shall offer source attachment and workspace-folder actions on desktop and touch surfaces, and shall preserve configured source rows while the user builds a batch.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.2:** When a task submits valid repository and folder sources, the system shall persist and materialize every source, expose repository sources in repository-aware task surfaces, and keep folders file-only.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.3:** When a submission contains an invalid, inaccessible, contradictory, cross-workspace, or executor-incompatible source, the system shall reject it before changing the task or executor workspace.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.4:** When any source in a multi-source submission fails during materialization, the system shall roll back all new attachments and restore any pre-existing Kandev-owned entry that the submission repointed.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.5:** When attachment changes the effective task root, the system shall preserve task state, plans, conversations, sessions, and existing attachments, publish the updated workspace projection, and retain the native provider session when compatible. If native resume is incompatible, recorded-history continuation shall require an explicit user action.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.6:** When an agent uses the legacy worktree-only add-branch action, the system shall create the new worktree as a task-root sibling, return its exact paths, refresh task projections, and leave the running agent working directory unchanged.


### REQ-TASKS-ATTACH-WORKSPACE-SOURCES-002: Initial workspace layout

**Intent:** Let a Worktree task reserve a parent workspace before its first launch.

#### Acceptance criteria

- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.1:** For a new single-repository Worktree task, Advanced shall offer “Start in a parent workspace folder”, disabled by default.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.2:** With that option enabled, the agent shall start in the task folder with the repository beneath it. With it disabled, single-repository startup shall retain the repository root.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.3:** The creation option shall survive queued launch, resume, backend restart, and additional sessions. Repository count changes alone shall not move an established workspace.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.4:** A task created with multiple repositories shall retain its parent-root layout. The dialog shall explain that this layout already applies.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.5:** Unsupported executors and repositoryless tasks shall not offer the option. Explicit unsupported values shall fail before task creation.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.6:** Existing tasks shall retain their recorded workspace and repository locations. Invalid or ambiguous workspace identity shall produce an actionable error without moving or recreating checkouts.

### REQ-TASKS-ATTACH-WORKSPACE-SOURCES-003: Repository placement

**Intent:** Let users choose repository locations while preserving existing work.

#### Acceptance criteria

- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.1:** For an eligible idle Worktree task rooted inside its primary repository, Add to workspace shall offer three repository placements: inside `./kandev/`, directly inside the current folder, or as siblings after workspace-root expansion.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.2:** Either nested placement shall create real worktrees below the established agent workspace. The agent CWD, native session, terminals, and development processes shall remain unchanged.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.3:** Root expansion shall place new repositories below the existing task folder and adopt that folder at an idle boundary. Existing repository contents and locations shall remain unchanged.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.4:** Nested repositories shall remain visible in Files and repository-specific Changes, editor, branch, and PR surfaces. Ordinary outer-repository status and staging shall exclude them without changing tracked ignore files or unrelated worktrees.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.5:** Name collisions, tracked destinations, unsafe paths, stale workspace previews, and incompatible live session roots shall fail before mutation. No existing directory shall be overwritten or adopted without ownership proof.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.6:** A failed batch shall restore its prior source rows, inventory, exclusions, and runtime state. Exact retries shall preserve the selected locations without duplicate worktrees or session restarts.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.7:** Resume, additional sessions, cleanup, and restart shall preserve each repository's recorded placement. Cleanup shall remove only owned resources under the existing preservation rules.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.8:** A task already rooted at its parent workspace shall add siblings without another layout-choice prompt or agent restart.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.9:** The existing active-turn legacy add-branch tool shall retain its sibling placement and unchanged-CWD contract. Its response shall not imply that a provider sandbox grants access.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.10:** Explicit nested placement shall apply to repository-only batches. Folder or mixed batches and unsupported executors shall retain their existing attachment contract.

### REQ-TASKS-ATTACH-WORKSPACE-SOURCES-004: Informed placement choice

**Intent:** Show the consequences before a workspace change.

#### Acceptance criteria

- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-004.1:** Each placement choice shall show its destination, benefits, and costs before submission. Nested choices shall explain instruction inheritance. Expansion shall explain process stops and native-session limitations.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-004.2:** The selected choice shall show the resulting tree and agent working directory. The action shall distinguish adding repositories from expanding the workspace.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-004.3:** Creation-time help shall explain future sibling attachment and differences in instruction and skill discovery. Help shall work with hover, keyboard focus, and touch.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-004.4:** Desktop and phone shall expose the same choices and information. Phone content shall scroll vertically within the drawer, with a reachable fixed action area and no page-level horizontal overflow.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-004.5:** Busy, unsupported, collision, and recovery-required states shall show actionable reasons. Failed submission shall preserve selected repositories, branches, and placement.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-004.6:** Cancel shall leave the task unchanged. Submission shall prevent duplicates and show progress. Context continuation shall never be inferred from opening the dialog or choosing a placement.

### REQ-TASKS-ATTACH-WORKSPACE-SOURCES-005: Add sources from the Files toolbar

**Intent:** Make adding sources discoverable beside file creation and uploads.

#### Acceptance criteria

- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-005.1:** The Files toolbar + menu shall contain Add repositories or folders alongside existing New file and upload actions. The overflow menu shall retain Open workspace folder and shall no longer duplicate source attachment.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-005.2:** A task with no repositories shall be eligible to add supported sources. Busy, loading, disconnected, and unsupported states shall show specific visible reasons, independent of repository count.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-005.3:** Desktop keyboard and phone touch users shall complete the same flow. Closing the source dialog shall restore focus to +. Existing file creation and upload actions shall remain available according to their capabilities.

### REQ-TASKS-ATTACH-WORKSPACE-SOURCES-006: Grow an established folder or scratch workspace

**Intent:** Add repositories and folders without replacing the workspace where work started.

#### Acceptance criteria

- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-006.1:** An idle task started from a local folder or None/scratch shall add supported repositories, folders, or mixed batches beneath its established CWD. It shall preserve existing files, task identity, conversation, native session, and running workspace processes.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-006.2:** Local folder and scratch tasks shall offer direct child placement or grouping under ./kandev/. The preview shall show resolved destinations and unchanged agent CWD. Folder links shall disclose that edits affect their original folder and that a provider may restrict access to the target.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-006.3:** Adding the first repository shall expose repository-specific Changes, branch, and PR capabilities when supported. It shall not initialize Git in the enclosing folder, change executor, move the workspace, or convert unrelated files into repository content. Folder-only attachment shall remain file-only.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-006.4:** Resume, restart, and additional sessions shall retain the original root and recorded source locations regardless of source count. Cleanup shall preserve user-owned roots and linked source folders and apply existing preservation rules to owned resources.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-006.5:** Remote and container tasks shall add cloneable repositories inside their existing executor workspace when supported, including tasks started without a repository. Paths and consequences shall identify the executor filesystem. Host-only repository paths shall require a cloneable remote identity.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-006.6:** The source picker shall distinguish live folder attachment from Upload folder. Arbitrary host folder attachment shall remain unavailable on remote/container executors with an explicit reason; existing upload support may offer a separate, explicitly described copy action. No automatic mount, transfer, or synchronization is implied.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-006.7:** Capability and destination checks shall use the actual environment and all live sessions. Unknown or disconnected environments shall not fall back to host operations. Stale previews, busy races, collisions, inaccessible sources, and authorization failures shall leave the prior workspace intact.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-006.8:** A failed batch shall compensate its new filesystem entries, source records, inventory, and tracking changes. Exact retries shall not duplicate sources or restart processes. Git enclosing a destination shall retain effective staging protection for attached repositories.

## Scope and compatibility

The new layout controls target the Worktree executor. Existing Local and remote source flows remain supported with their current capabilities.
Nested destinations are exactly `./kandev/<entry>/` and `./<entry>/`. They are not `.kandev/`, filesystem-root paths, arbitrary paths, or submodules.
The controls do not promise uniform harness discovery or bypass provider permissions.
The initial-layout and nested-placement portions are implemented in the [placement package](../../../plans/workspace-repository-placement/plan.md). Explicit root expansion remains blocked until the workspace-aware native restore contract in PR #3598 is available.

## Follow-up implementation plan

[Add sources to the current workspace](../../../plans/current-workspace-sources/plan.md) owns REQ-005/006. This follow-up extends repositoryless attachment and Local placement; it retains the remote host-folder restriction. Earlier Worktree-only placement limits describe the preceding package, not a prohibition on this extension.
