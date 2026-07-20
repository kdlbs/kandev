---
id: "01-workspace-connection"
title: "Workspace-scoped GitLab connection"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/gitlab-integration/spec.md"
---

# Task 01: Workspace-Scoped GitLab Connection

## Acceptance

- Each workspace reads, writes, tests, deletes, and health-checks exactly one
  GitLab host/auth configuration. All GitLab clients resolve from the
  authoritative workspace ID, and valid `http://`/`https://` self-managed
  origins preserve their configured scheme through every provider path.
- Legacy global host/token data migrates idempotently to the active or earliest
  workspace; copying a connection copies a PAT but no watches or task links.
- The GitLab settings and browse pages switch connections with the active
  workspace and never display or use another workspace's status.

## Verification

```bash
cd apps/backend && rtk go test ./internal/gitlab/... ./internal/backendapp/...
cd apps && rtk pnpm --filter @kandev/web test -- --run lib/api/domains/gitlab-api.test.ts components/gitlab/gitlab-settings.test.tsx components/integrations/integration-copy-config.test.ts hooks/domains/gitlab/use-gitlab-status.test.ts
cd apps/web && rtk pnpm run typecheck
```

## Files Likely Touched

- `apps/backend/internal/gitlab/models.go`
- `apps/backend/internal/gitlab/store.go`
- `apps/backend/internal/gitlab/store_config.go` (new)
- `apps/backend/internal/gitlab/store_config_test.go` (new)
- `apps/backend/internal/gitlab/service.go`
- `apps/backend/internal/gitlab/service_config.go` (new)
- `apps/backend/internal/gitlab/service_config_test.go` (new)
- `apps/backend/internal/gitlab/copy.go` (new)
- `apps/backend/internal/gitlab/factory.go`
- `apps/backend/internal/gitlab/factory_test.go`
- `apps/backend/internal/gitlab/provider.go`
- `apps/backend/internal/gitlab/provider_test.go`
- `apps/backend/internal/gitlab/controller.go`
- `apps/backend/internal/gitlab/controller_config.go` (new)
- `apps/backend/internal/gitlab/controller_config_test.go` (new)
- `apps/backend/internal/gitlab/controller_watches.go`
- `apps/backend/internal/gitlab/handlers.go`
- `apps/backend/internal/gitlab/poller.go`
- `apps/backend/internal/gitlab/service_search.go`
- `apps/backend/internal/gitlab/service_watches.go`
- `apps/backend/internal/gitlab/service_issue_watches.go`
- `apps/backend/internal/backendapp/services.go`
- `apps/backend/internal/backendapp/gitlab_service_test.go`
- `apps/web/lib/types/gitlab.ts`
- `apps/web/lib/api/domains/gitlab-api.ts`
- `apps/web/lib/api/domains/gitlab-api.test.ts`
- `apps/web/components/gitlab/gitlab-settings.tsx`
- `apps/web/components/gitlab/gitlab-settings.test.tsx`
- `apps/web/components/integrations/integration-copy-config.ts`
- `apps/web/components/integrations/integration-copy-config.test.ts`
- `apps/web/components/integrations/integrations-menu.tsx`
- `apps/web/hooks/domains/gitlab/use-gitlab-status.ts`
- `apps/web/hooks/domains/gitlab/use-gitlab-status.test.ts` (new)
- `apps/web/app/gitlab/gitlab-page-client.tsx`

## Dependencies

None.

## Inputs

- Spec: `What` workspace connection bullets, `gitlab_configs`, Connection API,
  Connection health, Permissions, Persistence guarantees.
- ADR: `docs/decisions/0030-workspace-scoped-integration-settings.md`.
- Patterns: `apps/backend/internal/jira/{store.go,service.go,provider.go,copy.go,handlers.go}`,
  `apps/backend/internal/linear/`, and
  `apps/web/components/integrations/integration-copy-config.ts`.
- Constraint: keep `GITLAB_TOKEN` as an explicit environment fallback only;
  never serialize a secret value or authenticated URL.
- Constraint: do not upgrade or reject an existing `http://` self-managed host;
  normalize origin/trailing slash while preserving its scheme.

## Output Contract

Report the config schema/migration, client resolution boundary, compatibility
surface removed or retained, frontend workspace behavior, files changed, tests
run, blockers, and residual credential-isolation risks. Mark this task `done`
and update `plan.md` only after targeted verification passes.
