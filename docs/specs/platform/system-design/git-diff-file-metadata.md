---
status: current
system: platform
requirements:
  - REQ-PLATFORM-GIT-DIFF-FILE-METADATA-001
created: 2026-10-02
updated: 2026-10-09
owners:
  - kandev
---

# Git diff file metadata design

## Boundary

Platform owns source classification shared by local commit and cumulative
comparisons. `GitOperator.ShowCommit` and `GitOperator.GetCumulativeDiff` in
`apps/backend/internal/agentctl/server/process/git_log.go` both use
`parseCommitDiffWithOptions`. The producers supply plain Git patch output;
the parser classifies each section from raw extended headers.

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
| AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.6, .7 | Plain comparison output; Callers and transport |
| AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.8, .9 | Built-in cumulative patches; Callers and transport |
| AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.10, .11 | Actual-byte comparison patches; Callers and transport |
| AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.12, .13 | Short parent gitlink patches; Callers and transport |
| AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.14 | Visible parent gitlink comparisons; Callers and transport |

## Visible parent gitlink comparisons

Short output selects a format; it does not override `diff.ignoreSubmodules`.
At the qualified repair baseline, an `all` preference removed a real committed
pointer update from both patch producers before the existing parser saw a
section. With the child unavailable, this removed the only evidence needed by
[AC-UI-SUBMODULE-REVIEW-001.5](../../ui/requirements/submodule-review.md).

At initial published repair `15e0e6a30605da37f8e3d20da15d3d63d90d3e54`, both
patch producers passed exact `--ignore-submodules=none`. This restored recorded
pointer changes but allowed the cumulative working-tree comparison to decorate
an unchanged child HEAD with `-dirty` for child-only tracked or untracked files.
That decoration is not recorded parent pointer evidence.

The bounded corrective policy keeps `ShowCommit` on exact
`--ignore-submodules=none` and uses exact `--ignore-submodules=dirty` for
`GetCumulativeDiff`. Git's dirty mode retains real pointer changes, including a
changed child HEAD, while excluding child-only worktree dirt from the parent
patch. The exact argument list in `securityutil.IsKnownSafeGitFlag` admits only
these two values; bare/abbreviated/empty names, unsupported values, suffixes,
whitespace and control-character variants remain rejected. No prefix admission
or validation bypass is needed. Exact dirty admission precedes the cumulative
argv correction; none admission remains required for commit detail.

Retain `--submodule=short`, first-parent semantics, color/textconv suppression,
cumulative external-diff suppression, fixed prefixes, captured environment,
managed execution, admission and cancellation. The no-patch commit metadata
query, parser, counts, budgets, status/tracker, history and other Git operations
keep their existing contracts. No config write, retry, new subprocess, API or
frontend change is required.

The parent map supplies the gitlink fallback through the existing registered
HTTP and WebSocket projections. Initialized children remain distinct scopes
with parent-recorded anchors. Review's existing
`suppressAvailableGitlinkFiles` in `apps/web/components/review/types.ts` retains
the parent when no child contributes files and suppresses it when child files
are available. This repair does not redefine discovery, source precedence,
child file collection or presentation. Parent cumulative patches exclude child-only dirt; initialized child patches retain their existing worktree behavior. A missing parent map does not establish
that initialized-child aggregate data is empty.

Independent permanent regressions use real Git operators and their own registered
selected/aggregate HTTP fixture. A real declared submodule is deinitialized after
committing its pointer advance, so the child cannot supply alternate evidence.
Use explicit positive old/new commit IDs and raw built-in short patch
oracles with explicit ignore-none for commit and ignore-dirty for cumulative with color, external diff and textconv disabled, never the production
parser or equality of empty results. Focused tracked/untracked child-dirt cases assert an unchanged parent produces no
section, while changed-pointer-plus-dirt retains exact old/new IDs without
`-dirty`. A registered aggregate case proves dirty child files remain in their
own scopes and parent-recorded anchors with parent entries absent. Bound ignore-mode controls rather than
crossing every child state, operation and setting. Independent HTTP repositories
share a gitlink path but have distinct child IDs and parent bases; initialized
child evidence separately verifies existing anchors and metadata. Snapshot owned
config, HEAD, refs, index entries and worktree status/content around reads,
resolving linked child Git paths. Isolate HOME/USERPROFILE/XDG and Git environment
before fixture creation and operator/manager capture. Native Git and portable
paths provide native Windows coverage without shell helper fixtures or shared
fixture edits.

