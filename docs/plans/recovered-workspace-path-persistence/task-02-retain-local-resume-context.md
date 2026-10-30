---
id: "02-retain-local-resume-context"
title: "Retain LOCAL resume context"
status: done
wave: 2
depends_on:
  - "01-preserve-recovered-workspace-path"
plan: "plan.md"
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002
acceptance_criteria:
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002.1
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002.2
system_design:
  - ../../specs/tasks/system-design/additional-session-workspace-reuse.md
---

# Task 02: Retain LOCAL resume context

## Scope

Port the existing detached/inherit-parent LOCAL and `local_pc` resume adapter
from the verified local source. Keep the current owned task workspace instead
of recreating its historical LOCAL repository. Cold WorkspaceInfo uses the
same validated direct-parent context. No new AC identifier is allocated.

## Files likely touched

- `apps/backend/internal/orchestrator/executor/resume_detached_local_workspace.go`
- `apps/backend/internal/orchestrator/executor/resume_inherited_local_workspace.go`
- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/task/service/workspace_info_inherited_local.go`
- `apps/backend/internal/agent/runtime/runtime.go` (existing contract alias only)
- `apps/backend/internal/task/service/service_turns.go`

## Preserved boundaries

The detached case requires no current task attachments and the exact historical
LOCAL session/repository and task-owned ready/stopped environment. The inherited
case additionally retains direct-parent, same-workspace, nonarchived-parent,
one-source/deleted-inventory and no-materialized-worktree/path/clone/common-dir
conditions. Normal repository resolution remains the fallback. Attachments,
preferences, history, environment ownership, current permission and native
recovery proof/claim/unknown-outcome semantics remain unchanged.

Factory extraction, provider usage, automatic recovery, continued-history
ancestry, setup replay, operating data repair and Windows process ownership are
outside this work order. No public API/schema/flag/worker is added.

## Verification selection

Root runs only the affected existing executor workspace/inherited-owner and
task WorkspaceInfo controls plus disposable overlays outside automatic test
collection. Baseline must reproduce a historical repository materialization request
or inappropriate cold source projection; candidate positives cover detached and
inherited LOCAL/`local_pc`. Negative cases cover foreign/archived parent,
workspace mismatch, worktree, multiple repositories, attached current source
and physical inventory evidence. Verify preferences/attachments/inventory are
unchanged and no provider or native recovery action is performed.

## Results

The original-source overlay reproduced detached source rematerialization,
inherited inventory refusal, and inappropriate cold source projection for both
LOCAL spellings. The ambiguous-inventory fixture was corrected to use a distinct
legal branch-slot key; no product assertion was weakened. The port exposes the existing lifecycle WorkspaceInfo type through the approved
runtime facade and binds the helper there; the contract and fields are unchanged.

The six disposable positive/negative groups passed with the race detector:
executor1.891s and task/service1.952s after the facade binding. They call existing resume settings and the
real SQLite WorkspaceInfo projection and retain session/attachment history.
Existing resume configuration/environment controls passed (executor7.463s).
The existing WorkspaceInfo controls have one unrelated Windows-only path
assertion failure: MultiRepoReturnsTaskRoot expects POSIX separators; the
unchanged original-source overlay fails identically (0.602s). Full package
success is not claimed. No existing test or path normalization was changed.

Formatting and the existing architecture check completed successfully.
Documentation coverage, normal hooks and hosted CI are
separate publication results. No installed provider operation or operating data repair was performed; earlier Task 01 results remain historical evidence.

## Committed regression coverage

Existing inherited-environment and workspace-info test files retain the six
scenario groups, covering local/local_pc, detached and inherited ownership,
physical-inventory exclusions and unchanged history. No production test seam,
framework or scheduled job is added. The persisted cases passed directly with
fts5 and race detection: executor3.116s and final task/service4.637s.

## Resume-request entry-point correction

The original helper fixture pre-filled WorkspacePath, whereas
newResumeLaunchRequest initializes only OriginalWorkspacePath. Without a
native conversation ID, lifecycle does not consult the workspace-info provider
and can select a scratch folder.

The exact published-source overlay failed the production-shaped request
matrix with an empty WorkspacePath (4.295s). The fixture no longer pre-fills
that field, and the native positive control uses the canonical persisted
DownstreamACPSessionID. A seventh existing-file group calls buildResumeRequest
for detached/inherited, local/local_pc and workspace-only/no-native/native
conversation modes.

Both validated exceptions now bind WorkspacePath from the retained environment.
OriginalWorkspacePath, ownership checks, session repository history and native
conversation selection remain unchanged. The final request cases, prior
positive/exclusion groups and selected existing environment/repository controls
passed together with fts5 and race detection (4.860s). The service projection
source is unchanged from its 4.637s validation above. Normal hooks and updated
head CI are separate publication results.
