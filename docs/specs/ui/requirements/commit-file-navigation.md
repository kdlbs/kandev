---
status: draft
system: ui
created: 2026-09-16
owners:
  - kandev
---

# Commit file navigation requirements

## Overview

Users need to identify and navigate the files in a historical commit before reading each patch.
UI owns this reusable file-navigation presentation contract. Repository identity, commit data, and provider permissions retain their existing owners.

Source: [issue 3718](https://github.com/kdlbs/kandev/issues/3718).

## Requirements

### REQ-UI-COMMIT-FILE-NAV-001: Historical commit file navigation

**Intent:** Give commit readers a file index and independent file collapse controls on desktop and phone.

#### Acceptance criteria

- **AC-UI-COMMIT-FILE-NAV-001.1:** Every returned commit file shall have an accessible expand/collapse control. Files start expanded. Collapsing one file hides only its body and retains its header.
- **AC-UI-COMMIT-FILE-NAV-001.2:** Each file's collapse state shall survive scrolling and unrelated rerenders within the open commit. A different commit, repository, source, or session shall start with independent state. Reopening a closed view need not restore state.
- **AC-UI-COMMIT-FILE-NAV-001.3:** The commit header area shall contain one flat index of all returned files, sorted by path. Each entry shall show its path and supplied additions/deletions, including measured zeroes. Missing statistics shall appear as unavailable, never as fabricated zeroes.
- **AC-UI-COMMIT-FILE-NAV-001.4:** Activating an index entry shall expand its file and scroll its header into the visible detail area. Repeated activation shall work. Other files shall retain their collapse state.
- **AC-UI-COMMIT-FILE-NAV-001.5:** When the task has more than one attached repository, the commit header shall identify the commit's repository. The label shall use the selected commit's identity, including GitHub-only commits, rather than a different active repository.
- **AC-UI-COMMIT-FILE-NAV-001.6:** Binary and empty-patch files shall remain in the index and retain their existing expanded-body treatment. An empty file collection shall retain the existing empty-commit message without an empty index.
- **AC-UI-COMMIT-FILE-NAV-001.7:** Local and GitHub-only commits shall support the same navigation on desktop and phone. Phone users shall reach every control by touch, with targets of at least 44px and no document horizontal overflow. Keyboard users shall activate the controls and identify the destination from focus.
- **AC-UI-COMMIT-FILE-NAV-001.8:** Existing Review behavior, commit display tools, and eligible commit actions shall remain available. Historical file lists shall not gain a reviewed checkbox, review mutations, or worktree actions. GitHub-only commits shall retain their existing read-only restrictions and error/retry behavior.
- **AC-UI-COMMIT-FILE-NAV-001.9:** The commit panel's file index shall have its own expand/collapse control and file count. It starts expanded. Collapsing the index shall leave the count, control, and file diff sections visible. Its state shall remain independent of file-body collapse and survive unrelated rerenders within that view.
- **AC-UI-COMMIT-FILE-NAV-001.10:** Activating a commit row in Changes shall expand or collapse that commit's changed-file list without opening the commit panel. Rows start collapsed and expand independently. The list shall use the saved Changes tree/flat preference on desktop and phone. Tree directories shall collapse independently. Inline files shall retain the same row density, file/status icon sizing, statistics layout, and filename hierarchy as other Changes files, without worktree mutation controls.
- **AC-UI-COMMIT-FILE-NAV-001.11:** Each commit row shall expose a separate, visible, keyboard- and touch-accessible Open commit action. This action shall use an icon without visible text, retain an accessible name, and open the desktop commit panel or phone commit sheet without toggling the inline list. Fine-pointer desktop rows shall retain their pre-expansion compact spacing; phone and coarse-pointer controls shall remain touch-sized.
- **AC-UI-COMMIT-FILE-NAV-001.12:** Activating a file in the inline list shall open its historical commit and reveal that file's expanded diff. The action shall retain the commit source and repository identity. It shall not substitute a cumulative diff or current worktree file.
- **AC-UI-COMMIT-FILE-NAV-001.13:** Inline file data shall load only after expansion. The expanded region shall show loading, empty, or retryable error states as applicable. Reopening a loaded list shall reuse its data within that mounted row. Late responses from a different target shall never populate the row.

## Compatibility

The [merge commit contract](merge-commit-details.md) still owns first-parent data semantics.
The [PR-only commit contract](pr-only-commit-details.md) still owns source routing and remote action restrictions.
This requirement adds presentation behavior without changing either data contract.
Commit-row primary activation intentionally changes from opening details to inline expansion. The new Open commit action replaces the previous navigation gesture.
The commit panel retains a flat index. The inline list in Changes follows its existing tree/flat setting.

## Out of scope

- Persistent collapse preferences, bulk collapse, file filtering, and repository grouping.
- Commit parent selection, provider pagination changes, new endpoints, or eager loading of all commit details.
- Review progress, comments, approval state, and worktree mutation features.

## Implementation plans

- [Commit file navigation](../../../plans/commit-file-navigation/plan.md)
