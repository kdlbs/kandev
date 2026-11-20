---
status: active
system: tasks
created: 2026-11-18
owners:
  - kandev
---

# Workspace Root Files Requirements

## Overview

A multi-repository task places each Git checkout in a shared task workspace
directory. Agents sometimes write files into that shared directory, next to the
checkouts. These files exist on disk and can be opened from chat, but no
repository reports them, so **Changes** omits them. The user then has no
place to find agent output that does not belong to a checkout yet.

This capability shows those files in Changes. The user can open them. The
user can also move a file into a chosen checkout, where the existing Git
actions take over. The tasks system owns the contract because it owns the
multi-repository task workspace layout ([multi-branch tasks](multi-branch.md)).
Per-repository Git status remains owned by
[workspace Git status](../../platform/requirements/workspace-git-status.md).

Source: [kdlbs/kandev#5372](https://github.com/kdlbs/kandev/issues/5372).

## Terminology

- **Task workspace root:** The directory that contains the checkouts of a
  multi-repository task. It is not itself a Git repository.
- **Checkout directory:** A directory under the task workspace root that the
  workspace tracks as a repository scope, plus any other directory under the
  root that contains a `.git` entry.
- **Workspace root file:** A regular file under the task workspace root that is
  not inside a checkout directory and is not excluded by
  `AC-TASKS-WORKSPACE-ROOT-FILES-001.3`.

## Requirements

### REQ-TASKS-WORKSPACE-ROOT-FILES-001: Show workspace root files in Changes

**Intent:** Make agent output that lives outside every checkout visible where
users already review task changes.

**User story:** As a developer running an agent across several checkouts, I
want files created next to the checkouts to appear in Changes, so that I can
find them and decide later whether they belong in a commit.

#### Acceptance criteria

- **AC-TASKS-WORKSPACE-ROOT-FILES-001.1:** When a multi-repository task
  workspace root contains at least one workspace root file, desktop and phone
  Changes shall show an **Outside repositories** section that lists each file
  by its root-relative path. The section shall show this list even when every
  checkout is clean.
- **AC-TASKS-WORKSPACE-ROOT-FILES-001.2:** When a workspace root file is
  created, modified, renamed, or deleted, an open Changes view shall show the
  updated list without a page reload. The update latency shall be the same as
  for per-repository status polling.
- **AC-TASKS-WORKSPACE-ROOT-FILES-001.3:** The list shall exclude checkout
  directories, directories whose name starts with `.`, `node_modules`
  directories, Kandev's workspace ownership marker, recovery-artifact
  exclusions, and symbolic links. Discovery shall not follow a symbolic link.
- **AC-TASKS-WORKSPACE-ROOT-FILES-001.4:** The list shall return at most 500
  files. When more eligible files exist, Changes shall say that the list is
  truncated.
- **AC-TASKS-WORKSPACE-ROOT-FILES-001.5:** When the task workspace is a single
  checkout or a Git repository, Changes shall not show the section. Its
  existing content and empty state shall stay the same.

### REQ-TASKS-WORKSPACE-ROOT-FILES-002: Open workspace root files

**Intent:** Let users inspect a workspace root file without affecting Git.

#### Acceptance criteria

- **AC-TASKS-WORKSPACE-ROOT-FILES-002.1:** When the user activates a workspace
  root file in Changes, the system shall open it in the existing file viewer or
  editor at its root-relative path. This is the same viewer that chat's
  **Open file** action uses.
- **AC-TASKS-WORKSPACE-ROOT-FILES-002.2:** Listing or opening workspace root
  files shall not run a Git command that writes. It shall not change any
  checkout's working tree, index, or refs.

### REQ-TASKS-WORKSPACE-ROOT-FILES-003: Move a workspace root file into a checkout

**Intent:** Let the user choose an explicit destination checkout so that
existing Stage, Commit, and Push actions can handle the file.

#### Acceptance criteria

- **AC-TASKS-WORKSPACE-ROOT-FILES-003.1:** A workspace root file shall offer a
  **Move to repository** action. The action shall list the task's checkout
  directories and default to none. It shall move the file only after the user
  selects a checkout and confirms.
- **AC-TASKS-WORKSPACE-ROOT-FILES-003.2:** After a confirmed move, the file
  shall be at `<checkout>/<root-relative path>`. It shall leave the Outside
  repositories section and appear as an untracked file in that checkout's
  Unstaged group. The system shall not stage, commit, or push the file.
- **AC-TASKS-WORKSPACE-ROOT-FILES-003.3:** When the destination path already
  exists, the move shall fail without overwriting anything. Changes shall show a
  localized error, and the source file shall stay unchanged.

## Out of scope

- Copying, rather than moving, a file into a checkout.
- Directory-level moves, bulk moves, and deleting workspace root files from
  Changes. The Files panel continues to own general file management.
- Diffs for workspace root files. They have no Git base.
- Single-repository tasks and Git-repository task roots. A Git root already
  reports such files as untracked paths.
- Changes to per-repository Git status, staging, commit, or push semantics.