The pure-data mobile exception applies: composition, copy, touch, scrolling,
navigation and breakpoints are unchanged. Registered HTTP and operator evidence
prove the repaired data seam; no browser fixture or preview is required. Public
Git operations, sessions/Review, feature-status and WebSocket guidance, README
and screenshots already describe faithful metadata, nested anchors and unavailable
child fallback; they need no edit. This remains within accepted nested-scope and
shared comparison boundaries, so no new ADR is needed. Delivery is in the
[one sequential work-order package](../../../plans/git-submodule-ignore-review-evidence/plan.md).

## Short parent gitlink patches

The prerequisite is exact safe-flag admission: add only `--submodule=short`
to the exact argument list in `securityutil.IsKnownSafeGitFlag`
(`apps/backend/internal/common/securityutil/git.go`). It was absent at the format repair
baseline `993f60ce889b6dcca1ca19fe98703a9159ab28c3` and is now admitted. Reject abbreviated names, other values, extra suffixes,
empty values and whitespace variants. Do not admit a `--submodule` prefix,
bypass validation, or change global Git configuration/environment policy.

Pass the exact flag after the subcommand and before the ref in ONLY the two
existing patch argument lists in `git_log.go`: `GitOperator.ShowCommit` and
`GitOperator.GetCumulativeDiff`. The no-patch commit metadata query, history
queries and all other Git producers retain their current arguments. Preserve
`--first-parent`, `--no-color`, `--no-textconv`, cumulative `--no-ext-diff`,
fixed `a/` and `b/` prefixes, managed execution, captured environment,
admission, cancellation and budgets.

With `diff.submodule=log`, Git emits a submodule log instead of the parent
gitlink's unified patch section. `splitDiffSections` therefore supplies no
parent file to the existing parser. Inline child diff display is likewise
unsuitable for a parent file identity. Select short gitlink patches at these
producers rather than parsing additional formats or treating child file
patches as parent files. The existing parser already handles the short
`Subproject commit` patch, mode 160000, and added/deleted/modified statuses.

Initialized child repositories still use their own operators and parent-recorded
comparison anchors under the [nested scope decision](../../../decisions/2026-08-05-nested-submodules-as-repository-scopes.md).
The [submodule Review contract](../../ui/requirements/submodule-review.md)
continues to own discovery, repository routing, and suppression of the parent
gitlink row when child file diffs are available. Parent and child API data remain
distinct. A missing parent patch does not imply the entire aggregate is empty.

## Plain comparison output

Pass `--no-color` to the patch-producing `git show --stat --numstat -p`
invocation in `ShowCommit` and the patch-producing `git diff` invocation in
`GetCumulativeDiff`. Keep it after the subcommand and before the comparison ref.
Register only exact `--no-color` in `securityutil.IsKnownSafeGitFlag`
(`apps/backend/internal/common/securityutil/git.go`), retaining rejection of
value forms and suffixed variants. No safe-prefix expansion or validator bypass
is permitted. This is a per-invocation output contract, not a repository configuration write
or a shared subprocess color policy. The separate no-patch commit metadata
query retains its existing custom format.

`splitDiffSections` recognizes `diff --git ` at column zero. Git display color
can prefix that header with escape sequences, making both methods report a
successful empty file map. Prevent decoration at its source rather than
changing the parser or stripping ANSI sequences from output. Escape bytes in
file content and Git-quoted paths remain data and must survive unchanged.

Use the existing `runGitCommand` validation, selected operator environment,
interactive admission, after-acquire budget, cancellation, and managed process
lifetime. No extra subprocess, environment override, config/ref write, retry,
or new error path is needed. The [managed execution design](git-subprocess-execution.md)
continues to own shared execution policy. Live tracker patches and workspace
path selection remain independent under the [path design](workspace-git-path-details.md).

## Built-in cumulative patches

Pass exact `--no-ext-diff` after `diff`, before the comparison ref, in
`GetCumulativeDiff`'s existing patch invocation. Register it only in the exact
argument list of `securityutil.IsKnownSafeGitFlag`; reject abbreviated, value,
suffixed and whitespace variants. This dependency belongs to the same repair,
with no prefix widening or validation bypass.

