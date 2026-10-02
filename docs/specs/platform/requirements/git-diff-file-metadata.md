---
status: active
system: platform
created: 2026-10-02
owners:
  - kandev
---

# Git diff file metadata requirements

## Overview

Commit readers and cumulative Review consumers need file statuses that describe
the actual Git change. Platform owns this shared source-data contract because
both comparison types expose the same file metadata. UI owns navigation and
presentation, rather than deriving change identity from filenames or content.

The active [merge-detail contract](../../ui/requirements/merge-commit-details.md)
already requires status preservation in `.2` and ordinary/root behavior in `.4`.
This contract makes classification explicit for both commit and cumulative
comparisons without extending merge-only behavior to another comparison type.
The [workspace status contract](workspace-git-status.md) separately owns live
porcelain membership, staged/unstaged facets, and detail enrichment.

## Requirements

### REQ-PLATFORM-GIT-DIFF-FILE-METADATA-001: Faithful comparison file status

**Intent:** Every returned comparison file identifies its Git change independently
of arbitrary path and content bytes.

#### Acceptance criteria

- **AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.1:** For every returned commit or cumulative comparison file, an ordinary modification shall remain `modified` when its filename, added lines, removed lines, unchanged context, or hunk description contains `new file mode`, `deleted file mode`, or `rename from`. Leading content whitespace shall not change classification.
- **AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.2:** Genuine added, deleted, and renamed changes shall retain `added`, `deleted`, and `renamed` respectively, including empty files, binary files, and renames with or without content changes. Misleading marker text in their paths or contents shall not override the actual change. Ordinary mode-only and binary modifications shall remain `modified`.
- **AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.3:** Commit and cumulative responses shall preserve each exact file key, path, patch bytes, line counts, and existing response fields while correcting status. Repository-selected reads shall return only the selected repository; aggregate reads shall retain distinct repository-qualified file identities and repository metadata for identical paths.
- **AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.4:** Status classification shall remain correct when emitted patch content is truncated or skipped by existing budgets. Single-commit detail remains uncapped, and cumulative byte limits, file limits, truncation counts, and skip reasons shall retain their behavior.
- **AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.5:** Ordinary, first-parent merge, root, and empty commit comparisons shall retain their existing bases and file membership. Correcting status shall not mutate the checkout or index, change repository selection, or introduce new public fields or status values.

## Cross-surface outcome

Desktop and phone consume the same corrected metadata through their existing
commit and Review surfaces. No new layout, interaction, copy, or navigation is
required. Provider-only history remains governed by its existing source contract.

## Out of scope

- Git invocation, environment, history-provider, rename-detection, or comparison-base changes.
- Live workspace porcelain parsing, NUL numstat parsing, literal path selection, and worktree mutations.
- Frontend layout, file-navigation changes, new status enums, or transport schemas.

## Design and delivery

- [Git diff file metadata design](../system-design/git-diff-file-metadata.md)
- [Metadata status repair package](../../../plans/git-diff-status-metadata/plan.md)
