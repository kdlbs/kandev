---
id: "02-single-scope-git-status-fallback"
title: "Fall back to sole non-root repository scope for detailed Git status"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PLUGIN-002
  - REQ-PLATFORM-GIT-CAPTURE-RECOVERY-001
  - REQ-AGENTS-NATIVE-CODE-REVIEW-001
acceptance_criteria:
  - AC-EXECUTORS-PLUGIN-002.4
  - AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.5
  - AC-AGENTS-NATIVE-CODE-REVIEW-001.7
system_design:
  - ../../specs/executors/system-design/remote-executor-plugins.md
  - ../../specs/platform/system-design/workspace-git-capture-recovery.md
  - ../../specs/agents/system-design/native-code-review-recovery.md
---

# Task 02: Fall back to sole non-root repository scope for detailed Git status

## Summary

Update `agentctl.Client.GetGitStatusWithDetails` so that when the root workspace returns `repository_unavailable` and `agentctl` registers exactly one non-root repository scope, the client re-reads detailed Git status with `?repo=<scope>`. This restores base-commit capture and cumulative diffs for single-repository `plugin_remote` sessions. Preserve the selected scope on uncommitted review files so findings match the corresponding repository-scoped Changes diff.

## In scope

- In `apps/backend/internal/agent/runtime/agentctl/git.go`:
  - Add `gitStatusErrorNoRepository = "repository_unavailable"`.
  - Extract `getGitStatusWithDetails(ctx context.Context, repo string) (*GitStatusResult, error)` sending `GET /api/v1/git/status?fresh=true&details=wait` (with `&repo=` + `url.QueryEscape(repo)` when `repo != ""`) and applying the existing HTTP status and detail-readiness checks.
  - Extract `soleNonRootRepositoryScope(ctx context.Context) (string, bool)` fetching `/api/v1/git/turn-changes/scopes` via `c.fetchJSONResult` and returning `(scopes[0], true)` only on HTTP 2xx with `len(scopes) == 1 && scopes[0] != ""`.
  - Update `GetGitStatusWithDetails(ctx)` to call `c.getGitStatusWithDetails(ctx, "")`, and only when `err != nil && result != nil && result.ErrorCode == gitStatusErrorNoRepository`, check `soleNonRootRepositoryScope(ctx)` and re-read `c.getGitStatusWithDetails(ctx, scope)`. Return the original `(result, err)` whenever no sole non-root scope is resolved.
- In `apps/backend/internal/agent/handlers/review_change_source.go`, copy a non-empty `GitStatusResult.RepositoryName` onto each uncommitted file value before returning it to review collection.
- Add focused regression tests:
  - `apps/backend/internal/agent/runtime/agentctl/git_status_single_scope_test.go` covering root-repo (1 request), bare-root single non-root scope success, bare-root single non-root scope with scoped `details_unavailable` failure, bare-root 0 scopes, bare-root 2+ scopes, bare-root scopes HTTP 404, and non-`repository_unavailable` errors (`details_unavailable`, HTTP 500).
  - `apps/backend/internal/agent/handlers/git_handlers_single_scope_test.go` exercising `GitHandlers.wsCumulativeDiff` against a fake agentctl HTTP server in root-repo, bare-root single non-root scope, bare-root multi-scope, and bare-root zero-scope layouts when persisted `base_commit_sha` is empty.
  - `apps/backend/internal/agent/handlers/review_change_source_test.go` exercising single-scope status fallback through `review.CollectChanges` and asserting the resulting repository/path key.

## Out of scope

- Changing `agentctl` server HTTP routes or response schemas (`/api/v1/git/status`, `/api/v1/git/turn-changes/scopes`, `/api/v1/git/cumulative-diff`).
- Changing `Client.GetGitStatus`, `Client.GetGitStatusFresh`, or multi-repository `cumulative_diff` behavior when more than one scope is registered.
- Changing turn-change checkout projection (owned by Task 01).

## Acceptance

1. When root `GET /api/v1/git/status?fresh=true&details=wait` succeeds, `Client.GetGitStatusWithDetails` makes exactly one request and never calls `/api/v1/git/turn-changes/scopes`.
2. When root status returns `error_code == "repository_unavailable"` and `/api/v1/git/turn-changes/scopes` returns exactly one non-empty scope `["<repo>-<branch>"]`, `Client.GetGitStatusWithDetails` fetches `GET /api/v1/git/status?fresh=true&details=wait&repo=<repo>-<branch>`, returning the scoped enriched `GitStatusResult` (or the scoped error when scoped enrichment is unavailable), and `GitHandlers.wsCumulativeDiff` resolves `base_commit` and returns a non-null `cumulative_diff`. For review collection, each uncommitted file retains that scope so its key matches the repository-scoped Changes diff.
3. When scopes are empty, contain more than one entry, contain `""`, or fail to load — or when root status fails with any error other than `repository_unavailable` — `Client.GetGitStatusWithDetails` returns the original root result and error without guessing a repository, and `wsCumulativeDiff` preserves `{"cumulative_diff": null}`.

## Verification

Write the client and handler regression tests first and confirm they fail before the fix (`git status details unavailable: repository_unavailable` and `cumulative_diff == nil` for the bare-root single-scope layout). After implementing the fix, run from the repository root:

