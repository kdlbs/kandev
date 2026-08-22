---
id: "09-e2e-docs-adr"
title: "E2E, docs, and ADR"
status: todo
wave: 4
depends_on: ["08-frontend-surfaces"]
plan: "plan.md"
spec: "../../specs/workspaces/membership.md"
---

# Task 09: E2E, Docs, and ADR

## Acceptance

- A Playwright spec covers the #2824 golden path with **no invitation**: an org
  whose default visibility is `org`, Ana creates a workspace and a task, Bruno
  logs in and sees it on his board, opens the transcript and the preview,
  assigns the task to himself, and prompts — attributed to Bruno.
- A Playwright spec covers the private path: default `private`, Bruno gets the
  not-found surface, Ana adds him as `collaborator`, Bruno gains access.
- A Playwright spec covers narrowing: Bruno as `viewer` on an org-visible
  workspace sees the board but has no prompt box and no terminal, and the
  direct API call returns 403.
- A Playwright spec covers revocation: Bruno loses access mid-view (once by
  removal, once by visibility flipped to `private`) and is routed out; a
  subsequent direct URL visit returns the not-found surface.
- A Playwright spec covers the guest: a `guest` sees no org-visible workspace
  and only the one they hold a row on.
- Seed tasks without an agent and locate by task id, so kanban column
  assertions do not race the agent auto-advance.
- `cmd/kandev/e2e_reset.go` deletes membership rows and resets visibility so a
  worker-scoped backend starts clean.
- An ADR records the durable decisions: permissions are named scopes in one
  registry with fixed roles rather than an admin bit; reach and permission are
  separate questions; visibility is the primary sharing mechanism and
  membership is the exception and narrowing path, chosen because a Kandev
  workspace is coarse; an explicit membership row outranks the org default in
  both directions; scope resolution fails closed and never falls back to the org
  default role; org admin remains a management role with no extra reach;
  attribution never falls back to the workspace owner; concurrency is serialized
  through the existing message queue rather than a human session lock; and the
  agent's in-session MCP identity stays the task owner rather than the last
  prompting member.
- `docs/public/**` documents team collaboration: org roles and what each can do,
  workspace visibility and the org default, adding members and narrowing them,
  takeover, and what stays owner-only. Run `/docs-maintainer`.
- `docs/specs/auth/spec.md` is amended: the "No workspace sharing/membership —
  one owner per workspace" v1 limit is replaced by pointers to
  `roles-and-scopes.md` and `workspaces/membership.md`, and the roles bullet is
  updated from two roles to four.
- Issue #2824 is answered with what shipped and what stayed out of scope
  (custom roles, human workflow approvers, group objects, PAT scopes),
  including the correction that the existing assignee and participant fields
  name agent profiles rather than people.

## Verification

- From `apps/web`: `pnpm run build:e2e` then `pnpm e2e:run` for the new specs.
  Do not pass flags through `pnpm e2e:raw --`; they are silently dropped and the
  whole suite runs instead.
- `make -C apps/backend test lint`

## Files Likely Touched

- `apps/web/e2e/` new specs and fixtures
- `apps/backend/cmd/kandev/e2e_reset.go`
- `docs/decisions/00XX-team-access-roles-and-visibility.md`, `docs/decisions/INDEX.md`
- `docs/public/**`
- `docs/specs/auth/spec.md`, `docs/specs/INDEX.md`

## Inputs

- Spec: Scenarios in both `roles-and-scopes.md` and `workspaces/membership.md`
  — together they are the conformance surface.
- Patterns: `apps/web/e2e/README.md`; the E2E reset invariant and the
  seed-without-an-agent rule for kanban column assertions.

## Output Contract

Report the scenario-to-E2E mapping across both specs with any uncovered
scenario named explicitly, the ADR number, the docs pages touched, the issue
reply, and set this task plus its plan checkbox to done.
