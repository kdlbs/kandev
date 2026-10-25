---
id: "02-batch-comparison-refs"
title: "Resolve comparison refs in one Git process"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-GIT-CAPTURE-RECOVERY-001
acceptance_criteria:
  - AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.7
  - AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.2
system_design:
  - ../../specs/platform/system-design/workspace-git-capture-recovery.md
---

# Task 02: Resolve comparison refs in one Git process

## Summary

Each comparison evidence read resolves its distinct safe references with one
`git rev-parse` process and falls back to the existing per-reference path
whenever the batch cannot be mapped exactly.

## In scope

1. Add tests in `agentctl/server/process` before the change, using a real
   temporary repository and the existing `SetGitEnvironment` + `GIT_TRACE`
   pattern to count commit resolutions:
   - With base and ahead/behind refs at `origin/main` plus distinct
     remote-head and comparison refs, one evidence read runs one `rev-parse`
     (baseline: four) and returns the same object IDs.
   - A missing reference yields the same values as separate resolution.
   - Unsafe references stay empty and never reach Git; an object-ID-shaped
     reference that does not exist stays empty.
   - A cancelled read yields empty values without an error.
   - `batchedRefOIDs` rejects truncated, extra, echoed, and empty output.
2. Add `gitRefOIDs`, used by `captureGitStatusComparisonEvidence`. It keeps
   the existing allowlist inline next to its `runGitOutput` call, resolves
   duplicate refs once, and uses `gitRefOID` directly for a single ref.
   It accepts a batch only on zero exit with exactly one object-ID line per
   argument; otherwise it uses per-reference resolution. A cancelled context
   returns empty values without the fallback.
3. Keep the evidence struct, comparison, and error contract unchanged.

## Out of scope

Other `rev-parse` callers, branch identity reads, enrichment, admission class,
command deadlines, and the corrective capture policy.

## Acceptance

- The process-count test fails on the baseline and passes after the change;
  value-equivalence tests pass on both.
- Existing capture recovery tests still pass.

## Verification

```bash
(cd apps/backend && go test -trimpath ./internal/agentctl/server/process -run '^Test(GitStatusComparisonEvidence|BatchedRefOIDs)' -count=1)
(cd apps/backend && go test -trimpath ./internal/agentctl/server/process -run '^TestWorkspaceTrackerGitStatusCaptureRecovery' -count=1)
(cd apps/backend && go build ./... && go vet ./internal/agentctl/server/process/...)
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/workspace_git_status_capture.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_status_comparison_evidence_test.go`

## Dependencies

None.

## Risks

`rev-parse` without `--verify` echoes unresolvable arguments and stops at the
first failure; the exact-mapping rule and fallback keep values identical.

## Results

Implemented on 2026-10-12 against `add376a2af`.

- RED (baseline `workspace_git_status_capture.go`, new behavior tests):
  `TestGitStatusComparisonEvidenceResolvesRefsInOneProcess` failed with four
  traced `rev-parse --verify '<ref>^{commit}'` calls. The missing, unsafe and
  unknown-object, and cancelled tests passed on the baseline, which confirms
  they pin unchanged values. The `batchedRefOIDs` table test was added after
  the RED run because the helper does not exist on the baseline.
- GREEN: `^Test(GitStatusComparisonEvidence|BatchedRefOIDs)` passes (one
  traced commit resolution for the batch case).
- Neighbours: `TestWorkspaceTrackerGitStatusCaptureRecovery*` pass except
  `KeepsSharedDeadline`, which fails the same way with the baseline file
  (`basic captures = 1`). `UsesReplacementIdentity` failed once in the
  combined run under load and passed three times alone; it resolves a single
  ref, so it does not take the batched path. The two POSIX-shell wrapper
  tests skip on Windows.
- `go build ./...` and `go vet ./internal/agentctl/server/process/` pass.

Review remediation on 2026-10-13: batch parsing removes one optional LF
terminator and one CR per record rather than discarding every trailing record.
Extra blank LF/CRLF records failed the new regression before the fix. The exact
parser and production test table passed `go test -trimpath -race -v -count=1`
in a network-disabled stdlib-only sandbox. SHA-256 and unterminated full IDs
remain accepted; abbreviated and echoed output remains rejected. Full tracker,
concurrent capture, cancellation, and process validation runs in hosted CI.