An external helper selected by `diff.external` or `GIT_EXTERNAL_DIFF` can
replace unified patch output with arbitrary text. A zero-exit helper output
without column-zero `diff --git ` sections reaches the existing parser as a
successful empty comparison. Select built-in output at the producer rather
than teaching the parser arbitrary helper formats. `ShowCommit`'s existing
`git show` uses built-in output by default and remains a positive control;
this external-helper defect does not justify adding an external-diff flag there.

Keep `--no-color`, fixed prefixes, base-to-worktree semantics and captured
environment handling. Do not clear helper variables, rewrite configuration,
add subprocesses, or change shared execution policy. Other Git producers,
rename detection and comparison-base selection retain their existing contracts.
Text conversion in these two comparison producers is handled separately below.

## Actual-byte comparison patches

The exact safe-flag dependency is part of this repair: add only `--no-textconv`
to `securityutil.IsKnownSafeGitFlag`'s exact argument list in
`apps/backend/internal/common/securityutil/git.go`.
Reject abbreviated, value, suffixed and whitespace variants; no prefix widening
or validation bypass is needed.

Pass that exact flag after the subcommand and before the ref in the two existing
patch-producing argument lists in `git_log.go`: `ShowCommit`'s
`show --first-parent --no-color --format= --stat --numstat -p` and
`GetCumulativeDiff`'s `diff --no-color --no-ext-diff`. The separate no-patch
metadata query remains unchanged. Retain fixed prefixes and every existing
comparison argument. Keep `runGitCommand`, its captured operator environment,
admission, cancellation and managed process lifetime unchanged.

`--no-ext-diff` does not disable a text converter selected through attributes
and `diff.<driver>.textconv`. Both patch producers otherwise use conversion.
Equal converted outputs can erase a real change's patch section while commit
stat/numstat still advertise the change. The parser then reports a successful
empty file map and derives zero commit totals. Other converters can replace
actual file content with a different patch. Disable conversion at these two
producers so their existing parser receives built-in patches from actual bytes.

This per-invocation selection neither edits driver configuration/attributes nor
forbids converters globally. No parser, count, budget, ref, shared command
framework, environment or other Git reader/write changes belong to the repair.

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

Apart from the plain-output flags, cumulative external-diff suppression, these two producers' text-conversion suppression, short gitlink display and explicit gitlink visibility, keep Git argv/environments, first-parent and root behavior, genuinely empty
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

Plain-output regressions use disposable real Git repositories with unset color
defaults, UI-only forced color, diff-only forced color overriding disabled UI,
both forced, and disabled diff overriding forced UI. Assert explicit membership,
statuses, counts, commit metadata and exact plain patch bytes; include literal
ANSI source content and an escape-containing filename on supported filesystems (exclude only that fixture
on native Windows, which rejects control characters in filenames). Cover dirty
cumulative reads, binary and empty-file changes, root/first-parent merge and
genuinely empty comparisons. Existing budget/limit tests remain authoritative.
Registered HTTP tests cover single, selected and aggregate reads across two
independent repositories with the same path and distinct content. Snapshot
owned config bytes, HEAD, refs, index entries and worktree status/content before
and after reads. Test setup may configure disposable fixtures; production reads
may not. Tests use existing captured-environment seams and do not replace them.

For the plain-output repair, no public-doc change is needed: the existing Git
operations reference already describes faithful read-only comparison data.
No user-facing option, workflow, schema, or rendered surface changes. The
source-data-only mobile assessment above applies. Delivery is recorded in the
[plain-output package](../../../plans/git-comparison-plain-output/plan.md).

External-helper regressions use actual Git through the public cumulative
operator and registered selected/aggregate HTTP routes. Independent repositories
carry the same path with distinct content and bases. Cover configured helpers,
environment-selected helpers and both together, with plain and commit positive
controls, dirty tracked changes, empty results and cumulative limits. Assert
explicit expected membership, counts, metadata and patch bytes from a built-in
raw Git oracle. A native helper fixture emits custom output and writes an owned
execution sentinel; a deliberate fixture control proves it can run, while
comparison reads must leave that sentinel absent. Keep the actual Git executable
in use. Prefer re-executing the native test binary over a POSIX-only script;
scope only demonstrated platform-specific assertions, not functional coverage.

