---
status: active
system: workspaces
created: 2026-10-06
owners:
  - kandev
---

# Editor File Containment Requirements

## Overview

Editor opening must distinguish a filename beginning with dots from a parent
directory component. A legitimate file such as `..notes.go` or
`..notes/inside.go` inside the selected worktree must be usable through the
existing editor action.

Workspaces owns this reusable contract because it defines which file targets
belong to a resolved worktree before any editor integration receives them.
Tasks retains session/worktree selection; UI retains editor presentation and
file-tree identity. This is separate from directory-browser visibility and
embedded-editor availability.

## Terms

- **Contained target:** A target whose normalized lexical location is the
  selected worktree or one of its descendants, using the backend host's native
  path semantics.
- **Parent escape:** A normalized location outside that worktree reached by a
  complete parent-directory component. A name merely beginning with `..` is
  not a parent-directory component.
- **Editor acceptance:** Producing the existing integration target or accepting
  dispatch. It does not prove a native application or embedded panel rendered.

## Requirements

### REQ-WORKSPACES-EDITOR-CONTAINMENT-001: Preserve contained editor targets

**Intent:** Open legitimate worktree files while retaining the existing lexical
boundary against parent escapes.

#### Acceptance criteria

- **AC-WORKSPACES-EDITOR-CONTAINMENT-001.1:** For an otherwise eligible editor
  request, a contained file or directory name beginning with `..` shall be
  accepted like an ordinary name. This includes both `..notes.go` and a file
  inside `..notes`. The returned target shall retain the requested file identity
  and any requested line and column.
- **AC-WORKSPACES-EDITOR-CONTAINMENT-001.2:** Ordinary contained targets, names
  beginning with one or more dots, and paths that normalize to contained targets
  shall retain their existing behavior. This includes a contained `.` component
  and an internal parent component that normalizes back inside the worktree.
- **AC-WORKSPACES-EDITOR-CONTAINMENT-001.3:** A true parent escape, including the
  parent itself, a parent descendant, and a normalized nested escape, shall
  return the existing invalid-editor-configuration failure and no integration
  target. A rejected request shall not dispatch an editor.
- **AC-WORKSPACES-EDITOR-CONTAINMENT-001.4:** Empty file targets and unavailable
  worktree targets shall retain existing folder and missing-workspace outcomes.
  Session selection, editor eligibility, and their existing error categories
  shall remain effective before target acceptance.

## Exclusions

- Symlink resolution, physical filesystem containment, or a new security policy.
- New absolute-path, volume, URI, native CLI colon, or foreign-platform syntax.
- Embedded-editor target encoding or frontend parsing.
- Editor discovery, new editor integrations, session lifetime, API fields,
  transport changes, UI composition, and localized copy.

## Implementation plan

- [Contained editor dot paths](../../../plans/editor-contained-dot-paths/plan.md)
