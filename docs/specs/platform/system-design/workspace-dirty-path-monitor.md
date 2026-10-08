---
status: current
system: platform
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
created: 2026-10-02
updated: 2026-10-02
owners:
  - kandev
---

# Workspace dirty-path monitor

## Purpose and boundaries

This supplement to [workspace Git status](workspace-git-status.md) specifies the
quick polling signal for repeated edits to dirty tracked files.
Platform owns observation and publication; Tasks retains environment/repository
binding. It implements `AC-PLATFORM-WORKSPACE-GIT-STATUS-001.42` and preserves
the dependency-tree policy in `.13` through `.15` and execution bounds in `.31`.
The full observation fingerprint and enrichment validation remain governed by
the parent design. The quick monitor signal does not replace either.

## Raw paths and modification evidence

`WorkspaceTracker.getWorkspaceState` in `workspace_monitor.go` runs
`git diff-files --name-only -z` through `runPollingGitOutputWithStderr`.
NUL framing supplies raw repository-relative paths regardless of `core.quotePath`.
`buildDirtyFilesID` splits only at NUL boundaries, ignores empty records, and
never trims, unquotes, normalizes Unicode, or splits a filename at a newline.

Each exact path contributes to the internal equality fingerprint. After the
existing `sanitizePath` containment check, `os.Stat` supplies that file's
nanosecond modification time when available. Missing/deleted files still
contribute their path and the absence of mtime evidence. Frame paths and mtime
evidence with NUL delimiters, so filename punctuation cannot impersonate record
separators. Paths cannot contain NUL on supported filesystems. Retain the
existing stat/symlink behavior and containment policy; do not add content reads,
hash scanning, or a new watcher/parser framework.

The index mtime signal and eligible-untracked query/fingerprint remain separate
and unchanged. Tracked paths under `node_modules` remain eligible, while
untracked dependency content stays excluded before enumeration.

## Tick and publication

`workspaceState.changed` compares the same three internal state fields.
A changed quick observation runs the existing `monitorTick` branch:
`tryUpdateGitStatus`, `updateFiles`, then a repository-scoped
`FileChangeNotification` with operation `refresh` and an empty path.
The status observer retains tracker-owned ordered basic/enrichment publication.
Its lock/admission behavior is unchanged; a refresh attempt can be skipped when
another update owns the lock. Subscriber channels remain bounded and nonblocking.
No delivery guarantee for full channels or new retry policy is introduced.
An identical quick observation executes no refresh branch.

Keep fast/slow/paused polling, overlap guards, failure thresholds, command
deadlines, captured instance Git environment, `GIT_OPTIONAL_LOCKS=0`, and
repository scoping unchanged.

## Other refresh sources and limits

The parallel Git poll hashes porcelain-v2 status entry text, besides observing
HEAD, branch, and upstream. It can start status refresh on a membership or
classification change. Repeated edits with unchanged status entry text need not
change that hash, and its index-only handler does not emit a file refresh.
Explicit/focus refreshes and direct file mutations can also refresh consumers.
These mechanisms may mitigate visible staleness; they do not establish the
missing monitor event. This correction makes no claim that every UI remains
stale until a new workspace mutation.

The polling guarantee requires a changed observable mtime. Timestamp-preserving
writes and stronger atomic filesystem evidence remain outside this correction.
Commit/cumulative file-status metadata from the
[Git diff metadata contract](git-diff-file-metadata.md) is independent.

## Verification and presentation

Use disposable real Git repositories with a committed baseline, first dirty
write, and second dirty write. Set fixed mtimes ten seconds apart and hold index
mtime constant; assert exact-path state changes and identical-state controls.
Default, explicit true, and false `core.quotePath` modes share the filename
matrix. Supported native cases include whitespace, tab, newline, double quote,
Unicode, and ordinary names. Similar decoy paths retain distinct mtime evidence.
Cover dirty tracked deletion and rename transitions, plus tracked dependency
paths and repository-qualified publication.

Direct controlled ticks capture the existing stream messages and accepted basic
status. Verify first-dirty and repeated-dirty refreshes, cached file updates,
and an unchanged tick with no additional refresh. Use existing lifecycle/channel
barriers and `Stop` to drain enrichment; do not start polling loops or an app.
Skip only individually unsupported native filename shapes with a stated OS
reason, retaining all supported controls.

Desktop and phone receive the same producer events. No layout, interaction,
breakpoint, localization, or payload change requires a rendered preview or new
browser test. Public docs need no change for this repair of existing refresh
behavior; internal documentation is the owning contract. No new durable
ownership/security boundary or architectural alternative requires an ADR.

## Delivery

See the single sequential [repair work package](../../../plans/workspace-dirty-path-monitor/plan.md).
