---
status: current
system: ui
created: 2026-10-05
requirements:
  - REQ-UI-TASK-WORKSPACE-CONTENT-SEARCH-001
owners:
  - kandev
---

# Task Workspace Content Search System Design

## Purpose and boundaries

This design pairs with the existing [task search requirement](../requirements/task-workspace-content-search.md), which already owns Files/Contents navigation, eligible file membership and independent repository scopes. Its backend inventory and transport are part of that vertical contract. This correction does not migrate ownership. [Workspace Git status](../../platform/system-design/workspace-git-status.md) separately owns changed-file observations and monitoring; its exact changed-path criteria do not define this complete search inventory. Workspace lifecycle and mutation authority remain with [Workspaces](../../workspaces/README.md).

## Requirement mapping

| Criteria under REQ-UI-TASK-WORKSPACE-CONTENT-SEARCH-001 | Design section |
| --- | --- |
| .1 through .5 | Existing navigation and transport |
| .6, .7 | Repository scopes and result contracts |
| .8, .9 | Exact Git inventory and search consumption |
| .10 | Browser retry recovery |

## Existing navigation and transport

The browser's `apps/web/lib/ws/workspace-files.ts` sends `workspace.files.search` and `workspace.content.search`. `WorkspaceFileHandlers.wsSearchFiles` and `wsSearchContent` in `internal/agent/handlers/workspace_file_handlers.go` authorize session access and resolve execution before delegating to the agentctl client. The client in `internal/agent/runtime/agentctl/client_files.go` forwards queries over HTTP; the existing command palette and `use-workspace-content-search.ts` consume the response. Shortcut arbitration, mode switching, route gating, cancellation and stale-result handling remain in their existing owners. This inventory correction changes no browser code or interaction contract.

`api.Server` registers aggregate GET `/api/v1/workspace/search` and `/api/v1/workspace/content-search`. The first returns the legacy `files` array plus structured `results`; the second returns typed content matches. Neither aggregate endpoint gains a repository selector. Selected reads use the existing GET `/api/v1/workspace/file/content` with URL-encoded `repo` and `path`, through `Manager.JoinRepoPath` and `WorkspaceTracker.GetFileContent`.

## Browser retry recovery

`useWorkspaceContentSearch` debounces for 250 ms, then makes eight attempts
500 ms apart through the real `searchWorkspaceContent` WebSocket wrapper with
`limit_per_repo: 50`. Each successful response clears the previous attempt's
error, including an empty response. After polling, an empty accumulated result
set fails only when the final attempt failed. A successful attempt followed by
a terminal failure is not proof of a completed empty search. Any collected
matches still survive later failures under the existing partial-result policy.

Keep incremental union/dedup by raw repository, path, line and column. Query,
session, inventory and enabled-state invalidation retain the existing effect
cancellation guard. No cache, backend retry, transport shape or persistence
change is needed. `CommandPanel` passes the hook state to
`WorkspaceContentSearch`, which already renders translated no-match and failure
rows. Desktop and phone use the same state logic and unchanged presentation;
this state-only correction requires no new viewport-specific interaction.

## Repository scopes and result contracts

`Manager.SearchWorkspaceFileResults` and `contentSearchTrackers` retain the unnamed root when `gitIndexPath != ""`, plus named child scopes. A bare task holder with sibling repositories contributes no additional searchable root. Repository materialization remains reconciled by the current manager paths.

Filename results retain task-root-relative paths prefixed by a named repository and the separate `repository_name`. Content results retain repository-relative `path`, raw `repository_name`, one-based `line` and UTF-16 `column`, `preview` and half-open UTF-16 `match_ranges`. JSON escapes control characters for transport; decoded path strings retain those characters. Prefixing and scope selection never substitute a same-path file in another repository.

## Exact Git inventory and search consumption

`WorkspaceTracker.getFileListClass` in `internal/agentctl/server/process/workspace_files.go` is the sole source of this Git-backed inventory. Use one invocation:

```text
git ls-files --cached --others --exclude-standard --stage -z -t
```

Keep `subproc.NewGitCommand`, `RunGitOutputAfterAcquire`, the caller's `GitWorkClass`, `gitCommandTimeout`, working directory, environment preparation and `gitCommandError`. Background cache refresh and interactive content enumeration retain their distinct admission classes and existing post-admission deadline. Do not add a second Git process or override `core.quotePath`.

