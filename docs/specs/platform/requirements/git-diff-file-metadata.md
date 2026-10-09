---
status: active
system: platform
created: 2026-10-02
updated: 2026-10-09
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
Comparison data must also remain usable when the user's Git configuration
forces terminal display color. Display decoration is not patch content.
Cumulative comparisons must retain built-in patch data when a user's Git setup
selects an external diff helper.
Commit and cumulative comparisons must also retain actual file bytes when
repository attributes select a text converter, even if its output hides changes.
Submodule display preferences must not hide or replace a parent repository's
gitlink change in these comparisons.
Submodule ignore preferences must also retain a recorded parent gitlink change,
including when the child is unavailable and the parent is the only review evidence.
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
- **AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.6:** With default color settings, forced `color.ui=always`, or an overriding `color.diff=always`, commit and cumulative comparisons shall return the same file membership, paths, statuses, counts, metadata, and plain patch data as color-disabled comparisons. A nonempty comparison shall not become a successful empty result because of display color. Existing genuinely empty results and output budgets shall retain their behavior.
- **AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.7:** Suppressing display color shall preserve literal escape bytes in source content and filenames and shall not change repository configuration, HEAD, refs, index, or worktree state. Repository-selected and aggregate comparisons shall retain their existing routing and repository identities.
- **AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.8:** With `diff.external`, `GIT_EXTERNAL_DIFF`, or both selecting external diff helpers, every cumulative comparison shall return the same built-in patch data, exact file membership, paths, statuses, counts, and metadata as a comparison without those helpers. This applies to committed and dirty tracked changes, genuinely empty comparisons, and repository-selected and aggregate reads; nonempty comparisons shall not become successful empty results because of helper output. Existing byte and file budgets, truncation counts, and skip reasons shall retain their behavior.
- **AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.9:** Cumulative comparison reads shall not execute configured or environment-selected external diff helpers and shall not change repository configuration, HEAD, refs, index, worktree state, or the caller's helper environment settings.
- **AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.10:** With repository attributes selecting a configured text converter, every commit and cumulative comparison shall return the same built-in patch data from actual file bytes, exact file membership, paths, statuses, line counts, and metadata as a comparison without that converter. This applies when converter output changes or suppresses the patch, to committed and dirty tracked changes, binary and empty-file changes, genuinely empty comparisons, and repository-selected and aggregate reads. Existing comparison bases, byte and file limits, truncation counts, and skip reasons shall retain their behavior.
- **AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.11:** Commit and cumulative comparison reads shall not execute selected text converters and shall not change repository configuration or attributes, HEAD, refs, index, or worktree content/state. Configured converters remain available to other Git operations under their existing contracts.
- **AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.12:** With default, short, log, or inline-diff submodule display preferences, commit and cumulative comparisons shall retain each changed parent gitlink's exact path, short gitlink patch with the old/new commit identities, status, and line counts. Forward and backward pointer updates, additions, and deletions shall remain inspectable; a gitlink-only commit shall not appear to have no changed files because of a display preference. Ordinary file changes, genuinely empty comparisons, and existing metadata and budgets shall retain their behavior.
- **AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.13:** Normalizing parent gitlink display shall preserve repository-selected isolation, aggregate repository-qualified identities, and separate initialized child comparisons against their parent-recorded anchors. It shall not mutate configuration, HEAD, refs, index, or worktree state, change comparison bases or API shapes, or alter existing Review suppression of a parent gitlink row when child file diffs are available.
- **AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.14:** With default, none, untracked, dirty, or all submodule ignore preferences, commit and cumulative comparisons shall retain an actual recorded parent gitlink pointer change with its exact path, old/new commit identities, status, patch and line counts. An unavailable or deinitialized child shall not cause that change to become a successful empty comparison; the parent evidence shall remain available to the existing Review fallback. Cumulative parent gitlink evidence shall represent an actual pointer change, without a `-dirty` decoration from child-only tracked or untracked content when the child HEAD is unchanged. Actual initialized child file changes shall remain visible in their existing separate scopes against parent-recorded anchors. Ordinary files and genuinely unchanged parent comparisons shall retain their behavior.

## Cross-surface outcome

Desktop and phone consume the same corrected metadata through their existing
commit and Review surfaces. No new layout, interaction, copy, or navigation is
required. Provider-only history remains governed by its existing source contract.

## Out of scope

- Git invocation or flag-validation changes beyond per-comparison display-color suppression, built-in cumulative patch selection, text-conversion suppression, short parent gitlink display and parent gitlink visibility for commit and cumulative patches; shared environment policy, history-provider, rename-detection, other readers' display or text conversion, or comparison-base changes.
- Live workspace porcelain parsing, NUL numstat parsing, literal path selection, and worktree mutations.
- Frontend layout, file-navigation changes, new status enums, or transport schemas.

## Design and delivery

- [Git diff file metadata design](../system-design/git-diff-file-metadata.md)
- [Metadata status repair package](../../../plans/git-diff-status-metadata/plan.md)
- [Plain comparison output repair package](../../../plans/git-comparison-plain-output/plan.md)
- [Built-in cumulative patch repair package](../../../plans/git-cumulative-built-in-patch/plan.md)
- [Actual-byte comparison patch repair package](../../../plans/git-comparison-textconv/plan.md)
- [Submodule comparison format repair package](../../../plans/git-submodule-comparison-format/plan.md)
- [Ignored submodule review evidence repair package](../../../plans/git-submodule-ignore-review-evidence/plan.md)
