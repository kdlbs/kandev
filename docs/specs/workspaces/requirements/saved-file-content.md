---
status: active
system: workspaces
created: 2026-10-06
owners:
  - kandev
---

# Saved File Content Requirements

## Overview and ownership

A successful file save must leave the submitted edit on disk. Overlapping saves
to different files must not consume one another's edits or acknowledge content
that was never saved. Private save preparation must not enter the user's Git
index when staging overlaps a pending save.

Workspaces owns filesystem mutation authority and the resulting file contents.
The [UI editor contract](../../ui/requirements/file-editor-mutation-ownership.md)
owns which editor may publish a save reply and advance its saved baseline. It
depends on a truthful disk-save result; editor lifetime checks cannot repair
incorrect file contents behind a successful response.

## Terms

- **Distinct files:** Different resolved filesystem targets in one workspace.
  Two aliases of the same target are not distinct files.
- **Eligible save:** A valid patch for the requested file, with a matching
  original-content hash and desired content consistent with that patch.
- **Save result:** Success or failure, the saved-content SHA256 hash on success,
  and the existing applied or overwritten resolution.

## Requirements

### REQ-WORKSPACES-SAVED-FILE-CONTENT-001: Independent file saves

**Intent:** Preserve every accepted edit when saves to distinct files overlap.

#### Acceptance criteria

- **AC-WORKSPACES-SAVED-FILE-CONTENT-001.1:** When two eligible saves to distinct
  files overlap, with no other writer to either target, both shall succeed and
  each file shall contain its own submitted content after both saves settle.
  This shall hold when the saves originate through separate consumers of the
  same workspace. Sequential saves shall produce the same per-file results.
- **AC-WORKSPACES-SAVED-FILE-CONTENT-001.2:** Each successful save shall return
  the SHA256 of the content it actually saved to its requested target, with the
  corresponding existing resolution. The registered file-update endpoint shall
  preserve this result in its success response. A save shall not change bytes
  in unrelated neighboring files.
- **AC-WORKSPACES-SAVED-FILE-CONTENT-001.3:** A stale original hash or an
  unappliable patch shall retain the existing desired-content overwrite fallback
  when supplied, including an explicitly empty desired value. Without that
  fallback, the rejected save shall leave the target unchanged and report
  failure rather than a successful saved-content hash.
- **AC-WORKSPACES-SAVED-FILE-CONTENT-001.4:** Cancellation or deadline expiry
  while a save waits to execute its patch shall report failure without writing
  its desired-content fallback. Other distinct-file saves shall remain able to
  complete independently.
- **AC-WORKSPACES-SAVED-FILE-CONTENT-001.5:** When Stage All overlaps an eligible
  pending save, no private transient save content shall become a file in the
  user's working tree or an entry in the real Git index. Successful save results
  shall still satisfy `.2`. This isolation shall also hold for sequential saves,
  failed saves, overwrite fallback and queued cancellation. Staging shall retain
  its existing selection behavior, including genuine user dotfiles, additions
  and deletions, as defined by
  [the staging contract](../../platform/requirements/workspace-git-status.md).
  Overlap does not promise that Stage All includes an edit that has not executed
  yet, or that the two operations form one atomic transaction.

## Exclusions

- Ordering, merging, retrying, or deduplicating concurrent saves to one target.
- Rollback of writes already executed before cancellation; crash durability.
- Expanding which files a patch may target or changing path authorization.
- Editor lifetime/publication rules, UI composition, transports, schemas,
  configuration, and database persistence.

## Design and delivery

- [Saved file content design](../system-design/saved-file-content.md)
- [Prevent overlapping file saves](../../../plans/prevent-overlapping-file-saves/plan.md)
- [Correct repository save targets](../../../plans/repository-save-target/plan.md)
- [Keep private save patches out of Git](../../../plans/private-save-patches/plan.md)