The record grammar is:

```text
untracked: ? SP filename NUL
tracked:   tag SP mode SP object SP stage TAB filename NUL
tag:       H | S | M
mode:      six octal digits (including 100644, 100755, 120000, 160000)
object:    Git hexadecimal object identity (SHA-1 or SHA-256)
stage:     0 | 1 | 2 | 3
```

`H` includes ordinary and assume-unchanged indexed files; `S` includes skip-worktree entries; `M` identifies unmerged stages. With this exact option set, deleted/modified entries still appear through the indexed H/S/M branch. R/C/K and lowercase tags belong to other options, which are absent. The command expands sparse indexes normally; `--sparse` and `--recurse-submodules` are absent. These facts are independently grounded in [Git's manual](https://git-scm.com/docs/git-ls-files) and [its producer source](https://github.com/git/git/blob/v2.51.0/builtin/ls-files.c).

Split only at NUL record boundaries. Remove the explicit tag and its one space. For `?`, the entire remaining payload is the path, even if it resembles mode/object/stage metadata or contains tabs. For H/S/M, separate the stage header from the filename at the first tab and inspect mode within that header. A source-local parser may validate the small header grammar; it must not apply whitespace parsing to the path. An empty stream is a successful empty inventory; the final NUL is framing, not an empty filename. Never trim, unquote, Unicode-normalize or newline-split a path. No new promise is made for invalid-UTF8 filename JSON.

Only actual tracked mode `160000` is excluded as a Gitlink. An untracked filename beginning with that text remains eligible. Apply `isRootOwnershipMarkerPath` to the extracted path, retaining nested files named like the ownership marker. Standard Git ignore rules remain authoritative for untracked files; tracked files remain included even if they match an ignore pattern. Preserve per-stage multiplicity rather than adding deduplication or changing ranking.

Each call returns a fresh `FileListUpdate` slice. `updateFilesClass` publishes it to `currentFiles` under the existing mutex after its existing error/cancellation checks. `SearchFiles` and `appendTrackerFileSearchCandidates` copy paths from that cache. `SearchContent` independently enumerates fresh inventory, sorts its own slice, then reads each file through existing `resolveSafePath` containment and regular-file/size/text checks. Sorting the fresh slice is not a shared-cache data race. Exact inventory fixes the current silent skip when an incorrectly parsed path cannot be opened.

## Failure, isolation and persistence

Command admission, cancellation, execution errors and cache publication remain governed by existing behavior. Unexpected malformed framing/header must not publish a partial inventory; return a source error through the current failure path if validation is added. Ordinary unreadable, removed, unsafe, oversized, binary or invalid-text content still follows existing per-file skip behavior. No retry, fallback enumeration, persistent index, cache ownership move, schema, runtime flag, security boundary or permission change is introduced.

Search and selected reads must leave Git HEAD, refs, index bytes, repository configuration and fixture file bytes unchanged. Keep the existing safe-path resolver and subprocess environment invariant. No new diagnostic metrics or raw filename logging are needed.

## Validation and presentation

Real Git fixtures must drive production inventory, background cache refresh, cached filename search and content search. Registered HTTP tests exercise both aggregate responses followed by selected reads, using distinct content in independent repositories sharing an exact path. Permanent coverage includes native supported names, quoting preferences, tracked tags/modes/stages and eligibility/exclusion controls. The [work order](../../../plans/exact-workspace-search-filenames/task-01-preserve-exact-search-paths.md) defines exact checks.

The exact-filename inventory correction is backend result-data correction. Desktop and phone consumers receive the same path strings and repository fields. Layout, touch, scrolling, navigation, shortcuts, localization and viewport behavior do not change; no rendered preview, browser/build or new mobile E2E is required for inventory handling. Browser retry recovery is verified through the real hook, WS wrapper and consumer with only request transport mocked. Existing UI feature limitations are outside this inventory contract.

## Related delivery

- [Exact filenames plan](../../../plans/exact-workspace-search-filenames/plan.md)
- [Content-search retry recovery plan](../../../plans/workspace-content-search-recovery/plan.md)
- [Completed root/submodule search correction](../../../plans/submodule-workspace-search/plan.md)
- [Completed match reveal work](../../../plans/content-search-match-reveal/plan.md)
- [Classified Git execution](../../platform/system-design/git-subprocess-execution.md)
