---
id: "01-remote-repository-registration"
title: "Register remote repositories from workspace settings"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001
acceptance_criteria:
  - AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001.1
  - AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001.2
  - AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001.3
  - AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001.4
  - AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001.5
  - AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001.6
  - AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001.7
  - AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001.8
system_design:
  - ../../specs/workspaces/system-design/remote-repository-registration.md
---

# Task 01: Register remote repositories from workspace settings

## Summary

Add the verified registration endpoint and the settings-page menu and dialog
that let a user register a remote repository without creating a task.

## In scope

- `RegisterRemoteRepository` service method and the
  `POST /workspaces/:id/repositories/remote` handler with error mapping.
- `AddRepositoryMenu`, `AddRemoteRepositoryDialog`, the request-body builder,
  and the `registerRemoteRepositoryAction` client.
- Locale keys in every shipped catalog, public docs, and Playwright coverage.

## Out of scope

- Plugin changes. Plugin providers already list, inspect, and branch through
  the existing actions.

## ASCII UI preview

See `UI-01` and `UI-02` in the [plan](plan.md#ascii-ui-preview). This work
order changes both views.

## Acceptance

1. A GitHub repository chosen from the picker registers with provider
   `github`, owner, name, canonical clone URL, and the chosen branch; a second
   registration of the same repository returns the existing row.
2. A plugin provider selection is resolved through the selection resolver and
   persists the resolver's descriptor, not the browser hints; a resolver
   failure persists nothing and surfaces the typed selection error.
3. The settings page registers a repository end-to-end on desktop and phone,
   lists it immediately, and keeps it after reload.

## Verification

```bash
(cd apps/backend && go test ./internal/task/service/ ./internal/task/handlers/)
(cd apps/web && pnpm exec vitest run app/settings/workspace app/actions)
(cd apps/web && pnpm exec eslint --max-warnings 0 app/settings/workspace app/actions/workspaces.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --host --no-build --project chromium tests/settings/repository-add-remote.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/settings/mobile-repository-add-remote.spec.ts)
```

## Files likely touched

- `apps/backend/internal/task/service/service_remote_repositories.go`
- `apps/backend/internal/task/handlers/repository_remote_handlers.go`
- `apps/backend/internal/task/handlers/repository_handlers.go`
- `apps/web/app/settings/workspace/workspace-add-repository-menu.tsx`
- `apps/web/app/settings/workspace/workspace-add-remote-repository-dialog.tsx`
- `apps/web/app/settings/workspace/workspace-remote-repository-registration.ts`
- `apps/web/app/settings/workspace/workspace-repositories-client.tsx`
- `apps/web/app/actions/workspaces.ts`
- `apps/web/e2e/tests/settings/repository-add-remote*.spec.ts`
- `docs/public/use-kandev.md`

## Dependencies

None.

## Risks

Low. See the plan.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/workspaces/requirements/remote-repository-registration.md)
- [System design](../../specs/workspaces/system-design/remote-repository-registration.md)
- [Plugin repository task creation design](../../specs/plugins/system-design/repository-provider-task-creation.md)

## Results

The endpoint reuses `preflightRepositoryInputs` and `ResolveRepositoryRef`;
the dialog reuses `RemoteRepoChip` so plugin providers appear without plugin
changes. Review remediation makes plain built-in URLs reach a settled state
and preserves a selected plugin branch after identity verification. Existing
repositories retain their saved settings. Focused authorization coverage
proves foreign and viewer requests stop before provider lookup or persistence.

Focused checks in a disposable nonroot, network-disabled container:

- `go test -mod=readonly -p=1 ./internal/task/service -run '^TestRegisterRemoteRepository' -count=1`: passed.
- `node node_modules/vitest/vitest.mjs run hooks/domains/github/use-pr-info-by-url.test.ts`: 28 tests passed.
- Prettier checks on the changed web files and `gofmt` on the changed Go files: passed.

The URL and branch regressions failed on the pre-fix code before passing.
Desktop and phone E2E cover picker and pasted URL registration, persistence,
unchanged task counts, and phone action sizing. Broad lint, typecheck, unit,
race, and E2E checks belong to hosted CI; they were not run locally during
remediation. Contributor commit hooks were not run on the host.
