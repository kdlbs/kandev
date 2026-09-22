---
created: 2026-09-21
status: implemented
requirements:
  - REQ-UI-FILE-TREE-PATH-SCOPE-001
system_design:
  - ../../specs/ui/system-design/file-tree-path-scope.md
legacy_specs: []
---

# Implementation Plan: File Tree Path Scope

## Overview

Keep agent-referenced workspace files and the task Files tree on one relative path identity, while
preserving the separate read-only absolute-file content contract. The implementation first
normalizes tool-originated paths before editor and cursor state diverge. It then validates every
source of file-tree state and rejects invalid direct requests at both backend boundaries.

## Confirmed root cause

- Agent read and edit metadata can contain `/home/jcfs/playground/mystery/...`, even though the
  current task workspace is already `/home/jcfs/playground/mystery`.
- `FilePathButton` displays the relative suffix but forwards the original absolute path.
- `useOpenFileAtLine` keys pending cursor and editor state with that unnormalized value.
- `FilesPanel` forwards the active editor path to `useFileTreeReveal`, which derives and persists
  `/home`, `/home/jcfs`, and every deeper absolute ancestor.
- A generic file refresh fans out one `workspace.tree.get` request for every expanded entry.
- `WorkspaceTracker.GetFileTree` joins each request to `workDir`; the absolute-looking request is
  therefore evaluated as `<workDir>/home/jcfs/...` and fails `stat`.
- `WorkspaceFileHandlers.wsGetFileTree` logs every agentctl failure at ERROR, so one contaminated
  expansion set produces the repeated stack traces in the incident.

## Scope

### In scope

- Normalize agent tool file paths contained by the active task workspace before editor, tab, and
  cursor state is written.
- Preserve absolute external file content reads without treating them as tree nodes.
- Validate reveal, restored expansion, and refresh-derived tree paths.
- Clean older invalid expansion state while retaining valid entries.
- Reject absolute and escaping tree paths before backend filesystem lookup or ERROR logging.
- Add focused frontend, backend, desktop, and phone regression coverage.

### Out of scope

- Changing ADR 0016 or restricting external read-only file content.
- Changing Files-panel or editor layout, controls, styling, copy, or navigation.
- Mapping arbitrary external directories into a task workspace.
- Changing Markdown source-checkout aliases or repository materialization.

## Technical approach

Add a pure frontend path module that owns containment and tree-path admission for POSIX and Windows
forms. Use it in the tool file opener before cursor state is keyed, then at reveal, restore, and
refresh boundaries. Keep external absolute opens unchanged and filter them only from tree state.

Validate `workspace.tree.get` in the WebSocket handler before execution lookup so invalid client
input returns a validation response without an ERROR log. Repeat the validation in agentctl before
joining or statting the path. Keep current behavior for valid relative directories and genuine
dependency failures.

## ASCII UI preview

### UI-01: Open a workspace file from an agent tool card

Entry point: an agent read or edit card whose recorded path is absolute. State: the file is inside
the active task workspace.

Current desktop behavior:

```text
Chat                         Files                         Editor
+----------------------+    +------------------------+   +---------------------------+
| Read public/...json  | -> | hidden state: /home/...|   | death-on-the-nile/case.json|
+----------------------+    +------------------------+   +---------------------------+
                                  |
                                  +-- refresh sends /home/... as tree paths
```

Proposed desktop behavior:

```text
Chat                         Files                         Editor
+----------------------+    +------------------------+   +---------------------------+
| Read public/...json  | -> | public                 |   | public/.../case.json      |
+----------------------+    |   assets               |   +---------------------------+
                            |     cases              |
                            |       death-on-the-nile|
                            +------------------------+
```

Proposed phone behavior:

```text
+--------------------------------+
| Chat                           |
| Read public/.../case.json      |
+--------------------------------+
              tap
               v
+--------------------------------+
| < Files   public/.../case.json |
|                                |
| existing native file viewer    |
+--------------------------------+
```

An external absolute file still opens in the existing editor or phone viewer, but the Files tree
does not move or store its ancestors. The path identity and unchanged navigation are requirements;
the spacing and example filenames are illustrative. No new copy or control is introduced.

## Work orders

- [done] [Task 01: Normalize Tool File Paths](task-01-normalize-tool-file-paths.md)
- [done] [Task 02: Enforce Tree Request Scope](task-02-enforce-tree-request-scope.md)

## Dependency order

```text
Task 01 -> Task 02
```

Task 01 establishes the shared path classifier and editor identity. Task 02 uses that classifier at
the Files-tree boundaries and adds the backend defense in depth.

## Verification strategy

- Pure frontend tests prove containment and canonical identity on POSIX and Windows paths.
- Hook tests prove editor open, pending cursor, and mounted-editor scroll use the same relative key.
- File-tree tests prove absolute active and stored paths never reach child loads or refresh requests.
- Backend process and handler tests prove validation happens before filesystem and lifecycle work and
  does not produce an ERROR log.
- A desktop Playwright scenario observes the absolute workspace path from a real seeded tool card,
  the relative editor result, and all file-tree request payloads.
- A phone Playwright scenario proves the same tool path and line target reach the existing native
  viewer without a composition change.
- Frontend typecheck/lint, backend test/lint, specification validators, and `git diff --check` cover
  the combined package.

## Risks

- String-prefix containment can misclassify `/workspace-old` as inside `/workspace`. The shared
  helper must enforce a complete path segment and test the collision.
- Normalizing only the editor open call while keeping the cursor key absolute would break
  scroll-to-line. Normalize before both operations.
- Rejecting all absolute paths in the file editor would regress ADR 0016. Tree admission and file
  content admission must remain separate.
- Filtering reveal alone leaves stale expansion entries available to generic refresh. Restore and
  refresh need their own guards.
- Backend validation only in the WebSocket handler leaves direct agentctl HTTP callers exposed.
  Validate again at `WorkspaceTracker.GetFileTree`.
- Windows drive, UNC, and Windows file URI paths need case-insensitive root containment, while POSIX
  paths remain case-sensitive.
- URI detection must distinguish absolute URI forms from repository-relative names containing
  literal colons, including `config:dev`.

## Package handoff

Implementation follows the two sequential TDD work orders. Begin each work order with its named
failing regression, keep the requirement and design draft until delivery is verified, and record
results in the work orders and this plan before creating the PR.

## Results

Implemented both work orders. Tool-originated workspace aliases now become canonical relative
editor identities before cursor state is written. Reveal, restore, refresh, the WebSocket handler,
and agentctl all reject invalid tree identities before issuing or executing a tree request.

PR review aligned UNC and Windows file URI containment with existing LSP case semantics, preserved
literal colons in canonical relative tree paths, shared the E2E workspace polling helper, and made
the agentctl regression assert the exported validation sentinel.

Validation completed with 90 focused frontend tests, frontend typecheck and changed-file lint,
focused backend tree and handler tests, backend lint, and one desktop plus one phone Playwright
scenario. The broad process package also exposed two unchanged process-runner fixture timing tests
that fail on this host; the affected `TestGetFileTree` tests pass independently.

Post-review verification passed the six-file focused frontend suite with 78 tests, typecheck,
changed-file ESLint, the workspace-path and handler Go tests, backend lint, all specification
validators, and the focused desktop and phone Playwright scenarios.
