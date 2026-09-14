---
id: "07-file-transfer-e2e"
title: "Workspace file transfer end-to-end coverage"
status: in_progress
wave: 6
depends_on:
  - "02-editor-download-actions"
  - "06-upload-entry-points"
plan: "plan.md"
requirements:
  - REQ-UI-WORKSPACE-FILE-TRANSFER-001
  - REQ-UI-WORKSPACE-FILE-TRANSFER-002
  - REQ-UI-WORKSPACE-FILE-TRANSFER-004
acceptance_criteria:
  - AC-UI-WORKSPACE-FILE-TRANSFER-001.4
  - AC-UI-WORKSPACE-FILE-TRANSFER-001.5
  - AC-UI-WORKSPACE-FILE-TRANSFER-002.2
  - AC-UI-WORKSPACE-FILE-TRANSFER-002.5
  - AC-UI-WORKSPACE-FILE-TRANSFER-004.3
  - AC-UI-WORKSPACE-FILE-TRANSFER-004.4
  - AC-UI-WORKSPACE-FILE-TRANSFER-004.5
system_design:
  - ../../specs/ui/system-design/workspace-file-transfer.md
---

# Task 07: Workspace file transfer end-to-end coverage

## Summary

Prove both halves work through the real stack. The unit tests own the boundaries; this owns the
integration evidence the user-facing requirements need.

## Scope

- A new `apps/web/e2e/tests/task/workspace-file-transfer.spec.ts` covering five scenarios: flat
  upload, conflict resolution, cancel, folder upload, and download from the unpreviewable-file
  screen.
- `setInputFiles` against the hidden inputs rather than simulating an OS dialog.

## Exclusions

- Any production change. If a scenario cannot pass, fix the owning work order rather than weakening
  the assertion.
- Folder download.

## Acceptance

- Uploading files through the create menu makes them appear in the tree at the destination without a
  manual refresh, and uploading a folder recreates its structure at the right relative paths.
- The conflict dialog lists exactly the conflicts, and each resolution behaves correctly: Replace
  overwrites, Keep both produces the `-1` name beside an untouched original, Skip writes nothing, and
  cancel writes nothing at all including the non-conflicting files.
- The unpreviewable-file screen exposes the download control and downloads bytes matching the source.

## Verification

```bash
cd apps/web && pnpm e2e:raw e2e/tests/task/workspace-file-transfer.spec.ts
```

## Files likely touched

- `apps/web/e2e/tests/task/workspace-file-transfer.spec.ts`

## Dependencies

Tasks 02 and 06. Both halves must be rendered.

## Parallelism

Sequential.

## Inputs

- Requirements: the three user-facing requirements and their acceptance criteria.
- Existing patterns: `apps/web/e2e/tests/` conventions, with `file-tree-multi-select.spec.ts` as the
  nearest neighbour for tree interaction.
- `apps/web/e2e/README.md` for project selection and gating.

## Risks

- Asserting on a toast that auto-dismisses is flaky. Prefer asserting the resulting tree node, and
  treat the confirmation as a secondary check.
- The cancel scenario is the one most worth getting right, because its assertion is an absence.
  Assert that the non-conflicting file is also absent, not just the conflicting one.
- Binary download assertions need the Playwright download event and a byte comparison, not a
  file-name check.

## Output contract

Report the scenarios covered, files changed, exact commands and results, then
mark this task `done` and update its checkbox in `plan.md`.

## Results

Spec written with five scenarios: flat upload, conflict resolution, cancel-writes-nothing, folder
upload, and download from the unpreviewable screen. It typechecks, lints, and passes the e2e sleep
ratchet.

**The spec has NOT been executed.** `pnpm e2e:raw -- <path>` does not forward the path filter, so two
attempts silently ran the entire suite instead. Correct invocation is
`pnpm e2e:raw e2e/tests/task/workspace-file-transfer.spec.ts` with no `--`. Running it also needs
`make -C apps/backend e2e-plugin-package`, a current backend build, and `make build-web`.

**A real defect was found while capturing PR media and fixed here:** `setInputFiles` on a
`webkitdirectory` input requires a directory path, not file descriptors. The folder scenario had
passed a synthetic file named `bundle/nested/leaf.txt`, which would have failed on first run. It now
builds a real directory on disk and points the input at it.

Every scenario this spec asserts was instead exercised manually against a live dev instance through
the real UI, driven by Playwright (see the manual verification section in `plan.md`). That is
evidence the feature works; it is not a substitute for the automated coverage, which still needs one
green run.
