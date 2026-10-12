---
status: active
system: ui
created: 2026-10-06
owners:
  - kandev
---

# Embedded editor file targets

## Overview and ownership

An embedded-editor file action must dispatch the file identity and cursor
position resolved for that action without interpreting filename characters as
coordinates. UI owns this reusable editor-target handoff, shared by file-tree
and file-action consumers. Workspaces retain path resolution and filesystem
authority; Tasks retain session lifecycle.

This is independent of [file-tree path scope](file-tree-path-scope.md), which
governs file-tree state and reads, [editor availability](embedded-vscode-executor-availability.md),
and the task [folder shortcut](../../tasks/requirements/open-task-folder.md).
Those contracts do not specify the exact-file embedded-editor handoff.

## Terms

- **Resolved target:** The canonical file path returned after the existing
  session/worktree resolution, with the effective line and column.
- **Dispatch:** Submission of that target to the existing embedded-editor
  open-file interface. Dispatch does not establish native editor rendering.
- **Supported filename:** A filename admitted by existing path resolution in a
  supported execution environment. Literal colons are covered for POSIX names;
  this does not establish native Windows embedded-editor support.

## Requirements

### REQ-UI-EMBEDDED-EDITOR-TARGET-001: Preserve the resolved file target

**Intent:** Prevent a file action from selecting another file or cursor position
because the filename contains a delimiter or encoded-looking text.

#### Acceptance criteria

- **AC-UI-EMBEDDED-EDITOR-TARGET-001.1:** When a supported file action succeeds,
  dispatch shall preserve the complete resolved file path. A colon inside a
  filename, including a numeric suffix such as `notes:2026` without a cursor
  target, shall not become a coordinate or truncate the filename.
- **AC-UI-EMBEDDED-EDITOR-TARGET-001.2:** Dispatch shall preserve a positive line
  and its positive column independently of filename characters. Without a
  positive line, both effective coordinates shall be zero; a positive line
  without a positive column shall use column zero.
- **AC-UI-EMBEDDED-EDITOR-TARGET-001.3:** Supported filenames containing spaces,
  Unicode, literal percent sequences, plus signs, or query-like punctuation
  shall reach dispatch unchanged after resolution. Encoded-looking filename
  text shall not undergo an additional decoding pass.
- **AC-UI-EMBEDDED-EDITOR-TARGET-001.4:** Actions without a file and explicitly
  marked directory actions shall retain their existing panel behavior without
  issuing an open-file dispatch, including repository-less folder-level actions.
- **AC-UI-EMBEDDED-EDITOR-TARGET-001.5:** A rejected session, worktree, path, or
  editor request shall issue no open-file dispatch. Existing error feedback,
  editor availability, external-editor behavior, and session/worktree selection
  rules shall remain intact. A valid file dispatch shall use the returned
  canonical target rather than substitute the caller's unnormalized path.
- **AC-UI-EMBEDDED-EDITOR-TARGET-001.6:** Existing desktop and phone consumers
  shall share the same target interpretation without changes to composition,
  copy, navigation, touch interaction, scrolling, or breakpoint behavior.

## Exclusions

Native code-server rendering and CLI filename interpretation, universal Windows
support, new editor discovery/settings/capability policy, multi-repository runtime
root remapping, public API/schema fields, generic URI/security handling, request
or toast lifetime changes, concurrency/cache/transport policy, and unrelated
dot-prefix path validation changes are outside this contract.
