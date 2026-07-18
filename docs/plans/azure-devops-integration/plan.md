# Azure DevOps Integration Plan

## Scope

Implement the read-only Azure DevOps Services integration defined in
[`../../specs/azure-devops-integration/spec.md`](../../specs/azure-devops-integration/spec.md):
workspace-scoped PAT configuration, direct REST work-item and pull-request
reads, persistent task PR links, and responsive settings/browse surfaces. No
runtime path may require `gh` or `az`.

## Architecture

- Add an independent `internal/azuredevops` package. Do not add Azure methods to
  `github.Client` or translate Azure records into GitHub API structs.
- Reuse Jira's workspace-scoped config/secret/health patterns and GitLab's
  source-host REST/task-review patterns.
- Persist provider-native Azure identifiers. Normalize only the summary fields
  required by shared task UI.
- Use Azure DevOps REST API 7.1, an injected HTTP client for deterministic
  tests, bounded response bodies, context-aware requests, and typed API errors.
- Register the service as non-fatal during backend boot and expose mock routes
  only under `KANDEV_MOCK_AZURE_DEVOPS=true`.

## Backend Touch Points

- New package: `apps/backend/internal/azuredevops/`.
- Service wiring: `apps/backend/internal/backendapp/services.go`,
  `helpers.go`, and `main.go` where pollers are started.
- Repository provider parsing/discovery where provider enums are currently
  restricted to GitHub/GitLab.
- Runtime defaults: `profiles.yaml` for the E2E mock selector only.
- Workspace cleanup and task/repository validation through existing service
  interfaces rather than integration-specific SQL outside the new package.

## Frontend Touch Points

- Typed API and types under `apps/web/lib/api/domains/azure-devops-api.ts` and
  `apps/web/lib/types/azure-devops.ts`.
- Domain hooks under `apps/web/hooks/domains/azure-devops/`.
- Settings route and integration menu entry.
- `/azure-devops` browse page with a compact work-item/PR segmented view,
  desktop filter rail, and mobile filter sheet.
- Task PR summary integration through a provider-tagged view model; Azure
  detail remains in Azure-specific components.
- No required action may be hover-only or desktop-only.

## Tests

- Go table tests for URL validation, PAT headers, API errors, WIQL batching,
  PR conversion, workspace isolation, persistence, and route status codes.
- Go service tests for repository/task association validation and restart
  persistence.
- TypeScript unit tests for API request/response normalization and pure filter
  or status helpers.
- Playwright desktop and `mobile-chrome` flows using the Azure mock controller:
  connect, browse work items, browse PRs, and open PR feedback.
- Security review of secret isolation, URL/SSRF validation, response-size
  bounds, and error redaction before final verification.

## Verification

- `rtk make -C apps/backend fmt`
- `rtk go test ./internal/azuredevops/...` from `apps/backend`
- `rtk make -C apps/backend test`
- `rtk make -C apps/backend lint`
- `rtk pnpm --filter @kandev/web typecheck` from `apps`
- `rtk pnpm --filter @kandev/web test -- --run` from `apps`
- `rtk pnpm --filter @kandev/web lint` from `apps`
- `rtk pnpm e2e -- --project=chrome e2e/azure-devops/azure-devops.spec.ts` from `apps/web`
- `rtk pnpm e2e -- --project=mobile-chrome e2e/azure-devops/mobile-azure-devops.spec.ts` from `apps/web`

## Risks

- Azure organization URLs are an outbound-request boundary. V1 accepts only
  canonical HTTPS `dev.azure.com/<organization>` URLs to avoid an SSRF-capable
  arbitrary host setting.
- WIQL returns references rather than hydrated work items and Azure caps batch
  retrieval at 200 IDs; ordering and partial omissions require explicit tests.
- Azure reviewer votes and branch policies do not map one-to-one to GitHub
  reviews and checks. Only summary states are shared.
- Existing task PR UI is GitHub-heavy. The implementation must extract only the
  smallest provider-tagged presentation contract required for Azure, not begin
  a broad GitHub/GitLab refactor.

## Task Waves

Wave 1: backend foundation

- [x] [Task 01: Workspace configuration](task-01-workspace-configuration.md)
- [x] [Task 02: REST client](task-02-rest-client.md)

Wave 2: backend product reads

- [x] [Task 03: Work-item and PR services](task-03-read-services.md)
- [x] [Task 04: Task PR persistence and backend wiring](task-04-task-pr-wiring.md)

Wave 3: frontend

- [x] [Task 05: Frontend data and settings](task-05-frontend-settings.md)
- [x] [Task 06: Responsive browse and task PR UI](task-06-frontend-browse.md)

Wave 4: integrated validation

- [x] [Task 07: E2E, security review, and documentation](task-07-e2e-security-docs.md)

Tasks within a wave are listed separately for ownership clarity but should run
sequentially in the current workspace when they touch the same package or state
composition files.
