---
status: active
system: ui
created: 2026-09-21
owners:
  - kandev
---

# File Tree Path Scope Requirements

## Overview

Agent tool messages can identify a file by its absolute host path, while the task Files tree and
`workspace.tree.get` address nodes relative to the task workspace root. Users must be able to open
an agent-referenced workspace file without that absolute identity entering file-tree expansion,
persistence, or refresh state and producing repeated failed tree requests.

The UI system owns this contract because it owns file-open identity, Files-panel expansion state,
and the requests derived from that state. The workspace file service remains the final validation
boundary for tree reads. Its separate read-only absolute-file content contract remains unchanged.

## Terminology

- **Workspace file path:** A normalized path relative to the active task workspace root, using `/`
  separators and containing no parent traversal. Literal colons remain valid path characters unless
  the value has an absolute URI or Windows drive form.
- **Workspace absolute alias:** An absolute path contained beneath the active task workspace root
  on a complete path-segment boundary.
- **External absolute path:** An absolute path outside the active task workspace. It can be opened
  read-only under ADR 0016, but it has no node identity in the task Files tree.
- **Tree request path:** The `path` supplied to `workspace.tree.get`. The empty string addresses the
  root; every other value is a workspace file path for a directory.

## Requirements

### REQ-UI-FILE-TREE-PATH-SCOPE-001: Keep file-tree paths workspace-relative

**Intent:** Opening files referenced by an agent must not corrupt file-tree state or cause repeated
backend errors.

**User story:** As a user inspecting an agent-referenced file, I want the file and Files tree to use
the active task workspace consistently, so that opening or refreshing the file does not issue
invalid directory requests.

#### Acceptance criteria

- **AC-UI-FILE-TREE-PATH-SCOPE-001.1:** When an agent read or edit message identifies a file by a
  workspace absolute alias, activating that file shall open the corresponding workspace file path
  and preserve any requested line target.
- **AC-UI-FILE-TREE-PATH-SCOPE-001.2:** When the active editor displays an external absolute path,
  the Files tree shall leave its expansion state unchanged and shall not attempt to reveal, restore,
  or refresh that path. The existing read-only external file content behavior shall remain
  available.
- **AC-UI-FILE-TREE-PATH-SCOPE-001.3:** Every tree request derived from active-file reveal,
  persisted expansion, or a workspace refresh shall use either the empty root path or a normalized
  workspace file path. Absolute filesystem paths, absolute URI forms, Windows drive prefixes, and
  paths containing parent traversal shall not cross the frontend tree-request boundary. A literal
  colon in an otherwise canonical relative path shall remain valid.
- **AC-UI-FILE-TREE-PATH-SCOPE-001.4:** When stored expansion state contains an invalid tree path,
  the Files tree shall discard that path and its derived ancestors before hydration or refresh,
  while retaining valid expanded paths for the same task environment.
- **AC-UI-FILE-TREE-PATH-SCOPE-001.5:** The backend tree endpoint shall reject an absolute or
  escaping request path as a validation failure before filesystem lookup. A rejected client path
  shall not be reported as an operational ERROR log.
- **AC-UI-FILE-TREE-PATH-SCOPE-001.6:** Desktop and phone shall keep their existing editor and file
  viewer compositions. A workspace absolute alias shall display and reopen with its workspace file
  path on both, with no new control or navigation step.

## Scenarios

- **GIVEN** the task root `/workspace` and an agent read of
  `/workspace/public/assets/case.json`, **WHEN** the user activates the read card, **THEN** the editor
  opens `public/assets/case.json` and the Files tree may reveal only that relative path.
- **GIVEN** a read-only editor for `/opt/reference/guide.md`, **WHEN** the Files panel is mounted or a
  refresh event arrives, **THEN** the editor remains usable and the Files tree does not expand or
  request `/opt` or any descendant.
- **GIVEN** stored expansion entries `src`, `src/components`, and `/home/user/project`, **WHEN** the
  Files tree restores, **THEN** it restores the two valid entries and removes the absolute entry
  without requesting it.
- **GIVEN** the relative directory `config:dev`, **WHEN** the user expands or refreshes it, **THEN**
  the Files tree requests `config:dev` without treating the literal colon as a URI scheme.
- **GIVEN** a direct `workspace.tree.get` request with `/home/user/project`, **WHEN** the backend
  validates it, **THEN** it returns a validation response without constructing
  `<workspace-root>/home/user/project`, calling `stat`, or emitting an ERROR log.

## Out of scope

- Removing the read-only absolute-file content behavior accepted by ADR 0016.
- Mapping an external host directory into the task Files tree.
- Changing file-tree layout, row styling, editor chrome, phone navigation, or localized copy.
- Changing workspace source aliases used by Markdown links.

## Implementation plan

- [File tree path scope](../../../plans/file-tree-path-scope/plan.md)
