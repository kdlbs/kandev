---
id: "02-connection-lifecycle"
title: "Enterprise connection and App lifecycle"
status: complete
wave: 2
depends_on: ["01-host-transport"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-ENTERPRISE-001
  - REQ-INTEGRATIONS-GITHUB-ENTERPRISE-002
  - REQ-INTEGRATIONS-GITHUB-ENTERPRISE-003
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-001.1
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-001.2
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-001.5
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-002.1
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-002.2
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-002.3
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-002.4
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.3
system_design:
  - ../../specs/integrations/system-design/github-enterprise-hosts.md
---

# Task 02: Enterprise connection and App lifecycle

## Summary

Fix the reported Enterprise CLI Save failure through the real service/handler validation path. Extend PAT/App setup and personal OAuth with host-bound state and generation fencing before enabling downstream workflows.

## In scope

- Preserve CLI discovery; replace the public-only token guard with validated host selection and host-correct `/user` verification. Strip all ambient token families.
- Carry host through PAT save, App import preparation/verification, manifest conversion, installation callbacks, refresh, and signed webhooks.
- Persist immutable flow host and OAuth policy; support explicit GHES confidential exchange and non-expiring user tokens.
- Add compatible flow/schema changes and atomic host replacement fencing, including personal binding invalidation and repository-filter reset.
- Reconcile the existing authentication design's host/PKCE scope with the new extension and preserve its credential ownership rules.

## Out of scope

The dropdown already exists and is not redesigned here. Resource/watch migrations, broader workflow routing, executor access, and new PAT/App UI fields belong to later orders.

## Acceptance

1. `TestEnterpriseCLIConnectionSaveAndRead` starts from a discovered Enterprise host/login, traverses real Save validation with a fake CLI/TLS provider, reloads the stored connection, and reads a repository from that host. Same login on another host cannot satisfy validation.
2. PAT, App import/manifest/install, personal OAuth/refresh, and webhook effects remain host-isolated, including equal IDs, replay, replacement races, and expected-generation failure.
3. Failed setup preserves the old connection; successful host replacement invalidates old flow/credential authority and never silently widens repository scope.

## Verification

Start at repository root. Read scoped AGENTS.md and use `/tdd` for changed
logic. New worktrees first run `(cd apps && pnpm install --frozen-lockfile)`.
Backend changes also run `bash scripts/lint-go-changed` with the explicit changed
Go file paths; frontend changes run ESLint on the explicit changed TS/TSX paths.
Record the expanded commands in Results. Do not mark skipped database tests as passed.

```bash
(cd apps/backend && go test ./internal/github -run 'TestEnterprise|Test.*(Connection|OAuth|GHAccount|Manifest|AppRegistration|Installation|Webhook)' -count=1)
(cd apps/backend && go test ./internal/github -run TestPostgresStoreSchemaReplay -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/github/gh_accounts.go`, `service_connections.go`, `auth_resolver.go`, `auth_principal.go`, and connection controller/request models.
- `apps/backend/internal/github/app_registration_import.go`, `app_registration_helpers.go`, `deployment_app_manifest.go`, `app_installation_service.go`, `personal_auth_service.go`, `oauth_flow.go`, `webhook_service.go`.
- `apps/backend/internal/github/store.go`, flow/registration repositories, `workspace_settings_service.go`, and associated serialization models.
- `apps/backend/internal/github/enterprise_connections_test.go`, `enterprise_app_auth_test.go` (new), existing connection/App tests and `store_postgres_test.go`.
- `docs/specs/integrations/system-design/github-authentication-01.md`, `github-authentication-02.md`, `github-authentication-03.md` (scope reconciliation only).

## Dependencies

01-host-transport. Follow the plan's acceptance order and authorized implementation lanes.

## Risks

A mock Save endpoint can conceal the exact reported defect. The regression must reach `resolveGHAccountToken` and real host-specific validation. PostgreSQL verification requires an isolated test DSN; record skips as missing evidence.

## Parallelism

Parallel-safe only within the explicitly owned implementation lane in plan.md; integration acceptance remains dependency-ordered.

## Inputs

- [Requirements](../../specs/integrations/requirements/github-enterprise-hosts.md), using the exact IDs above.
- [System design](../../specs/integrations/system-design/github-enterprise-hosts.md) and [host/OAuth ADR](../../decisions/2026-10-14-github-enterprise-host-boundaries.md).
- Existing source and neighboring tests named in Files likely touched; the plan's test matrix defines new regression names.

## Results

Implementation is complete. The final complete GitHub package, including actual
PostgreSQL checks, passed in 142.070 seconds after the last cleanup/mock changes.

The exact named-CLI Save/reload/repository-read regression passes through the real
host-specific client. PAT, App import/manifest/install, immutable OAuth policy,
non-expiring personal tokens, and host-bound secret reads have focused passing
coverage. Deterministic replacement tests prove captured old-host PAT/personal
connections cannot read a replacement host's secrets. PostgreSQL auth-flow,
setup, preparation, host-transition round trips and schema replay pass against a
disposable PostgreSQL 16 instance. No live GHES/GHE.com provider was available.

Host-changing PAT-to-CLI/App replacement retains the committed new connection
when obsolete-secret deletion fails; same-host compensation remains covered.
The mocked Enterprise App runtime uses real persisted installation and OAuth
flows with deterministic provider responses. Its endpoint regression changed
from `409 github_not_configured` to success, and mock normal/race suites passed.

Final full-package command (from `apps/backend`, with the isolated DSN supplied):

```bash
KANDEV_TEST_POSTGRES_DSN="$enterprise_test_dsn" GOMEMLIMIT=2GiB GOMAXPROCS=2 /usr/local/go/bin/go test -trimpath -p 1 ./internal/github -count=1
```

Final cleanup/replacement race regressions passed in 2.910 seconds. Final
changed-Go lint over connection cleanup, its regression tests and mock runtime
files passed with zero issues.

Final targeted commands:

```bash
(cd apps/backend && GOMAXPROCS=2 /usr/local/go/bin/go test -race -trimpath -p 1 ./internal/github -run 'TestEnterpriseCrossHostConnectionCleanupFailureKeepsCommittedAuthority|TestEnterpriseReplacementNeverRebindsOld' -count=1)
PATH=/usr/local/go/bin:$PATH GOMAXPROCS=2 bash scripts/lint-go-changed apps/backend/internal/github/service_connections.go apps/backend/internal/github/service_app_auth_connection_store.go apps/backend/internal/github/enterprise_secret_binding_test.go apps/backend/internal/github/mock_controller.go apps/backend/internal/github/mock_app_runtime.go apps/backend/internal/github/mock_app_runtime_test.go
```

PR remediation: reconnect now compares the retained settings host inside the
connection transaction. Switching from a disconnected enterprise host to
GitHub.com resets host-bound settings; reconnecting the same host retains scope.
Regression coverage includes settings/preset saves after reload, SQLite rollback
on a settings-write failure, and a real PostgreSQL reconnect round trip.

Post-remediation validation: the complete GitHub package passed with the isolated
PostgreSQL 16 DSN in 148.846 seconds using the full-package command above.
`go test -race -trimpath -p 1 ./internal/github -run
'TestEnterprise(SearchResponsesCarryHost|Reconnect)' -count=1` passed in 3.775
seconds. Remote CI and automated review remain pending until the fixup push.
