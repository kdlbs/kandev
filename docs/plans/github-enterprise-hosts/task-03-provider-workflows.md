---
id: "03-provider-workflows"
title: "Host-bound provider workflows"
status: complete
wave: 3
depends_on: ["02-connection-lifecycle"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-ENTERPRISE-002
  - REQ-INTEGRATIONS-GITHUB-ENTERPRISE-003
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-002.5
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.1
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.2
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.3
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.4
system_design:
  - ../../specs/integrations/system-design/github-enterprise-hosts.md
---

# Task 03: Host-bound provider workflows

## Summary

Keep every external repository, issue, PR, and watch tied to its originating host. Complete the provider-facing operations that follow a successfully saved Enterprise connection.

## In scope

- Add/backfill immutable host scope to watches, associations, settings/presets, and projections where canonical repository identity is insufficient.
- Route browse/import, issue links, PR discovery/create/review/merge/checks, fork/comparison targets, and remote contributions through the resolved host.
- Namespace caches, singleflight, rate state, and association keys; reject host-mismatched actions and fence stale asynchronous results.
- Isolate optional GraphQL capabilities so older provider schemas do not disable basic PR reads.
- Add upgrade/replay/restart and same-name/same-ID cross-host tests on SQLite and PostgreSQL.

## Out of scope

No multi-host workspace routing, cross-host forks, frontend layout work, or changes to Kandev release URLs.

## Acceptance

1. Full resource workflows keep the configured host in API requests, canonical identities, links, and task associations; foreign-host URL input is rejected before network access.
2. Host replacement leaves original resources intact but ineligible for mismatched credentials, including delayed poll/callback results and equal resource names/IDs on two hosts.
3. Idempotent migration preserves valid public state and explicit origins, fails closed on ambiguity, and optional capability failures do not break core reads.

## Verification

Start at repository root. Read scoped AGENTS.md and use `/tdd` for changed
logic. New worktrees first run `(cd apps && pnpm install --frozen-lockfile)`.
Backend changes also run `bash scripts/lint-go-changed` with the explicit changed
Go file paths; frontend changes run ESLint on the explicit changed TS/TSX paths.
Record the expanded commands in Results. Do not mark skipped database tests as passed.

```bash
(cd apps/backend && go test ./internal/github -count=1)
(cd apps/backend && go test ./internal/github -run 'TestEnterpriseHostMigration|TestPostgresStoreSchemaReplay' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/github/store.go`, watch/task-association stores, `service_routing.go`, `workspace_settings_service.go`, and models.
- `apps/backend/internal/github/service_task_issue.go`, `service_remote_contribution.go`, `service_fork.go`, `service_comparison_target.go`, PR/review/issue services and pollers.
- `apps/backend/internal/github/graphql.go`, capability models, provider repository API and webhook automation consumers.
- `apps/backend/internal/github/enterprise_workflows_test.go`, `enterprise_capabilities_test.go`, `enterprise_store_test.go` (new), and existing `store_postgres_test.go`.

## Dependencies

02-connection-lifecycle. Follow the plan's acceptance order and authorized implementation lanes.

## Risks

Do not stamp old resources with the newly selected connection host. Existing optional GraphQL fields may make a whole query fail. PostgreSQL requires `KANDEV_TEST_POSTGRES_DSN`; report a skipped run explicitly.

## Parallelism

Parallel-safe only within the explicitly owned implementation lane in plan.md; integration acceptance remains dependency-ordered.

## Inputs

- [Requirements](../../specs/integrations/requirements/github-enterprise-hosts.md), using the exact IDs above.
- [System design](../../specs/integrations/system-design/github-enterprise-hosts.md) and [host/OAuth ADR](../../decisions/2026-10-14-github-enterprise-host-boundaries.md).
- Existing source and neighboring tests named in Files likely touched; the plan's test matrix defines new regression names.

## Results

Implementation is integrated. Resource host persistence, foreign-host rejection,
historical PR actions, settings/preset draft guards, optional GraphQL fields,
review/issue event publication and downstream reservation guards have passing
coverage. Serialized events retain host and workspace generation; reservation
revalidates them without holding a connection lock across event publication.

Root integration evidence:

- WebSocket feedback/files/commits/commit-detail dispatcher regressions reproduced
  the missing `expected_host` boundary and now pass with authorization ordering.
- Workflow sync `github_host` normalization/storage and invalid-host regressions
  were RED before implementation and are GREEN. Captured host/generation guards
  reject delayed results before the workflow applier. Save/reload preserves the host.
- `KANDEV_TEST_POSTGRES_DSN=<isolated PostgreSQL 16 DSN> GOMAXPROCS=2
  /usr/local/go/bin/go test -trimpath -p 1 -race ./internal/workflowsync -count=1`
  passed, including actual PostgreSQL schema replay and host round trips.
- Changed-Go lint on the eight changed workflow-sync Go files passed with zero issues.

The complete `internal/github` package, including PostgreSQL tests, passed on the
integrated production snapshot in 142.070 seconds. Combined changed-Go lint
passed with zero issues, and Enterprise race checks passed in 19.565 seconds.
Full persistence conformance with actual PostgreSQL and the race detector passed
in 273.558 seconds. SQL portability checks also passed.

PR remediation: paged and unpaged workspace PR/issue search responses now carry
the captured operation's `github_host`. Response copies preserve shared provider
and cache entries. JSON regressions cover GitHub.com, GHES, and GHE.com, including
a repeated cached read and provider-object immutability.

The complete GitHub package passed with actual PostgreSQL in 148.846 seconds.
`go test -race -trimpath -p 1 ./internal/github -run
'TestEnterprise(SearchResponsesCarryHost|Reconnect)' -count=1` passed in 3.775
seconds. Specification catalog and structure checks passed. Remote CI and review
remain pending until the fixup push.

GHE trial remediation (AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.1/003.2): task
creation still routes the dialog's legacy `github_url` through a public-only
parser. Correct the task-service parser and persisted origin, retain an explicit
GitHub provider hint for arbitrary Enterprise hosts, and verify Enterprise
origins against the workspace connection before provider probes or persistence.
Cover legacy and explicit payloads, GHES custom ports, SSH locators, GHE Cloud,
and mismatched/unconfigured origins with focused task-service/backendapp tests.
This repairs the existing host-isolation contract; no UI contract changes.
Repository registration also sends a bare `remote_url` without a provider
hint. Resolve that shape only against the configured workspace GitHub host;
unknown or mismatched origins remain rejected. Regression fixtures use
synthetic hosts and repository names. No diagnostic artifacts are committed.

Regression results: the task and registration preflight tests reproduced the
invalid-selection error before correction. Host-preserving creation, bare-URL
registration, foreign/unconfigured origin rejection before writes, public-host
identity separation, and provider binding passed focused race tests. Existing
public default-branch probing remains covered; Enterprise locators never use
the public-only probe. The legacy field continues accepting known providers.

Final remediation validation (2026-10-15), after the shared SSH constant cleanup:

```bash
cd apps/backend
GOMAXPROCS=2 go test -trimpath -p 1 -race ./internal/task/service ./internal/backendapp \
  -run 'Test(EnterpriseRepository|ParseEnterpriseRepository|CreateTaskEnterpriseRepository|RegisterEnterpriseRepository|GitHubRepositoryOriginVerifier|.*DefaultBranch.*)' -count=1
```

The targeted race suites passed. Documentation catalog validation, specification
lint, and `git diff --check` passed. Public guidance already describes the
supported host and repository behavior; no public documentation change is needed.


### Greptile follow-up

Retained old-host PR watches no longer abort current-host batches. Host-aware fallback keys keep otherwise identical PR targets separate. The persisted-result regression covers batch, poller fallback, and passive fallback, with old-host data unchanged.
