---
id: "01-capture-mixed-change-regression"
title: "Capture mixed-change regression"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
acceptance_criteria:
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.9
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.10
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.12
system_design:
  - ../../specs/platform/system-design/workspace-git-status.md
---

# Task 01: Capture mixed-change regression

## Summary

Create permanent backend, desktop, and mobile tests that reproduce the confirmed `MM` failure before
production changes. Record red results showing that the staged facet is missing and the diff is
combined.

## In scope

- Add a focused backend mixed-status test file.
- Add one desktop and one mobile Playwright regression using a real tracked file.
- Capture the expected red reason for each test before implementation.

## Out of scope

- Production contract or UI changes.
- Broad E2E suite execution.

## Acceptance

- The backend test expects independent staged and unstaged facets from one path.
- Both browser tests expect the same path in both sections and layer-specific diff content.
- Before correction, each targeted test fails only because current code collapses the two layers.

## Verification

```bash
cd apps/backend
go test ./internal/agentctl/server/process -run 'TestWorkspaceGitStatusPreservesMixedChangeFacets'

cd apps/web
pnpm e2e:run tests/git/git-changes-panel.spec.ts -- --grep "same path in staged and unstaged sections"
pnpm e2e:run --project mobile-chrome tests/task/mobile-changes-panel.spec.ts -- --grep "same path in staged and unstaged sections"
```

The commands are expected to fail at this red checkpoint. Save the precise assertion failures in the
work-order Results section before continuing.

## Files likely touched

- `apps/backend/internal/agentctl/server/process/workspace_git_mixed_changes_test.go`
- `apps/web/e2e/tests/git/git-changes-panel.spec.ts`
- `apps/web/e2e/tests/task/mobile-changes-panel.spec.ts`

## Dependencies

None.

## Risks

- E2E setup must mutate only its disposable fixture worktree and wait on observable status updates.

## Parallelism

`sequential`

## Inputs

- Requirement acceptance criteria `.9`, `.10`, and `.12`.
- Existing workspace Git process tests and desktop/mobile Changes E2E fixtures.
- Manual reproduction evidence from `MM README.md` and `AM mixed-stage-panel-test.txt`.

## Results

- `go test -json ./internal/agentctl/server/process -run '^TestWorkspaceGitStatusPreservesMixedChangeFacets$' -count=1` failed at the intended assertion: the `FileInfo` wire had one combined `+2` diff and no `staged_change` facet.
- Desktop Chromium failed at the intended assertion: `staged-files-section-collapse-toggle` was absent.
- Mobile Chrome failed twice at the same intended assertion: `staged-files-section-collapse-toggle` was absent.