Snapshots bracket production reads after fixture setup and retain config bytes,
HEAD, refs, index entries, worktree status and file bytes. Snapshot/oracle Git
diffs must select built-in output too, so fixture helpers cannot corrupt the
oracle or execute during observation. Establish helper environment overrides
before manager construction, or use the existing explicit operator environment
provider; ambient changes after captured-environment construction are invalid
coverage. Do not alter existing shared fixture helpers for this repair.

The mobile and public-docs assessment remains data-only: the existing Git
operations guide's comparison metadata and read-only guidance is accurate;
there is no UI, copy, schema, user setting or workflow change. Delivery is in the
[built-in cumulative package](../../../plans/git-cumulative-built-in-patch/plan.md).

Text-converter regressions use actual Git through both public operator methods
and registered selected/aggregate HTTP routes. A guarded native test-binary
helper follows the portable pattern in `git_log_external_diff_test.go`, without
rewriting shared fixtures or requiring a Windows shell script. No-driver
controls assert positive membership and old/new bytes. Configured helper modes
alter output or emit identical constant text, and deliberate raw Git controls
prove both modes execute before clearing only their owned sentinels. Production
reads must return actual bytes and leave the sentinels absent. Raw patch oracles
and snapshot diffs explicitly disable textconv, external diff and color; they
never use the production parser as their oracle.

Keep the matrix targeted: committed and dirty text changes, selected converted
binary content, an empty-file change and genuinely empty comparison. Preserve
first-parent/root semantics with the existing focused controls and cumulative
budgets with existing limit controls. Independent HTTP repositories share a
path but contain distinct old/new bytes, bases and driver settings. Assert
selected isolation, aggregate NUL-qualified keys, `repository_name`, `base_ref`
and ordinary-repository `is_submodule` omission, plus commit metadata and totals.
Snapshot config/attribute bytes, HEAD, refs, index entries and dirty content
after fixture setup and around reads. Install any helper environment guard
before operator/manager capture. Only the necessary caller seam uses HTTP;
there is no browser, database, application launch or new transport contract.

The source-data-only mobile exception applies. The existing Git operations
how-to/reference guidance remains accurate; no public guide edit is required.
Delivery is in the [one-work-order text-converter package](../../../plans/git-comparison-textconv/plan.md).

Submodule regressions are independently authored in new process and registered
HTTP test files; reuse accepted discovery receipts without importing or replaying
the discovery source. Native Git fixtures use disposable parent/child repositories,
actual gitlink commits and no shell helper. Compare explicit file identities,
statuses, counts and old/new commit IDs with raw Git short-patch oracles that
also disable color, external diff and text conversion. Do not use the production
parser as the oracle or accept equality of two empty results.

Keep the matrix bounded: forward updates under unset/short/log/diff preferences;
backward updates, additions and deletions under log with short controls; an
ordinary file under log; a dirty tracked gitlink; a root gitlink addition and a
genuinely empty comparison. Existing focused first-parent, prefixes, uncapped
commit and cumulative-budget tests retain their owners and provide compatibility
evidence once, rather than replaying sibling packages in full.

Registered selected commit/cumulative reads and aggregate cumulative reads use
independent repositories with the same gitlink path but different child commit
IDs and parent bases. Aggregate requests include the required `base` query
parameter while each scope resolves its own stored base. Assert selected
isolation, NUL-qualified aggregate keys, exact `repository_name`/`path`/`base_ref`,
and `is_submodule` only on child-scope files. An initialized child's actual file
patch remains separate and anchored to its parent's original gitlink. Snapshot
configuration bytes, HEAD, refs, index entries, and worktree status/content in
all owned scopes around production reads; resolve linked child Git paths rather
than assuming `.git` is a directory. Install fixture environment isolation before
operator/manager capture. No server port, browser, database or application launch
is required.

The pure-data mobile exception applies: existing desktop/phone composition,
copy, navigation, touch behavior and scrolling are unchanged. Public audit of
Git operations, sessions/Review, feature status, WebSocket guidance, README and
screenshots found the existing metadata, nested-scope and read-only descriptions
remain accurate. No public guide edit or new ADR is needed. Delivery is in the
[submodule comparison format package](../../../plans/git-submodule-comparison-format/plan.md).
