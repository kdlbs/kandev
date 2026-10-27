---
status: active
system: ui
created: 2026-10-27
owners:
  - kandev
---

# Tool file reference opening

## Overview and ownership

A person inspecting an agent's read or search result should be able to open a
referenced file even when its basename has no extension. UI owns this shared
interaction for normalized tool-card file references. Tasks retain session
ownership, and Workspaces retain file resolution and filesystem authority.
The separate [file-tree path scope](file-tree-path-scope.md) contract governs
workspace aliases, tree state and read-line identity after activation.

## Terms

A **file reference** is a path presented as a read-file target or an entry in
an expanded code-search result. A search's scope path is not a file reference.
An **explicit directory** here is `.` or a path ending in `/` after surrounding
whitespace is inspected. A reference without a directory marker does not prove
that the underlying filesystem entry is a file.

## Requirements

### REQ-UI-TOOL-FILE-OPEN-001: Open tool file references

**Intent:** Make file references usable regardless of filename extension while
preserving the existing editor and session boundaries.

#### Acceptance criteria

- **AC-UI-TOOL-FILE-OPEN-001.1:** When an opener is available, activating a
  supported read-file reference or expanded search-result file reference with
  a nonblank, non-directory-marked path shall invoke the existing file-open
  action, whether or not its basename contains a dot.
- **AC-UI-TOOL-FILE-OPEN-001.2:** Activation shall preserve the referenced path
  through existing read-selector and workspace-path resolution, and a positive
  read line shall be handed to the existing session-scoped cursor request. Search
  result activation shall retain its existing opener and ownership behavior.
- **AC-UI-TOOL-FILE-OPEN-001.3:** Blank references, explicit directories and
  references without an opener shall remain inactive. Search scope paths shall
  remain noninteractive. Failed or missing files shall retain the existing
  file-opening error behavior.
- **AC-UI-TOOL-FILE-OPEN-001.4:** Desktop and phone users shall be able to
  activate the same references by their existing click or touch interaction and
  reach the existing editor or phone file viewer without an added navigation step.

## Exclusions

Filesystem probing or new type metadata, provider or protocol changes, read
selector grammar changes, generalized path classification, editor ownership or
error policy changes, new copy, and public feature documentation are outside
this contract. An unmarked directory or stale result may be offered as an open
attempt; the existing file service decides whether it can be opened.
