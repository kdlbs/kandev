---
id: "02-child-summary-line"
title: "Render the child summary line"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-WAKE-CHILD-SUMMARIES-001
  - REQ-OFFICE-WAKE-CHILD-SUMMARIES-003
acceptance_criteria:
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-001.2
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-001.3
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-001.4
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-001.5
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-001.6
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-001.7
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-001.8
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-001.9
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-001.10
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-001.11
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-001.12
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-003.7
system_design:
  - ../../specs/office/system-design/children-completed-wake-summaries.md
---

# Task 02: Render the child summary line

## Summary

Give `ChildSummaryPrompt` a `PRLinks` field and make `writeChildSummaryLine`
produce exactly one bounded, control-character-free line per child, with
self-delimiting comment and pull-request segments. Rename the section heading so
it does not assert completion for a list that may contain a restarted child.

## In scope

- Add `PRLinks []string` to `ChildSummaryPrompt`.
- Render four line shapes; both optional segments carry their own leading ` — `
  and an omitted segment takes that delimiter with it.
- Replace every rune rejected by `strconv.IsPrint` with a single space in every
  interpolated field, before any cap and before `%q` quotes the comment.
- Count every cap in Unicode code points: identifier and state 50, comment 500,
  title 200, and each URL 200. A value over its cap renders as a leading slice
  plus ` [truncated]`, with slice and marker together never exceeding the cap.
- Render at most ten URLs, sorted ascending by URL string before capping, with
  ` (+N more)` carrying the count *not* rendered, clamped to ` (+999+ more)`.
- Change the heading from `"\nCompleted children:\n"` to `"\nChild tasks:\n"`;
  leave the lead-in, closing instruction, and truncation notice unchanged.
- Update `prompt_builder_test.go:240`, which asserts on the old heading.

## Out of scope

- Where the data comes from. This work order renders a populated
  `PromptContext` and reads no database.
- The `ChildSummariesTruncated` decision, which Task 01 computes.
- Any other reason's prompt builder.

## Acceptance

- A child with a CR, LF, U+2028, or tab in its title or comment still renders as
  exactly one line, and a multibyte comment of 501 code points renders as 485 code
  points plus the marker while one of 500 renders whole and unmarked, both valid
  UTF-8.
- A child with 12 links renders ten sorted URLs and ` (+2 more)`; a child with no
  links renders no pull-request text and no stray delimiter; a child with a
  comment and no links, and one with links and no comment, each render their one
  segment with a single delimiter.
- The heading is `Child tasks:`, and a context with no summaries renders neither
  heading nor truncation notice while keeping the lead-in and closing instruction.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/office/service/ -run 'TestBuildPrompt_ChildrenCompleted|TestWriteChildSummaryLine|TestTruncateComment' -count=1
```

```bash
cd apps/backend && golangci-lint run ./internal/office/service/...
```

## Files likely touched

- `apps/backend/internal/office/service/prompt_builder.go`
- `apps/backend/internal/office/service/prompt_builder_test.go`

## Dependencies

None.

## Risks

- `truncateComment` currently uses `len(s) > 500` and `s[:485]`, both byte
  operations. Converting to runes is the point of the change; leaving either in
  bytes mislabels non-ASCII comments and can emit invalid UTF-8 into a live agent
  prompt, so every cap test needs a multibyte fixture rather than ASCII padding.
- `(+N more)` is the count not rendered, not the total. A child with 12 links
  renders `(+2 more)`; emitting `(+12 more)` reads as if none were shown.
- `writeChildSummaryLine` grows three segments and a sanitizer; `funlen` is 80
  lines and `cyclop` 15. Extract the sanitizer and the pull-request segment as
  helpers rather than growing one function.
- The existing `TestBuildPrompt_ChildrenCompleted_NoSummaries` asserts the *old*
  heading is absent, so it keeps passing after the rename for the wrong reason.
  It must be re-pointed at the new literal.

## Parallelism

`parallel-safe` with Task 01. Files are disjoint; the only coupling is the shared
500-code-point comment limit, whose SQL side Task 01 owns.

## Inputs

- System design, [Rendered line contract](../../specs/office/system-design/children-completed-wake-summaries.md#rendered-line-contract).
- `apps/backend/internal/office/service/prompt_builder.go:354-398`.
- `apps/backend/internal/office/service/prompt_builder_test.go:180-243`.

## Results

Implemented. `ChildSummaryPrompt.PRLinks` added; `sanitizePromptField` (via `strconv.IsPrint`), `capRunes`, `truncateComment` and `renderChildPRLinks` added; identifier and state are capped at 50 code points; heading is now `Child tasks:`. Deviation: the comment keeps 485 code points rather than the maximal 488, per the design's explicit per-field figure and its Persistence ceiling of 497. The stale `"Completed children:"` assertion in `prompt_builder_test.go` was re-pointed, and multibyte identifier and state cap coverage is in `child_summary_line_test.go`.
