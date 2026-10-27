---
id: "01-host-transport"
title: "Host-aware clients and transport"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-ENTERPRISE-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-001.2
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-001.3
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-001.4
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-001.5
system_design:
  - ../../specs/integrations/system-design/github-enterprise-hosts.md
---

# Task 01: Host-aware clients and transport

## Summary

Introduce the shared validated host value and route all direct GitHub client families through explicit web/REST/GraphQL endpoints. This is the prerequisite for safely accepting an Enterprise CLI token.

## In scope

- Own `internal/common/githubhost` normalization, endpoint derivation, and CLI token-family classification.
- Add host-aware token, App, OAuth, and manifest-conversion clients; preserve explicit public compatibility constructors.
- Enforce authenticated redirect/pagination boundaries, TLS verification, private-host connectivity, and restricted direct dialing.
- Add table-driven family tests and adversarial request-destination tests using injected TLS transports.

## Out of scope

Connection persistence, UI controls, workflow identity migrations, and executor wiring belong to later orders.

## Acceptance

1. Public, GHE.com, and GHES REST/GraphQL requests reach their exact derived endpoints; credentials never reach a foreign redirect/pagination target.
2. Invalid input and forbidden direct-dial destinations fail before credentials are transmitted; trusted private-network TLS works without a production bypass.
3. Existing public-host client behavior and API version headers remain compatible.

## Verification

Start at repository root. Read scoped AGENTS.md and use `/tdd` for changed
logic. New worktrees first run `(cd apps && pnpm install --frozen-lockfile)`.
Backend changes also run `bash scripts/lint-go-changed` with the explicit changed
Go file paths; frontend changes run ESLint on the explicit changed TS/TSX paths.
Record the expanded commands in Results. Do not mark skipped database tests as passed.

```bash
(cd apps/backend && go test ./internal/common/githubhost -count=1)
(cd apps/backend && go test ./internal/github -run 'TestEnterpriseHostTransport|TestPATClient|TestTokenClient|TestAppClient|TestGraphQL' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/common/githubhost/host.go`, `host_test.go` (new).
- `apps/backend/internal/github/token_client.go`, `pat_client.go`, `graphql.go`, and related REST helpers.
- `apps/backend/internal/github/app_client.go`, `oauth_flow.go`, `deployment_app_conversion_client.go`.
- `apps/backend/internal/github/enterprise_transport_test.go` (new) and neighboring client tests.

## Dependencies

None. Follow the plan's acceptance order and authorized implementation lanes.

## Risks

GHES GraphQL is not REST-base plus `/graphql`. Go redirect behavior is insufficient as a credential boundary. Keep trusted operator proxy routing explicit.

## Parallelism

Parallel-safe only within the explicitly owned implementation lane in plan.md; integration acceptance remains dependency-ordered.

## Inputs

- [Requirements](../../specs/integrations/requirements/github-enterprise-hosts.md), using the exact IDs above.
- [System design](../../specs/integrations/system-design/github-enterprise-hosts.md) and [host/OAuth ADR](../../decisions/2026-10-14-github-enterprise-host-boundaries.md).
- Existing source and neighboring tests named in Files likely touched; the plan's test matrix defines new regression names.

## Results

Host value and client transport implementation are complete and verified. The host/endpoint, unsafe pagination,
OAuth/manifest/App routing, redirects, TLS, direct destination and operator proxy
regressions were observed failing before implementation.

Passed from `apps/backend`:

```bash
PATH=/usr/local/go/bin:$PATH GOMAXPROCS=2 go test -trimpath -p 1 ./internal/common/githubhost ./internal/github -run 'TestHost|TestEnterpriseHostTransport|TestPATClient|TestTokenClient|TestAppClient|TestGraphQL' -count=1
```

Combined changed-Go lint passed with zero issues after all implementation lanes
stabilized. The complete GitHub package with actual PostgreSQL passed in
142.070 seconds; Enterprise race tests passed in 19.565 seconds. Host-family
fixtures verify routing and TLS behavior, not live-provider certification.


### Greptile follow-up

The shared host package and its existing tests moved to `internal/common/githubhost` to satisfy the backend/agentctl ownership boundary. All consumers and verification paths use the neutral package.
