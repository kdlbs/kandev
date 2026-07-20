---
id: "01-linear-diff-enrichment"
title: "Linear diff enrichment"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/workspace-git-status.md"
---

# Task 01: Linear diff enrichment

## Acceptance

- One request-local accumulator tracks emitted diff bytes across unstaged, staged, and untracked enrichment without rescanning the complete files map inside per-file loops.
- Every changed path remains in the result after total-budget exhaustion, and files not enriched retain `budget_exceeded`.
- Per-file filesystem and diff work stops when the context is cancelled, returning cancellation rather than successful partial status.
- Existing file-size, per-file output, total output, binary, truncation, overshoot, and carry-forward behavior remains unchanged.

## Verification

```bash
cd apps/backend
rtk go test ./internal/agentctl/server/process -run 'Test(DiffBudget|EnrichUntrackedFileDiffs|GetGitStatus)'
rtk go test ./internal/agentctl/server/process -run '^$' -bench 'BenchmarkEnrichUntrackedFileDiffs' -benchmem
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/workspace_git_diff.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_diff_test.go`

## Dependencies

None.

## Inputs

- Spec requirements for complete metadata, bounded diff content, linear enrichment, and prompt cancellation.
- Existing diff-limit and skip-reason tests in `workspace_git_diff_test.go`.

## Output contract

Report the budget representation, cancellation checkpoints, benchmark or operation-count evidence, changed files, targeted test results, and remaining risks. Do not truncate the changed-path list or alter API payloads.
