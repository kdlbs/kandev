---
spec: docs/specs/workspaces/requirements/create-local-repository.md
created: 2026-07-30
status: completed
---

# Implementation Plan: Descriptor-Bound Local Repository Initialization

## Overview

`git init /dev/fd/3` cannot enter an inherited directory descriptor on macOS because fdescfs
presents the descriptor without directory search permission. PR #2052 avoids that failure by
setting `exec.Cmd.Dir` to the staging pathname, but doing so lets a pathname replacement redirect
Git before the post-initialization identity check. Replace the platform-specific descriptor path
with a small inherited-fd helper process so Linux and macOS initialize the verified directory
identity without changing the backend process's working directory.

## Backend

### Inherited-descriptor Git helper

- Add `apps/backend/internal/task/gitinit/` with a narrow command boundary for local repository
  initialization.
- On Linux and macOS, start the current executable with a private helper argument, an explicit
  internal environment marker, and the verified directory inherited as fd 3. Package initialization
  intercepts only that marked subprocess before any application or test entry point. The helper
  calls `fchdir(3)` and replaces itself with `git init --initial-branch=main`, preserving context
  cancellation because Git keeps the helper process identity.
- Windows retains its existing pathname/current-directory behavior. Other compiled Unix targets
  keep a compatibility fallback, but remain outside the supported product scope because they do not
  implement the required ownership and exclusive-publication guarantees.
- Keep the helper out of public `cmd/kandev` dispatch and CLI help.

### Task service

- Update `initializeGitRepository` in
  `apps/backend/internal/task/service/local_repository_initialization.go` to request the command
  from the helper boundary and remove the `/proc/self/fd` versus `/dev/fd` runtime path selection.
- Preserve combined-output error reporting, unborn `main`, cancellation, post-initialization
  identity validation, exclusive publication, and fail-closed cleanup.
- Correct implementation comments so they describe supported Linux/macOS behavior without claiming
  that the user-facing feature works on BSD.

## Tests

- **Descriptor identity after pathname replacement**
  - **Files:** `apps/backend/internal/task/gitinit/command_descriptor_test.go`,
    `apps/backend/internal/task/service/local_repository_initialization_descriptor_test.go`
  - **How:** open staging directory A, rename it, install replacement B at the old pathname, invoke
    the real helper subprocess, and assert `.git` is created only in A while B remains empty.
- **Helper dispatch and failure propagation**
  - **Files:** `apps/backend/internal/task/gitinit/*_test.go`,
    `apps/backend/internal/task/handlers/repository_handlers_test.go`
  - **How:** use a helper-process test to exercise inherited fd 3, verify hidden dispatch, and assert
    invalid descriptors or missing Git return a non-zero result with useful stderr. Exercise the
    HTTP handler from its own test binary to prove dispatch is not tied to a package-specific
    `TestMain`.
- **Existing initialization behavior**
  - **File:** `apps/backend/internal/task/service/local_repository_initialization_test.go`
  - **How:** retain real-Git coverage for the commitless unborn `main` repository, persistence
    rollback, and pathname canonicalization.

## Implementation Waves

Wave 1:

- [x] [task-01-descriptor-bound-git-init](task-01-descriptor-bound-git-init.md) - completed

The repair is one sequential task because the helper command, process-init trampoline, service
wiring, and identity regression test form one process boundary and touch shared behavior.

## Risks

- The helper must never change the long-running backend's process-wide working directory.
- The helper must activate only for a subprocess carrying both the private argument and internal
  environment marker, regardless of which application or test binary links the package.
- `fchdir` and descriptor inheritance are supported on Linux and macOS; Windows must keep its
  existing non-`ExtraFiles` path.
- A replaced request-owned inode may be impossible to clean up after an attacker moves it, but the
  replacement path must remain untouched and no repository row may be persisted.
