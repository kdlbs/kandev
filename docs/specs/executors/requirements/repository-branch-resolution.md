---
status: active
system: executors
created: 2026-09-10
owners:
  - kandev
---

# Repository branch resolution

## Overview

Executor preparation must use the upstream branch name when it materializes a repository.
The executor system owns this representation at the script boundary. The task system retains ownership of the selected base reference.

## Requirements

### REQ-EXECUTORS-REPOSITORY-BRANCH-001: Repository branch names

**Intent:** A selected origin reference must materialize the intended branch without a manual change to the task.

#### Acceptance criteria

- **AC-EXECUTORS-REPOSITORY-BRANCH-001.1:** For `origin/main` or `refs/remotes/origin/main`, repository preparation shall use upstream branch `main`.
  The same rule shall preserve nested branch suffixes.
- **AC-EXECUTORS-REPOSITORY-BRANCH-001.2:** Plain branch names shall remain unchanged, including `feature/login` and `upstream/main`.
  An explicit `refs/heads/origin/topic` shall identify literal branch `origin/topic`.
- **AC-EXECUTORS-REPOSITORY-BRANCH-001.3:** Resolution shall preserve the stored task base reference, worktree base reference, and selected task branch.
- **AC-EXECUTORS-REPOSITORY-BRANCH-001.4:** Current and saved executor scripts that use the repository branch placeholder shall receive the same branch name.
  Branch names shall remain literal shell data.
- **AC-EXECUTORS-REPOSITORY-BRANCH-001.5:** An unavailable branch shall fail preparation without a fallback to a different branch.

### REQ-EXECUTORS-REPOSITORY-BRANCH-002: Selected remote checkout

**Intent:** A fresh remote workspace must materialize the selected branch or pull request before its agent starts.

#### Acceptance criteria

- **AC-EXECUTORS-REPOSITORY-BRANCH-002.1:** A fresh remote workspace with an explicit checkout branch shall use that branch instead of a generated task branch.
- **AC-EXECUTORS-REPOSITORY-BRANCH-002.2:** A selected GitHub pull request shall materialize its fetched head, including a head in a fork.
  The comparison base shall remain the selected base branch.
- **AC-EXECUTORS-REPOSITORY-BRANCH-002.3:** Review checkout shall not require permission to push to the head repository or create an editable contribution binding.
- **AC-EXECUTORS-REPOSITORY-BRANCH-002.4:** Failure to fetch or check out the explicit selection shall fail preparation before agent startup.
  Preparation shall not substitute the base branch or a generated branch.
- **AC-EXECUTORS-REPOSITORY-BRANCH-002.5:** Tasks without an explicit selection shall retain generated-branch behavior.
  Resuming an existing workspace shall preserve its commits, uncommitted changes, and branch without a forced reset.

## Exclusions

- Discovery or selection of remotes other than the existing clone source, `origin`.
- Changes to branch pickers, task recovery controls, or persisted branch values.
- Automatic retries, deployments, and changes to existing remote workspaces.

## System design

- [Repository branch resolution](../system-design/repository-branch-resolution.md)
