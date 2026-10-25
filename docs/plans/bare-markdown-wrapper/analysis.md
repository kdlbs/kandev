# Bare-wrapper ambiguity analysis

## Finding

No content-only heuristic can guarantee both requested outcomes for every valid
Markdown message. A bare code block can contain headings and tagged-looking fence
lines as literal text. Those characters do not encode the author's wrapper intent.

The work remains blocked. A thematic-break boundary does not establish wrapper
intent: the same bounded range can contain two complete independent blocks. No
safe content-only discriminator is available for this task.

## Source evidence

- `apps/web/lib/markdown/normalize-cache.ts`:
  `strengthenMarkdownWrapperFence` rejects a bare first opener through
  `MARKDOWN_WRAPPER_OPEN_RE`.
- The same helper uses `firstNonBlankLineIndex` and `lastNonBlankLineIndex`.
  Introductory and trailing prose also prevent detection.
- `markdownWrapperInnerFenceInfo` is a heuristic scan, not evidence of author intent.
  `FENCE_OPENER_LINE_RE` can match text inside an already open code block.
- A boundary search without an enclosing-fence parser also sees rules and fence
  candidates inside a valid five-backtick code block.
- `apps/web/components/shared/memoized-markdown.tsx` passes
  `normalizeCached(content)` directly to `ReactMarkdown`.
- Existing tests preserve Markdown samples followed by separate blocks and
  preserve tagged wrappers with trailing prose outside the wrapper.

## Compact reproduction

Construct the raw string from the following lines. `BT3` means exactly three
backticks, and `BT3go` means those backticks immediately followed by `go`.
`EMPTY` means an empty line. These labels are not part of the input.

```text
Copy everything between the lines.
EMPTY
---
EMPTY
BT3
Open a pull request.
## Why
BT3go
func first() {}
BT3
More prose.
BT3go
func second() {}
BT3
Final prose.
BT3
EMPTY
---
EMPTY
That is self-contained.
```

Desired interpretation: one raw Markdown code block between the rules.
Observed interpretation: three code blocks. The first ends after `func first()`.
The second contains `func second()`. The third consumes the final rule and outro.

The pure transform leaves this input unchanged. This establishes both the
reported symptom and the missing interior-range behavior.

## Counterexample A: two complete legitimate blocks

```text
BT3
## Fence syntax
BT3go
func first() {}
BT3
This paragraph separates two examples.
BT3
func second() {}
BT3
```

The first block shows a Markdown syntax fragment, including a heading and a
literal tagged opener. It ends at the first pure fence after `func first()`.
The paragraph is normal text. The second bare block contains `func second()`.
Both blocks have explicit closing fences.

This input satisfies all three suggested signals:

1. Its first and last nonblank lines are matching bare fences.
2. A same-length tagged opener and pure closer occur strictly between them.
3. It contains an ATX heading and prose between code samples.

Strengthening the outermost fences merges two intended code blocks and their
separating paragraph. Adding the reproduction's introduction and rules does not
remove this ambiguity from an interior-range detector.

## Counterexample A2: rule-bounded independent blocks

```text
---
BT3
## Fence syntax
BT3go
func first() {}
BT3
This paragraph separates two examples.
BT3
func second() {}
BT3
---
```

This input has the proposed thematic-break boundaries, but it is also two
complete bare code blocks separated by a paragraph. The first code block contains
the literal heading and tagged opener. Strengthening the first and last fences
would merge both blocks and the paragraph, so the boundary is not a safe repair
signal.

A valid five-backtick text block can contain the complete compact reproduction as
literal content. A raw-line scan would find its rules and inner fences and rewrite
bytes that Markdown already treats as code text.

## Counterexample B: requiring two tagged pairs

```text
BT3
## Fence syntax
BT3go
func first() {}
BT3
More prose.
BT3go
func second() {}
BT3
Final prose.
BT3
```

Under GFM, this is a bare syntax sample, a separate Go block, and an empty final
code block. A fence without an explicit closer extends to the end of the input.
Such input is valid Markdown, including during streaming.

The alternative intended interpretation is one wrapper containing two tagged
examples. Both interpretations use exactly the same bytes. Requiring two tagged
pairs or prose between them cannot distinguish those intentions.

Headings in these examples are meaningful literal documentation. They are not
evidence that sibling blocks are nonsensical. Natural-language introduction text
and horizontal rules also occur in legitimate example documents.

## Read-only verification

Installed `normalizeMarkdown`, React, and `react-markdown` produced the following
results on 2026-09-11:

| Input | Normalizer changes bytes | Rendered code blocks |
| --- | --- | --- |
| Compact reproduction | No | 3 |
| Counterexample A | No | 2 |
| Counterexample B | No | 3 |
| Two independent tagged Go blocks | No | 2 |

For counterexample A, the emitted HTML has a paragraph between two `pre`
elements. The first code text contains the literal heading and tagged opener.
Counterexample B and the independent Go case also ran with all production remark
plugins: GFM, breaks, and gemoji.

## Disposition

The proposed boundary contract is rejected. Counterexample A2 proves that it
violates the preservation guarantee, and the five-backtick case proves that a
boundary scan can rewrite literal code content. The normalizer therefore keeps
the existing tagged-wrapper behavior and leaves bare wrapper candidates unchanged.
The work order remains blocked until a producer marker, parser-level intent
signal, or other product decision supplies information that raw Markdown lacks.
