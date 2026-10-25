---
status: current
system: workspaces
requirements:
  - REQ-WORKSPACES-COPYFILES-GLOB-TOKENIZATION-001
---

# Repository Copy-file Tokenization Design

## Purpose and boundaries

The workspace system owns the repository copy-file grammar. The shared
`internal/worktree/copyfiles` package parses settings for host materialization
and remote planning. Executor transport and task setup remain separate consumers.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-WORKSPACES-COPYFILES-GLOB-TOKENIZATION-001` | [Tokenizer and consumers](#tokenizer-and-consumers) |

## Tokenizer and consumers

`splitTopLevelCommas` scans the original string without rewriting pattern bytes.
It tracks brace alternation depth and skips classes through an unescaped closing
bracket. An opener without a closer remains outside class grouping, preserving
independent following entries and their non-fatal warnings. Once no closer
exists, later openers cannot form a class; the scan stays linear. Class contents
cannot alter brace depth. POSIX
backslashes consume the next byte for syntax tracking, including inside classes;
Windows backslashes remain separators, consistent with `doublestar.FilepathGlob`
normalizing native paths. Nested alternations retain their existing behavior.
No glob matcher or dependency is replaced.

`ParseSpecs` trims entries, extracts the reserved terminal suffix through
`parsePatternSpec`, and deduplicates normalized patterns in input order.
`ValidateSpec` uses the same splitter and retains its reserved-mode validation
boundary. Repository create/update use it before storing `Repository.CopyFiles`.
No schema migration or new setting is needed.

`Manager.copyConfiguredFiles` supplies `ParseSpecs` output to `Copy` before setup.
The remote lifecycle path supplies `Parse` output to `Plan`; symlink modes become
bytes remotely. Both retain native literal-path priority, then resolve fully unescaped POSIX
exact paths before `doublestar.FilepathGlob`. The dependency's literal shortcut
unescapes metacharacters only, so exact escaped commas need this contained
literal fallback. Unescaped glob metacharacters and dangling escapes keep the
existing glob/error path. Pattern bytes and native Windows paths stay unchanged.
For POSIX globs, unescape commas only in the literal directory prefix selected
by `doublestar.SplitPattern`, preserving all remaining expression escapes.
Tests exercise these public package pipelines using temporary source and target
directories, checking exact paths, bytes, warnings, and mode precedence.

Native Windows CI runs the focused copyfiles regression immediately after Go
setup, before the longer existing Windows-sensitive suite. That suite emits
streaming JSON diagnostics while retaining verbose output, race detection,
all package scopes, its 25-minute test timeout, and the 40-minute job cap.
This ordering provides native grammar evidence before a later suite cancellation;
streaming output distinguishes test execution from compilation without changing
assertions or establishing that hosted-runner slowness has been corrected.

## Failure, security, and observability

Malformed reserved suffixes continue to fail save validation. Invalid globs and
missing matches continue to produce non-fatal materialization warnings. Existing
source/destination containment checks, idempotency, relative-link rules, and
remote per-file size limits remain authoritative. The tokenizer neither touches
source data nor adds logs or metrics.

## Related decisions

- [Worktree copy-file materialization modes](../../../decisions/0010-worktree-copy-files.md)
