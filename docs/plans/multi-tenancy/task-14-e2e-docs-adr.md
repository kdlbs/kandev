---
id: "14-e2e-docs-adr"
title: "E2E, docs, and ADR"
status: todo
wave: 5
depends_on: ["13-frontend-surfaces"]
plan: "plan.md"
spec: "../../specs/multi-tenancy/spec.md"
---

# Task 14: E2E, Docs, and ADR

## Acceptance

- A Playwright spec covers the cross-org boundary end to end: two orgs, two
  users, and an attempt from org A to open org B's workspace, task, session,
  and port preview by URL, each landing on a not-found surface.
- A Playwright spec covers the operator surface: create org, invite its first
  admin, suspend (the member sees the organization-unavailable page), resume,
  and delete with the slug confirmation.
- A container-project spec (`KANDEV_E2E_CONTAINERS=1`) asserts an org-labelled
  container is not adopted by another org's recovery.
- `cmd/kandev/e2e_reset.go` deletes org rows and roots so a worker-scoped
  backend starts clean, and no poller recreates rows mid-reset.
- An ADR records the durable decisions: the tenant is derived from the identity
  and never from the request, one org per user, identity-free contexts are
  denials rather than grants, instance templates carry no credentials, and
  `local_pc` fails closed under multi-org.
- `docs/public/**` documents enabling organizations, the operator versus org
  admin split, the per-org filesystem and agent-credential model, and the
  standalone-executor limitation. Run `/docs-maintainer`.
- Root `AGENTS.md`/`CLAUDE.md` and `apps/backend/AGENTS.md` gain the tenancy
  registry rule and the inverted identity-free-context rule, since both change
  conventions every future change must follow.
- `docs/specs/auth/spec.md` is amended: its "no workspace sharing" limit stands,
  and the shared-agent-credential limit is narrowed to within an org.

## Verification

- From `apps/web`: `pnpm run build:e2e` then `pnpm e2e:run` for the new specs
  (do not pass flags through `pnpm e2e:raw --`; they are silently dropped)
- `KANDEV_E2E_CONTAINERS=1` container project
- `make -C apps/backend test lint`

## Files Likely Touched

- `apps/web/e2e/` new specs and fixtures
- `apps/backend/cmd/kandev/e2e_reset.go`
- `docs/decisions/00XX-organization-tenancy.md`, `docs/decisions/INDEX.md`
- `docs/public/**`
- `AGENTS.md`, `CLAUDE.md`, `apps/backend/AGENTS.md`
- `docs/specs/auth/spec.md`, `docs/specs/INDEX.md`

## Inputs

- Spec: Scenarios (the conformance surface for the E2E specs).
- Patterns: `/docs-maintainer`, `/record decision`, `apps/web/e2e/README.md`,
  and the E2E reset invariant in `apps/backend/AGENTS.md`.

## Output Contract

Report the spec-scenario to E2E-test mapping with any scenario left uncovered
named explicitly, the ADR number, the docs pages touched, and set this task plus
its plan checkbox to done.
