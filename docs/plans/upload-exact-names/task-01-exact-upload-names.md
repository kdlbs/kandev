---
id: "01-exact-upload-names"
title: "Preserve exact upload names"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-WORKSPACE-FILE-TRANSFER-001
  - REQ-UI-WORKSPACE-FILE-TRANSFER-003
  - REQ-UI-WORKSPACE-FILE-TRANSFER-004
acceptance_criteria:
  - AC-UI-WORKSPACE-FILE-TRANSFER-001.3
  - AC-UI-WORKSPACE-FILE-TRANSFER-001.4
  - AC-UI-WORKSPACE-FILE-TRANSFER-001.8
  - AC-UI-WORKSPACE-FILE-TRANSFER-003.1
  - AC-UI-WORKSPACE-FILE-TRANSFER-004.1
  - AC-UI-WORKSPACE-FILE-TRANSFER-004.2
  - AC-UI-WORKSPACE-FILE-TRANSFER-004.4
  - AC-UI-WORKSPACE-FILE-TRANSFER-004.6
system_design:
  - ../../specs/ui/system-design/workspace-file-transfer.md
---

# Task 01: Preserve exact upload names

## Summary

Repair the shared agentctl upload path interpretation so preflight and file publication preserve
valid whitespace-bearing names. Independently reproduce the reported router failure with a new
regression before changing production code.

## In scope

- `scopedUploadPath` and `sanitizeUploadSegment` in the agentctl upload API.
- Authenticated preflight and multipart router tests against disposable real filesystem paths.

## Out of scope

- Permission and overwrite policy changes, broader path APIs, separator conventions, UI copy,
  unrelated uploads, and support for entirely whitespace relative paths.

## Acceptance

- Both routes use the exact valid name; empty `dir` means root, whitespace-only `dir` names a folder,
  and empty or entirely whitespace relative paths are rejected. Return paths match writes.
- Exact and unspaced collisions are distinguished; ordinary upload, unresolved conflict, replace,
  and keep-both preserve existing behavior, including a conflict arising after preflight.
- Empty, traversal, absolute and existing symlink escape rejection continue without writes outside
  allowed roots. Existing normalization and native filesystem restrictions remain unchanged.

## Verification

Add `TestWorkspaceUploadExactNames` in the file below, run the first command before the fix and
record the expected causal failures, then run both commands after the fix. Run from repository root:

```bash
(cd apps/backend && GOMAXPROCS=2 GOTOOLCHAIN=local GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -p 1 -timeout 10m ./internal/agentctl/server/api -run 'TestWorkspaceUploadExactNames|TestHandleFileUpload|TestHandleUploadPreflight' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agentctl/server/api/workspace_upload.go apps/backend/internal/agentctl/server/api/workspace_upload_exact_names_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/upload-exact-names apps/backend/internal/agentctl/server/api
git status --short -- docs/plans/upload-exact-names
```

Use the shared heavy lease before implementation tests or lint. Run no broad replay. Probe native
filesystem support for the exact names and skip only unsupported name cases. Preserve test result
output and document actual command results when complete.

## Files likely touched

- `apps/backend/internal/agentctl/server/api/workspace_upload.go`
- `apps/backend/internal/agentctl/server/api/workspace_upload_exact_names_test.go`
- This work order and `plan.md` for execution status and results.

## Dependencies

None.

## Risks

- `relative_path` made solely of whitespace is already rejected by the public upload path, even if
  the filesystem accepts it. Preserve that rule; do not silently change it while fixing `dir`.
- Preflight does not reserve the destination; final conflict checks remain necessary.

## Parallelism

`sequential`

## Inputs

- [Workspace file transfer requirements](../../specs/ui/requirements/workspace-file-transfer.md)
- [Workspace file transfer system design](../../specs/ui/system-design/workspace-file-transfer.md)
- Existing upload router tests and `WorkspaceTracker` path resolution.

## Results

- Independent router regression before the fix: exit 1 with seven causal failures; ordinary and
  invalid-path controls passed.
- Targeted Go command above: passed, including replacement, keep-both, late conflict, and existing
  symlink containment tests. All new exact-name cases ran on this host.
- Changed-Go lint: passed with its default memory limit. Documentation catalog and specification
  lint passed; `git diff --check` passed. The fresh workspace dependency installation passed.
- Public documentation is unchanged: existing pages describe folder upload as a copy flow but do
  not specify filename normalization. This correction restores the intended selected-name behavior
  without changing user instructions, interface copy, or screenshots.
