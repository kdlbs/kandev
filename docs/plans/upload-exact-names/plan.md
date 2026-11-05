---
created: 2026-10-27
status: implemented
requirements:
  - REQ-UI-WORKSPACE-FILE-TRANSFER-001
  - REQ-UI-WORKSPACE-FILE-TRANSFER-003
  - REQ-UI-WORKSPACE-FILE-TRANSFER-004
system_design:
  - ../../specs/ui/system-design/workspace-file-transfer.md
legacy_specs: []
---

# Implementation Plan: Exact Workspace Upload Names

## Overview

Preserve meaningful whitespace in workspace upload names through preflight and publication. One
sequential work order repairs agentctl path interpretation and verifies the authenticated routes
against a disposable filesystem. The Files-panel transfer contract owns the outcome; workspace
mutation code retains path and containment authority.

## Scope

### In scope

- Exact destination and relative names, preflight agreement, and existing conflict resolutions.
- Existing empty and all-whitespace relative-path rejection and containment behavior.
- Reject CR/LF in upload `dir`, `relative_path` and `repo` at preflight and write boundaries.

### Out of scope

- Permissions, overwrite policy, backslash conventions, broad path APIs, UI copy, other uploads,
  and lifecycle behavior. No rendered UI changes.

## Technical approach

The provided router reproduction observed successful writes at trimmed names and a false preflight
conflict; an ordinary-name control passed. In `apps/backend/internal/agentctl/server/api/workspace_upload.go`,
`sanitizeUploadSegment` trims the entire fragment, and `scopedUploadPath` treats a whitespace-only
`dir` as root. Preserve raw whitespace before existing separator/path validation; only `dir == ""`
means root. Retain the current all-whitespace `relative_path` rejection consistently in preflight
and write, as the task handler, agentctl client, and agentctl write validation already reject it.
Do not expand the shared path or repository-scoping contract. Continue using
`WorkspaceTracker.CheckUploadConflicts` and `WriteFileStream` with their existing containment and
final collision checks. The filesystem may reject names it does not support.

A Node 24.21.0 FormData probe serialized the text value `file\n` as the field bytes `file\r\n`;
preflight uses JSON while multipart fields are read and forwarded unchanged by the task handler and
agentctl client. This establishes a transport mismatch risk, not a browser-to-filesystem test. Reject
CR and LF in all three upload path metadata fields in the agentctl API before lookup or mutation,
including `dir` and `repo` on an empty preflight list. Keep ordinary spaces and tabs and the current
repository resolution unchanged; do not add encoding fields or normalize line endings.

## Tests

In `apps/backend/internal/agentctl/server/api/workspace_upload_exact_names_test.go`, add
`TestWorkspaceUploadExactNames` exercising the authenticated router and actual temporary filesystem.
Map AC-001.3, .4, .8 and AC-004.1, .2, .4, .6 to spaced filename, destination and nested
components, including a whitespace-only destination, response path and bytes; no false collision
with an unspaced sentinel; a true exact collision; unresolved conflict, replace, and keep-both
with original bytes retained as appropriate. Cover ordinary names as a control. Test empty and
all-whitespace relative paths in both routes, and empty destination as root. Map AC-003.1 to
absolute and traversal rejection and existing symlink containment cases. Exercise unsupported
native filename restrictions by probing fixture creation and skipping only affected cases, rather
than assuming POSIX naming rules on every host. Use distinct contents for competing targets.

For the corrective regression, test the authenticated preflight and multipart routes with LF, CR,
and CRLF in `dir`, `relative_path`, and `repo`, including an empty preflight list. Use distinct
near-identical LF and CRLF filesystem sentinels to show that the serialized upload cannot overwrite
a different target than the JSON preflight checked. Assert both requests reject and the sentinels
remain unchanged, including a forged direct upload with `replace`. Probe native filename support
only for sentinel cases; ordinary, tab, empty-list, and rejection controls must always run.

## E2E tests

Existing `apps/web/e2e/tests/task/workspace-file-transfer.spec.ts` in the `chromium` project covers
picker, folder upload, and conflict flows for AC-001.3, .4 and AC-004.1-.4. The targeted router
regression supplies exact-name filesystem evidence for AC-001.8; this backend-only correction does
not change browser behavior. The earlier authenticated router reproduction supplies design-phase
failure evidence, not a substitute for the independent permanent regression.

## Work orders

- [x] [Task 01: Preserve exact upload names](task-01-exact-upload-names.md)

## Verification results

The original exact-space implementation and CR/LF correction passed their targeted Go suite and
changed-Go lint; catalog, specification lint and diff checks passed. See the work order for the
independent regression and historical results.

## Risks

- Whitespace-only relative paths are rejected today at multiple upload layers even where the host
  filesystem permits them. This repair preserves that restriction; changing it is separate work.
- Preflight is advisory, so the write must still recheck a conflict created after preflight.
- Names containing CR or LF are deliberately unsupported for this upload contract, even on a
  filesystem that permits them. No browser end-to-end newline reproduction has been run.
