---
id: "04-workflowsync-provider-dispatch"
title: "Workflow sync provider dispatch"
status: done
wave: 2
depends_on: ["02-workflowsync-config-provider"]
plan: "plan.md"
spec: "../../specs/gitlab-workflow-sync/spec.md"
---

# Task 04: Workflow Sync Provider Dispatch

## Acceptance

1. `workflowsync` defines the provider-neutral `RepoEntry` and splits
   `ClientProvider` into `GitHubClientProvider` and `GitLabClientProvider`
   exactly as the spec's `## API Surface` → `### Backend` section states.
   `Service` holds both, either may be nil.
2. `fetchFiles` dispatches on `cfg.Provider`: GitHub uses owner/name and adapts
   `github.RepoContentEntry` into `RepoEntry`; GitLab uses `cfg.ProjectPath`
   and maps tree types (`blob` → file, `tree` → dir). File selection
   (`isSyncableFile`), ordering, hashing, parsing, and apply are unchanged and
   shared across providers.
3. A nil client for the *configured* provider yields a provider-specific,
   actionable error — the hardcoded "GitHub is not authenticated" at
   `service.go:184` becomes conditional — and the failure is recorded through
   the existing `recordFailure` path with `last_hash` cleared.

## Verification

```bash
cd apps/backend && go test ./internal/workflowsync/... -race
```

## Files Likely Touched

- `apps/backend/internal/workflowsync/service.go` — `RepoEntry`, the two
  interfaces, `Service` fields, `NewService`, `fetchFiles`.
- `apps/backend/internal/workflowsync/provider.go` — `Provide` signature.
- `apps/backend/internal/workflowsync/service_test.go`.

## Inputs

- Spec `## API Surface` → `### Backend`, and `## Failure Modes`.
- `service.go:182-207` — current `fetchFiles`.
- Task 02 supplies `cfg.Provider` and `cfg.ProjectPath`.

## Risks

- Keeping `github.RepoContentEntry` in the shared path would leave
  `workflowsync` GitHub-coupled. Adapt at the provider boundary, not downstream.
- The per-workspace mutex must still be held across the GitLab fetch, matching
  the documented concurrency guarantee at `service.go:47-53`.

## Output Contract

`SyncWorkspace` fetches from whichever provider the config names, with unchanged
parse/apply/reconcile semantics and provider-appropriate errors. Tests cover
both dispatch branches and both nil-client branches. DI wiring is task 05.
