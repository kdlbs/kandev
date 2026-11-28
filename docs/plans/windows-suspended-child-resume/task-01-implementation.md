---
id: "01-implementation"
title: "fix(winproc): resume suspended children through their process handle"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-WINDOWS-SUSPENDED-RESUME-001
acceptance_criteria:
  - AC-PLATFORM-WINDOWS-SUSPENDED-RESUME-001.1
  - AC-PLATFORM-WINDOWS-SUSPENDED-RESUME-001.2
  - AC-PLATFORM-WINDOWS-SUSPENDED-RESUME-001.3
system_design:
  - ../../specs/platform/system-design/windows-suspended-child-resume.md
---

# fix(winproc): resume suspended children through their process handle

## Outcome and scope

Resume the exact suspended Windows child through its process handle with work independent of the system thread count.

Out of scope: Job ownership changes, force termination, and changes to non-Windows launch behavior.

## Implementation acceptance

1. The affected native entrypoint follows the linked requirement and design while preserving unrelated behavior and history.
2. Rejected, stale and failed operations retain their existing refusal or rollback semantics.
3. The focused implementation checks and linked documentation coverage have honest recorded results; unexecuted follow-up checks remain pending.

## Files and dependencies

The source files are the existing PR #4716 diff. This work order adds its delivery record and links existing requirements and system design; it introduces no runtime gate or permanent test.

- [System design](../../specs/platform/system-design/windows-suspended-child-resume.md)
- [Plan](plan.md)

## Verification and results

Validation:
- Four original in-process checks passed through a temporary Go overlay: one process handle, propagated resume failure, unopenable process, and export availability.
- A temporary entry-point check created a hidden `CREATE_SUSPENDED` child, assigned it through `InstallKillOnCloseJobForSuspendedCommand`, and observed normal exit.
- All normal commit hooks passed. The new worktree's existing declared commitlint dependency was installed from its unchanged lockfile before the successful commit.
- Temporary checks were removed; no permanent tests or CI jobs were added.

Documentation-only correction: git diff --check and the existing .github/scripts/pr-docs.cjs validateCoverage entrypoint with the candidate changed paths and the linked candidate documents supplied on stdin. No GitHub status is synthesized locally.
