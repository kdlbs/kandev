---
status: active
system: ui
created: 2026-07-14
owners:
  - kandev
---
# Review File Status Cues Requirements

## Overview

Reviewers can see changed filenames in Review's file tree, but cannot tell whether a file was added, modified, deleted, or moved without opening its diff. This slows navigation and makes patchless changes, especially pure renames, easy to miss.

## Requirements

### REQ-UI-REVIEW-FILE-STATUS-001: Review File Status Cues

**Intent:** Reviewers can see changed filenames in Review's file tree, but cannot tell whether a file was added, modified, deleted, or moved without opening its diff. This slows navigation and makes patchless changes, especially pure renames, easy to miss.

#### Acceptance criteria

- **AC-UI-REVIEW-FILE-STATUS-001.1:** Every changed file in Review shows a persistent trailing status marker: green plus for added, amber dot for modified, rose minus for deleted, and purple arrow for moved/renamed.
- **AC-UI-REVIEW-FILE-STATUS-001.2:** Untracked files use the added visual language while retaining the accessible label `Untracked`.
- **AC-UI-REVIEW-FILE-STATUS-001.3:** The marker does not rely on color alone. Its accessible name and native title describe the status; a moved file may include its previous path when available.
- **AC-UI-REVIEW-FILE-STATUS-001.4:** The desktop marker occupies a fixed trailing column. At narrow sidebar widths, the filename truncates before the marker, stale warning, or comment count overlap.
- **AC-UI-REVIEW-FILE-STATUS-001.5:** Below the desktop breakpoint, where the file tree is hidden, the same status marker appears in each sticky diff header. No required cue depends on hover.
- **AC-UI-REVIEW-FILE-STATUS-001.6:** Desktop and phone sticky diff headers render ordinary left-to-right repository paths without relocating punctuation. A dot-prefixed directory segment keeps its dot attached to that segment instead of moving beside a separator or filename.
- **AC-UI-REVIEW-FILE-STATUS-001.7:** When a sticky diff header cannot show all directory context, it elides the leading directory text while keeping the nearest directory suffix and filename visible. Truncation does not change the full path exposed by the header title or accessible collapse/expand label.
- **AC-UI-REVIEW-FILE-STATUS-001.8:** Review includes changed files even when they have no textual patch, including pure renames. Their diff body shows status-specific explanatory text instead of an indefinite loading message.

## Migrated source detail

## Why

Reviewers can see changed filenames in Review's file tree, but cannot tell whether a file was added, modified, deleted, or moved without opening its diff. This slows navigation and makes patchless changes, especially pure renames, easy to miss.

## What

- Every changed file in Review shows a persistent trailing status marker: green plus for added, amber dot for modified, rose minus for deleted, and purple arrow for moved/renamed.
- Untracked files use the added visual language while retaining the accessible label `Untracked`.
- The marker does not rely on color alone. Its accessible name and native title describe the status; a moved file may include its previous path when available.
- The desktop marker occupies a fixed trailing column. At narrow sidebar widths, the filename truncates before the marker, stale warning, or comment count overlap.
- Below the desktop breakpoint, where the file tree is hidden, the same status marker appears in each sticky diff header. No required cue depends on hover.
- Desktop and phone sticky diff headers render ordinary left-to-right repository paths without relocating punctuation. A dot-prefixed directory segment keeps its dot attached to that segment instead of moving beside a separator or filename.
- When a sticky diff header cannot show all directory context, it elides the leading directory text while keeping the nearest directory suffix and filename visible. Truncation does not change the full path exposed by the header title or accessible collapse/expand label.
- Review includes changed files even when they have no textual patch, including pure renames. Their diff body shows status-specific explanatory text instead of an indefinite loading message.
- Existing explicit skip reasons for binary, oversized, truncated, or budget-exceeded diffs take precedence over generic status-specific text.
- Existing file icons, tree hierarchy, filtering, review checkboxes, stale indicators, comment counts, source precedence, and multi-repository identity remain unchanged.

## Scenarios

- **GIVEN** added, modified, deleted, and renamed files, **WHEN** Review opens, **THEN** its desktop tree shows four distinct markers aligned at the trailing edge of their rows.
- **GIVEN** an untracked file, **WHEN** Review opens, **THEN** it uses the added plus marker and its accessible name says `Untracked`.
- **GIVEN** a pure rename with no textual patch, **WHEN** Review opens, **THEN** the file remains reviewable, uses the moved marker, and its body explains that the move has no textual changes.
- **GIVEN** another changed file with no textual patch, **WHEN** its diff body renders, **THEN** Review shows status-appropriate empty-state text rather than `Loading diff...`.
- **GIVEN** a file with an explicit diff skip reason, **WHEN** its body renders, **THEN** the skip-reason message is shown instead of the generic status empty state.
- **GIVEN** a 160 px Review sidebar and a long filename, **WHEN** the row renders, **THEN** the filename truncates while the status marker, stale warning, and comment count remain visible without overlap.
- **GIVEN** a mobile viewport, **WHEN** the user reviews a changed file, **THEN** its sticky diff header exposes the status marker without horizontal page overflow.
- **GIVEN** the changed path `.agents/skills/pr-fixup/SKILL.md`, **WHEN** its desktop sticky diff header renders, **THEN** `.agents` remains the first directory segment, `SKILL.md` remains the filename, and no dot is rendered beside another segment or separator.
- **GIVEN** a long changed path with a dot-prefixed directory in a phone viewport, **WHEN** its sticky diff header truncates the directory, **THEN** the visible directory suffix and filename retain their original punctuation order without horizontal page overflow.

## Out of scope

- Grouping or filtering Review files by status.
- Replacing compact markers with visible `A`/`M`/`D`/`R` badges or filename tinting.
- A new mobile file navigator.
- Status animations.
- Backend or API changes; Review consumes existing git and pull-request status metadata.
- Changing file-path data, sorting, copying, or Git operations.
- Defining a new visual-order policy for paths containing strong right-to-left Unicode characters.
