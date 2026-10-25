---
status: active
system: workspaces
created: 2026-10-18
owners:
  - kandev
---

# Hosted Editor Workspace Requirements

## Overview

Opening a selected task workspace in a configured hosted editor must identify
that workspace as a folder. Workspaces owns the reusable resolved-root target
contract; Tasks retains session/worktree selection and UI retains presentation.
The existing [editor containment contract](editor-file-containment.md) continues
to govern lexical admission independently.

A root target is an omitted or empty file target, or a target that resolves
lexically to the selected workspace root. A hosted launch succeeds at Kandev's
boundary when it returns the requested URL. It does not prove that an external
IDE loaded, authenticated, or rendered.

## Requirements

### REQ-WORKSPACES-HOSTED-EDITOR-001: Hosted workspace root opening

**Intent:** Open the requested workspace through the existing editor action.

#### Acceptance criteria

- **AC-WORKSPACES-HOSTED-EDITOR-001.1:** For an eligible hosted editor and a
  resolved root target, the returned URL shall contain `folder` equal to the
  selected absolute workspace root. For an ordinary configured URL without
  preselected target parameters, it shall contain no `file` parameter. Empty,
  dot, and normalized root-equivalent targets shall have the same outcome;
  requested line and column shall not be appended to the folder value.
- **AC-WORKSPACES-HOSTED-EDITOR-001.2:** Explicit worktree identity and its
  session association identity shall each target that selected worktree. A
  missing explicit selection shall fail without falling back to another root.
  With no explicit selection, existing first-worktree and single-repository
  local-root fallback shall retain their target identity and eligibility rules.
- **AC-WORKSPACES-HOSTED-EDITOR-001.3:** Contained non-root file requests shall
  retain Kandev's existing hosted `file` value and positive line/column suffix
  behavior. Ordinary configured URL paths and non-target query values shall
  retain their meaning. Spaces, hash signs, plus signs, and Unicode in paths
  shall remain opaque path data after URL decoding.
- **AC-WORKSPACES-HOSTED-EDITOR-001.4:** Missing session/workspace/editor,
  disabled editor, invalid configuration, and escaped targets shall retain
  their existing error categories and return no launch URL. Embedded root
  launches shall retain `internal://vscode` and their executor capability gate.
- **AC-WORKSPACES-HOSTED-EDITOR-001.5:** Existing desktop and phone editor
  root actions using the shared opener shall receive the same selected-root
  folder URL. Existing selection, loading, and visible failure behavior shall
  remain effective without changes to layout, touch interaction, or copy.

## Out of scope

Nested folder API semantics, vendor-specific file schemes or preselected
workspace URL precedence, filesystem type inference or stat, remote path
mapping, SSH/native launch policy, embedded editor lifecycle, editor discovery
or installation, frontend/API fields, and lexical/security boundary redesign.
No live hosted-editor rendering claim is required.

## Implementation plan

- [Hosted editor workspace](../../../plans/hosted-editor-workspace/plan.md)
