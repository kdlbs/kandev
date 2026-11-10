---
status: draft
system: workspaces
created: 2026-10-28
owners:
  - kandev
---

# Root commit revert availability requirements

## Overview

The Changes timeline offers local Git operations on a repository's commits. Workspaces owns the Git state and operation contract; the timeline presents that contract on desktop and phone.

## Requirements

### REQ-WORKSPACES-ROOT-REVERT-001: Offer revert only when a known parent exists

**Intent:** Avoid offering an undo operation that cannot move the current branch to a parent commit, while preserving existing actions when parent information is unavailable.

#### Acceptance criteria

- **AC-WORKSPACES-ROOT-REVERT-001.1:** When a latest local commit is known to have no parent, the Changes timeline shall omit Revert commit from its context and row actions on desktop, coarse-pointer devices, and phone. Other eligible commit actions remain available.
- **AC-WORKSPACES-ROOT-REVERT-001.2:** A latest eligible local commit with a known parent shall retain Revert commit, and activating it shall target that commit and its repository. Missing parent metadata shall retain the prior action eligibility rather than be treated as proof of a root commit.
- **AC-WORKSPACES-ROOT-REVERT-001.3:** Existing older-commit, pushed-commit, and provider-only mutation restrictions shall remain intact in unified and separate histories. Amend and Reset shall retain their existing eligibility and behavior.

## Out of scope

- Changing Git history semantics, removing a branch or ref, or adding an operation to undo a root commit.
- Changing provider history, permissions, transport, or localized copy.

## Implementation plans

- [Root commit revert availability](../../../plans/root-commit-revert-availability/plan.md)
