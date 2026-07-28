---
status: shipped
created: 2026-07-27
owner: kandev
---

# Task Workspace Content Search

## Why

People working inside a task need to find code by what it says, not only by its
file name. Opening a terminal and running a repository-specific search breaks
the workbench flow, and it is especially awkward when a task contains several
repositories.

## What

- On a task route, **Cmd+Shift+F** on macOS or **Ctrl+Shift+F** elsewhere opens
  the command palette directly in workspace content-search mode.
- The shortcut works while a workbench editor or text input has focus and takes
  precedence over that surface's local find shortcut. It prevents the browser's
  default search action.
- The command palette visibly exposes **Commands**, **Files**, and **Contents**
  as low-chrome text tabs beside the search field. The active mode uses a subtle
  underline, and each mode's direct shortcut remains discoverable from its
  tooltip without adding a separate selector row.
- **Files** and **Contents** are available only on the active task-detail route
  when its selected session belongs to that task. Elsewhere the palette is
  command-only, does not intercept either workspace-search shortcut, and
  normalizes a previously open workspace-search palette back to **Commands**.
- Clicking a mode or pressing **Tab** / **Shift+Tab** switches among those modes
  without discarding the current query. The existing **Cmd/Ctrl+Shift+K**
  shortcut still opens file-name and path search directly.
- File-name and path search covers every repository materialized for the active
  task. Multi-repository results use task-root-relative, repository-prefixed
  paths so same-named files remain distinguishable and open in the correct
  repository. The palette groups those matches by repository and shows paths
  relative to each group.
- Search covers every repository materialized for the active task session.
  Tracked files and untracked, non-ignored files are eligible; ignored files,
  directories, and workspace metadata are not.
- Matching is case-insensitive and fuzzy within one line. A contiguous match
  ranks ahead of a non-contiguous subsequence match. Adjacency, word boundaries,
  and shorter gaps improve a fuzzy match's rank.
- Each result shows its repository, repository-relative path, one-based line
  number, and a single-line content preview. Matched characters are highlighted
  in the preview.
- Results are grouped by repository. A repository's display label may be
  shortened for readability, but selecting a result retains the repository's
  raw transport identity.
- Selecting a result opens or activates that repository's file and places the
  cursor at the result's one-based line and column. Both Monaco and CodeMirror
  reveal the location, including when the editor is mounted after selection.
- Empty-query, searching, no-match, unavailable-session, and failed-search
  states are distinguishable without closing the palette.
- Search is bounded: a query contains at most 200 characters, each repository
  returns at most 50 results, and files larger than the existing 10 MiB
  workspace-file limit are skipped.
- Binary files, invalid UTF-8, unreadable files, paths that escape the
  repository, and files removed during a search are skipped without failing
  the entire query.
- Match ranges use UTF-16 offsets into the returned preview so browser
  highlighting remains correct for non-BMP Unicode text.
- Search does not require an external executable such as ripgrep, so it behaves
  consistently in local, container, remote, and Windows executors.

## API surface

The browser requests workspace content search over the session-scoped WebSocket
action:

```text
workspace.content.search
```

The request contains `session_id`, `query`, and an optional
`limit_per_repo`. The response contains ordered result records:

| Field             | Meaning                                                       |
| ----------------- | ------------------------------------------------------------- |
| `repository_name` | Raw repository identity used by workspace file APIs           |
| `path`            | Repository-relative slash-separated path                      |
| `line`            | One-based source line                                         |
| `column`          | One-based UTF-16 source column of the first matched character |
| `preview`         | Searchable source line presented by the palette               |
| `match_ranges`    | Half-open UTF-16 ranges into `preview`                        |

The backend-to-agentctl HTTP transport exposes the same result shape. Workspace
file-name search keeps its legacy `files` array and additionally returns
structured `results` entries containing `repository_name` and the
task-root-relative `path`, allowing grouping-aware consumers to separate
repositories without parsing path strings.

## Permissions and isolation

- A request is authorized through the session ID before its execution
  workspace is accessed.
- Only repositories attached to that session's task execution are searched.
- Repository-relative paths are resolved through the existing workspace path
  containment rules; symlinks or traversal cannot expand search scope.
- Result data contains no absolute host path.

## Failure modes

- Without an active task session, the palette explains that content search is
  unavailable and sends no request.
- A blank query clears prior results and sends no request.
- A query over the maximum length is rejected consistently rather than scanning
  with a silently different value.
- If one eligible file disappears, becomes unreadable, or is invalid while the
  search runs, other files and repositories still return results.
- If the execution cannot be recovered or the transport request fails, the
  palette presents a retryable failure state and keeps the entered query.
- Cancellation stops outstanding repository work and stale responses do not
  replace results for a newer query.

## Scenarios

- **GIVEN** a task editor has focus, **WHEN** the user presses
  **Cmd/Ctrl+Shift+F**, **THEN** task content search opens and the browser or
  editor find UI does not.
- **GIVEN** the palette is open with a query, **WHEN** the user clicks another
  top-level mode or presses **Tab**, **THEN** the mode changes, the query remains,
  and the input keeps focus.
- **GIVEN** the user leaves the active task workbench, **WHEN** the palette is
  opened or a workspace-search shortcut is pressed, **THEN** only
  **Commands** is offered and the workspace-search shortcut keeps its native
  behavior.
- **GIVEN** a line containing an exact occurrence and another containing only a
  fuzzy subsequence, **WHEN** both match the query, **THEN** the exact occurrence
  ranks first.
- **GIVEN** a task with two repositories containing the same search term,
  **WHEN** the user searches, **THEN** both repositories have clearly separated
  result groups with repository-relative paths.
- **GIVEN** a task with two repositories containing matching file names,
  **WHEN** the user searches in **Files** mode, **THEN** matching paths from both
  repositories appear with their repository prefixes.
- **GIVEN** two repositories with the same relative path, **WHEN** the user
  selects the result from the second repository, **THEN** Kandev opens the
  second repository's file at the returned line and column.
- **GIVEN** an untracked non-ignored text file and an ignored text file,
  **WHEN** both contain the query, **THEN** only the untracked non-ignored file
  appears.
- **GIVEN** a preview containing non-BMP Unicode before and inside a match,
  **WHEN** results render, **THEN** the supplied match ranges highlight the
  intended characters without splitting a surrogate pair.
- **GIVEN** a search is followed quickly by another query, **WHEN** the first
  response arrives last, **THEN** the palette retains the second query's
  results.

## Out of scope

- Replacing file-name and path search.
- Searching tasks other than the task currently open.
- Searching Git history, ignored files, generated artifacts, agent messages,
  terminal output, or files outside attached repositories.
- Regular-expression, whole-word, case-sensitive, replace, or persistent search
  options.
- A persistent full-text index or external search service.