```bash
(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/agentctl -run 'TestGetGitStatusWithDetails|TestGitStatus' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agent/handlers -run 'TestWsCumulativeDiff|TestReviewChangeSource' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/orchestrator/executor -run 'TestExecutorBaseCaptureRecovery' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agent/handlers -run '^TestReviewChangeSourceCollectsScopedFilesWithRepositoryIdentity$' -count=1)
bash scripts/lint-go-changed \
  apps/backend/internal/agent/runtime/agentctl/git.go \
  apps/backend/internal/agent/runtime/agentctl/git_status_single_scope_test.go \
  apps/backend/internal/agent/handlers/git_handlers_single_scope_test.go \
  apps/backend/internal/agent/handlers/review_change_source.go \
  apps/backend/internal/agent/handlers/review_change_source_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
make -C apps/backend build
```

## Files likely touched

- `apps/backend/internal/agent/runtime/agentctl/git.go`
- `apps/backend/internal/agent/runtime/agentctl/git_status_single_scope_test.go` (new)
- `apps/backend/internal/agent/handlers/git_handlers_single_scope_test.go` (new)
- `apps/backend/internal/agent/handlers/review_change_source.go`
- `apps/backend/internal/agent/handlers/review_change_source_test.go`

## Dependencies

None.

## Risks

- Using `c.fetchJSONResult` (`c.httpClient`) for `/api/v1/git/turn-changes/scopes` instead of `c.longRunningHTTPClient` keeps the fallback bounded under the normal HTTP client timeout and avoids nil-client failures in minimal HTTP-only test setups.
- Matching `result.ErrorCode == "repository_unavailable"` strictly (and requiring `len(scopes) == 1 && scopes[0] != ""`) prevents masking real repository detail failures (`details_unavailable`, timeouts, HTTP 5xx) or guessing among multiple repositories.

## Parallelism

`sequential`

## Inputs

- [Fix plan](plan.md), Defect 2 root cause and technical approach.
- [Remote executor plugins system design](../../specs/executors/system-design/remote-executor-plugins.md).
- [Workspace Git capture recovery system design](../../specs/platform/system-design/workspace-git-capture-recovery.md), Consumers and errors.
- [Native Code Review recovery system design](../../specs/agents/system-design/native-code-review-recovery.md), scoped review file identity.
- Existing client and handler test patterns in `apps/backend/internal/agent/runtime/agentctl/git_test.go` and `apps/backend/internal/agent/handlers/git_handlers_test.go`.

## Results

Implemented the single-non-root-scope fallback in `agentctl.Client.GetGitStatusWithDetails`. It retries detailed status only when the root response reports `repository_unavailable` and the scope endpoint returns exactly one non-empty path; all other root errors and ambiguous or unavailable scope results retain the root response.

- Red: the focused client test failed on the single-scope fallback and proved no scope lookup occurred; the handler test returned `cumulative_diff: null` for the single-scope layout.
- Client: `(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/agentctl -run 'TestGetGitStatusWithDetails|TestGitStatus' -count=1)` passed.
- Handler: `(cd apps/backend && go test -trimpath -race ./internal/agent/handlers -run 'TestWsCumulativeDiff|TestReviewChangeSource' -count=1)` passed.
- Executor: `(cd apps/backend && go test -trimpath -race ./internal/orchestrator/executor -run 'TestExecutorBaseCaptureRecovery' -count=1)` passed.
- Lint: `bash scripts/lint-go-changed apps/backend/internal/agent/runtime/agentctl/git.go apps/backend/internal/agent/runtime/agentctl/git_status_single_scope_test.go apps/backend/internal/agent/handlers/git_handlers_single_scope_test.go` passed with 0 issues.
- Documentation: `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check` passed.
- Build: `make -C apps/backend build` passed.

PR review remediation: `ReviewChangeSource` now copies a named status scope onto
uncommitted file payloads before review collection. The regression first failed
because the collected key was bare-path, then passed after the fix.

- Regression: `(cd apps/backend && go test -trimpath -race ./internal/agent/handlers -run '^TestReviewChangeSourceCollectsScopedFilesWithRepositoryIdentity$' -count=1)` passed.
- Handler: `(cd apps/backend && go test -trimpath -race ./internal/agent/handlers -run 'TestWsCumulativeDiff|TestReviewChangeSource' -count=1)` passed.
- Agentctl: `(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/agentctl -run 'TestGetGitStatusWithDetails|TestGitStatus' -count=1)` passed.
- Executor: `(cd apps/backend && go test -trimpath -race ./internal/orchestrator/executor -run 'TestExecutorBaseCaptureRecovery' -count=1)` passed.
- Lint: `bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/turn_changes.go apps/backend/internal/agent/runtime/lifecycle/manager_execution.go apps/backend/internal/agent/runtime/lifecycle/manager_turn_changes.go apps/backend/internal/agent/runtime/lifecycle/turn_changes_plugin_remote_test.go apps/backend/internal/agent/runtime/agentctl/git.go apps/backend/internal/agent/runtime/agentctl/git_status_single_scope_test.go apps/backend/internal/agent/handlers/git_handlers_single_scope_test.go apps/backend/internal/agent/handlers/review_change_source.go apps/backend/internal/agent/handlers/review_change_source_test.go` passed with 0 issues.
- Documentation: `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.test.py`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check -- docs/specs docs/decisions` passed.
- Build: `make -C apps/backend build` passed.
