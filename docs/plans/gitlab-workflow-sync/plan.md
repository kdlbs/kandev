---
spec: docs/specs/integrations/requirements/gitlab-workflow-sync.md
created: 2026-08-06
updated: 2026-08-06
status: done
---

# GitLab Workflow Sync Plan

## Scope

Add a GitLab-backed source to workflow sync, alongside the existing GitHub one.
Backend: two new `gitlab.Client` methods, workspace-routed service wrappers, a
provider column plus `project_path` on the sync config, and provider dispatch in
`fetchFiles`. Frontend: a provider selector on the sync dialog and the matching
URL parse/build helpers. Everything downstream of fetching bytes is untouched.

## Current State

- `apps/backend/internal/workflowsync/service.go:32-39` — `ClientProvider` is
  typed against `github.RepoContentEntry`; `github.Service` is the only
  implementation.
- `apps/backend/internal/workflowsync/service.go:182-207` — `fetchFiles` calls
  it directly and hardcodes a GitHub-specific unauthenticated error at line 184.
- `apps/backend/internal/workflowsync/store.go:31-66` — self-contained schema
  with an idempotent `addPollEnabledColumn` migration precedent. No `provider`.
- `apps/backend/internal/workflowsync/models.go:38-110` — `Config`,
  `SetConfigRequest`, and `Normalize()` assume GitHub owner/name.
- `apps/backend/internal/backendapp/services.go:571-578` — wires `githubSvc` as
  the sole client provider.
- `apps/backend/internal/gitlab/client.go` — `Client` interface, ~30 methods,
  all `projectPath`-first. **No repository tree or file-content method.**
- `apps/backend/internal/gitlab/service_config.go:432-458` —
  `ClientForWorkspace` / `ClientForWorkspaceHost` already resolve a
  per-workspace, revision-cached client.
- `apps/web/components/settings/workflow-sync-dialog.tsx` and
  `apps/web/hooks/domains/settings/use-workflow-sync.ts` — GitHub-only, parse
  via `@/lib/utils/github-repo-url`.

## Architecture

`Config.Provider` selects one of two injected provider clients. Each interface
keeps its own upstream listing shape (`github.RepoContentEntry` /
`gitlab.RepoTreeEntry`) at the boundary, and `workflowsync` converts both to a
provider-neutral `dirEntry` inside its fetch loop, so no provider-typed value
leaks past that conversion. Credential and host resolution stay inside the
respective integration packages — `workflowsync` never touches tokens.

```
Config.Provider ──┬── "github" ─→ GitHubClientProvider ─→ github.Service ─→ App/PAT client
                  └── "gitlab" ─→ GitLabClientProvider ─→ gitlab.Service.ClientForWorkspace ─→ PAT/glab client
                                            │
                                            └─→ []RepoEntry ─→ existing parse → apply → reconcile
```

## Backend Touch Points

| File | Change |
| --- | --- |
| `internal/gitlab/models.go` | `RepoTreeEntry` type. |
| `internal/gitlab/client.go` | `ListRepoTree`, `GetRepoFileContent` on the interface. |
| `internal/gitlab/pat_client.go` | REST impl: tree (paginated) + raw file. |
| `internal/gitlab/mock_client.go` | Seedable in-memory impl. |
| `internal/gitlab/noop_client.go` | Not-configured stubs. |
| `internal/gitlab/service_repo_contents.go` *(new)* | Workspace-routed wrappers via `ClientForWorkspace`. |
| `internal/workflowsync/service.go` | `RepoEntry`, two provider interfaces, dispatch in `fetchFiles`, provider-aware errors. |
| `internal/workflowsync/models.go` | `Provider`, `ProjectPath`, provider-conditional `Normalize()`. |
| `internal/workflowsync/store.go` | Two `ADD COLUMN` migrations; select/scan/upsert updates. |
| `internal/workflowsync/provider.go` | `Provide` accepts both providers. |
| `internal/backendapp/services.go` | Pass `gitlabSvc` into `initWorkflowSyncService`. |

## Frontend Touch Points

