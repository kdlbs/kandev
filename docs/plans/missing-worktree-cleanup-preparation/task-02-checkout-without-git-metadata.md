---
id: "02-checkout-without-git-metadata"
title: "Treat a checkout without Git metadata as absent"
status: done
wave: 1
depends_on:
  - "01-recover-preparation"
plan: "plan.md"
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
acceptance_criteria:
  - AC-TASKS-RUNTIME-CLEANUP-001.9
system_design:
  - ../../specs/tasks/system-design/runtime-cleanup-preparation.md
---

# Task 02: Treat a checkout without Git metadata as absent

## Summary

`CaptureCleanupHeadOIDs` treated any existing directory as a live checkout and
ran `git rev-parse HEAD` in it. When the checkout's `.git` file is gone but the
directory survives inside another repository, Git discovers that enclosing
repository and its HEAD is recorded as the worktree's cleanup identity. Only
checkout-local Git metadata may supply HEAD; otherwise the exact branch
inspection from Task 01 applies.

## In scope

- Add a capture test for a checkout directory without `.git` inside another
  repository before the change.
- Require checkout-local `.git` before reading HEAD, both before inspection and
  when the inspection fails.
- Pin the HEAD inspection to the checkout's own `.git` (`GIT_DIR`), so `.git`
  removed after the presence check cannot let Git reach an enclosing
  repository.
- Record the rule in the runtime cleanup preparation design.

## Out of scope

Branch inspection itself, cleanup audits, and other worktree states.

## Acceptance

- A checkout directory without `.git` records the worktree branch's commit, not
  the enclosing repository's HEAD.
- Existing capture tests keep their behavior.

## Verification

From the repository root:

```bash
(cd apps/backend && go test -trimpath ./internal/worktree -run 'TestCaptureCleanupHeadOIDs|TestRunCheckoutLocalGitInspect' -count=1)
(cd apps/backend && go vet -trimpath ./internal/worktree)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- apps/backend/internal/worktree/manager_cleanup.go
- apps/backend/internal/worktree/manager_cleanup_capture_test.go
- docs/specs/tasks/system-design/runtime-cleanup-preparation.md

## Dependencies

Task 01.

## Risks

Low: a checkout whose `.git` cannot be read for another reason now fails the
capture closed through the existing inspection-error classification.

## Parallelism

`sequential`

## Inputs

- REQ-TASKS-RUNTIME-CLEANUP-001 and AC-TASKS-RUNTIME-CLEANUP-001.9.
- The linked runtime cleanup preparation design.

## Results

- RED: before the change, `TestCaptureCleanupHeadOIDs_CheckoutWithoutGitMetadataUsesBranch`
  recorded the enclosing repository's HEAD.
- GREEN: the new test passes; `TestCaptureCleanupHeadOIDs_PathRemovedDuringInspection`
  uses a shell-script fake `git` that deletes its working directory. The author
  reported Windows failures with and without this change; this review did not
  independently execute that fixture on Windows. No required CI failure is
  waived on that report.
- Review follow-up: without the `GIT_DIR` pin,
  `TestRunCheckoutLocalGitInspect_DoesNotDiscoverEnclosingRepository` returned
  the enclosing repository's HEAD for a directory without `.git`; with it the
  inspection fails, and a linked worktree whose `.git` is a file still reports
  its own HEAD.

- Maintainer review: pin `GIT_DIR` relative to the Git command's checkout
  directory, so both absolute and relative recorded checkout paths work. The
  relative-path regression covers ordinary repositories and linked worktrees.
- A deterministic subprocess test removes only the linked checkout's `.git`
  after the presence check, leaves the directory inside a distinct enclosing
  repository, and checks that capture falls back to the worktree branch. This
  shell-script fixture runs on POSIX; it does not establish Windows behavior.
- Maintainer verification: contained copied-function proofs passed for the
  merged source, including the relative-path and deterministic metadata-removal
  regressions. These proofs use documented Manager, store, classifier, and
  subprocess-admission fakes; hosted CI supplies real-package verification.
- Protected-main static documentation validators passed for the merged tree:
  `scripts/list-docs.py validate` and `scripts/lint-spec-files.py --all`.
