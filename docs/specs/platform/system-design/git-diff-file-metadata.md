---
status: current
system: platform
requirements:
  - REQ-PLATFORM-GIT-DIFF-FILE-METADATA-001
created: 2026-10-02
owners:
  - kandev
---

# Git diff file metadata design

## Boundary

Platform owns source classification shared by local commit and cumulative
comparisons. `GitOperator.ShowCommit` and `GitOperator.GetCumulativeDiff` in
`apps/backend/internal/agentctl/server/process/git_log.go` both use
`parseCommitDiffWithOptions`. Only its per-section status decision changes.

The [merge-detail requirement](../../ui/requirements/merge-commit-details.md)
retains first-parent/root/empty comparison semantics and uncapped commit detail.
The draft [file-navigation contract](../../ui/requirements/commit-file-navigation.md)
retains presentation ownership. Neither receives cumulative classification criteria.
The [workspace path design](workspace-git-path-details.md) retains NUL numstat,
literal selection, and porcelain-owned workspace classification.

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.1, .2 | Raw extended-header classification |
| AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.3 | Callers and transport |
| AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.4, .5 | Preserved contracts |

## Raw extended-header classification

Git's [patch format](https://git-scm.com/docs/diff-format) separates the
`diff --git` path header, extended headers, file headers, and patch payload.
Modes are six octal digits; rename headers carry a whole path after the header
keyword. Marker substrings anywhere in a section are not status evidence.

Keep one small process-local status helper in `git_log.go`. Start after the
section's first `diff --git` line. Inspect raw lines only in the extended-header
region. Match a complete `new file mode <mode>` or `deleted file mode <mode>`
line with a six-digit octal mode, or `rename from <nonempty path>` at column zero.
The existing default is `fileStatusModified`; recognized metadata yields the
existing added/deleted/renamed strings. There is no new enum or generic patch AST.
Valid Git sections do not contain competing add/delete/rename classifications.

End metadata inspection at the first file header (`--- ` or `+++ `), hunk
header (`@@`), binary summary (`Binary files `), or `GIT binary patch` marker.
Do not inspect that boundary line or anything after it for status. This also
covers pure renames and empty-file changes that end after extended headers.
Old/new mode and index/similarity/copy headers alone retain the modified default.
Malformed or incomplete status-like lines provide no classification evidence;
they do not create a new error path. Existing section/path eligibility remains
authoritative.

Never trim, dedent, C-unquote, or normalize lines before classification.
A context line begins with a space, an added line with `+`, and a removed line
with `-`; each prefix is data framing. Hunk function text and filenames in the
first section header, file headers, rename paths, and binary summaries cannot
be rescanned for marker substrings. A genuine rename whose source contains
`new file mode` still classifies from the rename header itself.

## Callers and transport

`ShowCommit` passes uncapped options and authoritative mixed text numstat counts.
`GetCumulativeDiff` passes its existing byte budgets and uses patch line counts.
Classify each full section before `applyDiffBudget`, so skipped/truncated patch
output cannot erase metadata or change status. Leave path extraction, section
splitting, counts, and byte accounting untouched.

`server/api/git.go` serializes the same maps through
`GET /api/v1/git/commit/:sha` (optional `repo`) and
`GET /api/v1/git/cumulative-diff` (base/target/repository selection and aggregation).
`GitOperatorFor`, stored per-repository comparison bases, aggregate NUL-qualified
keys, `repository_name`, `base_ref`, and `is_submodule` keep their existing owners.
Existing backend/WebSocket projections pass status through; they need no edits.
Real HTTP tests exercise the registered router with disposable repositories,
including selected repository reads and cumulative aggregation. No server port,
application instance, browser, database, or external service is needed.

## Preserved contracts

Keep Git argv/environments, first-parent and root behavior, genuinely empty
results, fixed prefixes, exact paths and patch bytes, line counts and aggregates,
per-file/total/file-count limits, skip reasons, and all response shapes.
Workspace mutation and history-provider code are outside this helper's boundary.
No persistence, configuration, metrics, retries, or authorization changes occur.
This is a repair within existing boundaries; no architecture decision with
meaningful new alternatives requires an ADR.

## Verification and surface assessment

Parser regressions cover all three markers in unquoted/quoted paths, added,
removed, and context lines (including leading whitespace), and hunk descriptions.
Positive controls include genuine added/deleted empty and binary sections,
ordinary/mode-only/binary changes, pure and edited renames, and misleading text
in real renamed paths. Test original map keys, patch bytes and counts as well as
statuses, and preserve status across budgeted output.

Disposable real Git repositories drive both public operator methods. Existing
first-parent, path, count, prefix and budget controls remain in the targeted run;
new root/empty controls close gaps in those preserved semantics. Parser fixtures
remain platform-independent. Scope only genuinely unsupported filesystem names
or mode operations on native Windows; do not skip portable marker filenames.

Mobile-parity assessment: source-data normalization alone changes no layout,
touch behavior, scrolling, navigation, or viewport-dependent interaction. Shared
process/HTTP evidence suffices; no new desktop/mobile Playwright flow or ASCII
UI preview is required. Public-docs assessment: add one sentence to the existing
Git operations reference explaining status provenance. It remains a how-to page
with a bounded reference subsection, and adds no new page or navigation entry.

See the [one-work-order repair package](../../../plans/git-diff-status-metadata/plan.md).