| File | Change |
| --- | --- |
| `lib/types/workflow-sync.ts` | `provider`, `project_path` on config + request types. |
| `lib/utils/gitlab-repo-url.ts` *(new or extend existing)* | Parse/build GitLab project URLs incl. subgroups and self-managed hosts. |
| `hooks/domains/settings/use-workflow-sync.ts` | Provider state; route parse/build by provider. |
| `components/settings/workflow-sync-dialog.tsx` | Provider selector mirroring `REMOTE_REPOSITORY_PROVIDERS`; provider-aware placeholder and labels. |
| `apps/web/locales/**` | New i18n keys; no hardcoded copy (root `CLAUDE.md` i18n ratchet). |

## Tests

Backend, colocated `*_test.go`:

- `gitlab`: tree pagination, URL-encoding of nested `project_path`, raw file
  fetch, 404/403 mapping, mock and noop conformance to the interface.
- `workflowsync/models_test.go`: `Normalize()` matrix — provider default,
  invalid provider, GitHub rejects `project_path`, GitLab rejects
  owner/name, nested path accepted, `..`/empty-segment/leading-slash rejected.
- `workflowsync/store_test.go`: fresh-DB and replay migration, round-trip of
  both provider shapes, legacy row reads as `github`.
- `workflowsync/service_test.go`: dispatch to the right provider, nil-provider
  error text per provider, GitLab fetch → parse → apply, failure recording.

Frontend: `lib/utils/gitlab-repo-url.test.ts` (subgroups, self-managed hosts,
trailing `/-/tree/<branch>/<path>` forms) and a `use-workflow-sync` test for
provider switching.

E2E: not required — no new user-visible flow beyond a form field, and the
containers project does not cover workflow sync today.

## Verification

```bash
make -C apps/backend test
```
```bash
cd apps/backend && golangci-lint run ./... --new-from-rev="$(git merge-base HEAD origin/main)" --timeout=5m
```
```bash
cd apps/web && pnpm run typecheck && pnpm run lint && pnpm vitest run lib/utils lib/api hooks/domains/settings
```
```bash
cd apps && pnpm --filter @kandev/web run i18n:check && pnpm --filter @kandev/web run i18n:ratchet
```

## Risks

- **Reconcile-on-switch deletes workflows.** Switching provider makes the new
  file set authoritative; workflows absent from it are deleted. Spec'd
  behavior, but the dialog should warn on provider change. Mitigation: task 06
  includes a confirmation affordance.
- **Interface churn across four GitLab client impls.** Missing one breaks the
  build, not runtime. Low severity, caught by compilation.
- **`project_path` URL-encoding.** GitLab needs the full path percent-encoded
  as a single path segment; a naive join silently 404s on subgroups. Covered by
  an explicit nested-path test.
- **Tree pagination.** A directory with more than the default page size would
  silently truncate the synced set. Explicitly paginated and tested.
- **i18n ratchet.** New dialog copy must go through `t()` or the pre-commit
  ratchet fails.

## Task Waves

**Wave 1 — independent, parallel-safe**
- `task-01-gitlab-repo-content-client.md` — GitLab client methods + impls.
- `task-02-workflowsync-config-provider.md` — model + schema + validation.

**Wave 2 — depends on wave 1**
- `task-03-gitlab-workspace-repo-contents.md` — service wrappers (needs 01).
- `task-04-workflowsync-provider-dispatch.md` — interfaces + dispatch (needs 02).

**Wave 3 — integration**
- `task-05-wire-gitlab-provider.md` — DI wiring (needs 03, 04).

**Wave 4 — frontend**
- `task-06-frontend-provider-selector.md` — UI + types + i18n (needs 05).

## Status

| Task | Status |
| --- | --- |
| 01 gitlab-repo-content-client | done |
| 02 workflowsync-config-provider | done |
| 03 gitlab-workspace-repo-contents | done |
| 04 workflowsync-provider-dispatch | done |
| 05 wire-gitlab-provider | done |
| 06 frontend-provider-selector | done |
