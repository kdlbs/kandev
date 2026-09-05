---
id: "01-github-commit-detail"
title: "GitHub commit detail contract"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/pr-only-commit-details.md"
---

# Task 01: GitHub commit detail contract

- **Acceptance:** The backend exposes one workspace-authorized
  `github.pr_commit.get` action for an exact owner/repository/SHA; the response
  contains GitHub commit metadata, exact aggregate statistics, and changed-file
  patches merged across provider pages; `PRCommitInfo.stats_available`
  distinguishes unavailable list statistics from known zeroes; unauthorized
  repositories, unsafe SHAs, upstream errors, and missing commits return errors
  without touching a task worktree.
- **Verification:** Follow strict TDD. First add parser, client, and
  handler/service regressions and run
  `cd apps/backend && go test -run 'Test.*(PRCommitDetail|CommitDetail|PRCommitStats)' ./internal/github`;
  record failures showing the absent action/model and zero-value ambiguity.
  After the minimal implementation, rerun that command, then run
  `cd apps/backend && go test ./internal/github` and `git diff --check`.
- **Files likely touched:**
  `apps/backend/pkg/websocket/actions.go`;
  `apps/backend/internal/github/models.go`;
  `apps/backend/internal/github/client.go`;
  `apps/backend/internal/github/client_helpers.go`;
  `apps/backend/internal/github/gh_client.go`;
  `apps/backend/internal/github/pat_client.go`;
  `apps/backend/internal/github/noop_client.go`;
  `apps/backend/internal/github/mock_client.go`;
  `apps/backend/internal/github/service_pr.go`;
  `apps/backend/internal/github/handlers.go`; and focused tests alongside those
  files, including authorization coverage.
- **Dependencies:** None.
- **Parallelism:** sequential — the model, client interface, authorization
  service, and WebSocket handler form one wire contract and their RED/GREEN
  changes overlap the same package.
- **Inputs:** repair spec API contract; current `PRCommitInfo`, `PRFile`,
  `GetPRCommits`, `GetPRFiles`, `ensureRepositoryInWorkspaceScope`, personal
  read-client resolution, and GitHub handler authorization test patterns.
- **Output contract:** Report the final request/response shape, exact files
  changed, RED/GREEN and full-package results, pagination behavior,
  authorization/error behavior, confirmation that no worktree or persistence
  path was added, and synchronized task/plan status.

## Results

- RED: parser, client, authorization, and WebSocket regressions exposed the
  missing individual-commit contract and the zero-value ambiguity for list
  statistics.
- GREEN: added `github.pr_commit.get`, workspace/repository authorization,
  exact-SHA validation, shared PAT/CLI parsing, paginated file merging with
  provider-order deduplication, and mock/noop support. List commits now set
  `stats_available: false`; individual details carry exact metadata,
  statistics, and files.
- Verification: `go test ./internal/github` passed with 1081 tests; the
  focused parser/client/service/handler tests and `git diff --check` passed.
- Individual commit details remain read-only and are never imported into a
  task worktree or persisted.
