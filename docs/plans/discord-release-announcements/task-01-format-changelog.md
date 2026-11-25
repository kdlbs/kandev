---
id: "01-format-changelog"
title: "Format grouped attributed changelogs"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-RELEASE-DISCORD-002
acceptance_criteria:
  - AC-RELEASE-DISCORD-002.1
  - AC-RELEASE-DISCORD-002.2
  - AC-RELEASE-DISCORD-002.3
  - AC-RELEASE-DISCORD-002.4
  - AC-RELEASE-DISCORD-002.5
  - AC-RELEASE-DISCORD-002.6
  - AC-RELEASE-DISCORD-002.7
system_design:
  - ../../specs/release/system-design/discord-announcements.md
---

# Task 01: Format grouped attributed changelogs

## Summary

Create a pure formatter for release sections and resolved contributor records.
Count the complete selected release, then produce one compact announcement with attributed highlights.

## In scope

Section parsing, GitHub credit links, unavailable-credit labels, emojis, Unicode limits, whole-release counting and bounded selection, and mention suppression.
Include synthetic fixtures resembling the current changelog, with maintainer entries, multiple PR links, missing authors, and publication warnings.

## Out of scope

Network calls, workflow edits, live messages, and changes to historical changelogs.

## Acceptance

1. Grouped entries preserve their source text, PR links, credits, and the exact GitHub release destination. Each heading shows the category name and total entry count without a PR-type prefix. Counts include entries omitted from excerpts.
2. Whole-release category counts and their sum remain unchanged after selection or trimming. The message distinguishes displayed highlights from totals and has no attachment or additional-message fallback.
3. The message uses category emojis and omits the rejected copy. Escaping and mention settings prevent Discord mentions.

## Verification

Run from the repository root after implementing the new test file:

```bash
python3 .github/scripts/discord-release-content_test.py
git diff --check
```

## Files likely touched

- `.github/scripts/discord-release-content.py` (new)
- `.github/scripts/discord-release-content_test.py` (new)

## Dependencies

None.

## Risks

Markdown parsing must preserve multiline entries and distinguish publication warnings from changelog categories.
The existing notes intentionally omit maintainer credits, so the caller must supply resolved author data.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/release/requirements/discord-announcements.md), REQ-RELEASE-DISCORD-002.
- [Design](../../specs/release/system-design/discord-announcements.md), Entry selection and Presentation.
- `cliff.toml`, `CHANGELOG.md`, and `.github/AGENTS.md`.

## Results

Red: eight formatter tests failed because the formatter did not exist.
Green: `python3 .github/scripts/discord-release-content_test.py` passed all eight tests.
`git diff --check` passed.
