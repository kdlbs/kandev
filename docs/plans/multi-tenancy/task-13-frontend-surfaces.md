---
id: "13-frontend-surfaces"
title: "Frontend org and operator surfaces"
status: todo
wave: 5
depends_on: ["12-org-lifecycle"]
plan: "plan.md"
spec: "../../specs/multi-tenancy/spec.md"
---

# Task 13: Frontend Org and Operator Surfaces

## Acceptance

- The boot payload carries `org: {id, name, slug}` for authenticated callers
  and nothing for anonymous ones; the store hydrates it and a top-level test
  proves the value reaches the consuming component, not just the hook.
- Settings > System gains an operator-only Organizations page: list, create,
  suspend/resume, per-org OS user, and delete behind a type-to-confirm on the
  slug. The confirm token is compared with `===` and therefore NOT translated
  (`// i18n-exempt`).
- Settings > System gains an operator-only Templates section for the eight
  config kinds.
- Settings > Organization lets an org admin rename the org and manage members
  and invites.
- Existing executor / profile / environment / agent / editor / prompt /
  notification-provider settings render `source: "instance"` rows read-only
  with a "Use in this organization" action that creates the shadowing org row
  and re-renders without a duplicate.
- A suspended org lands on a dedicated organization-unavailable route, never on
  `/login`.
- With the flag off, no org UI, no operator pages, and no `org` key are present.
- Every string goes through `t()` / `<Trans>` in all five locales; no em dashes;
  `pnpm run i18n:check` and `pnpm run i18n:ratchet` pass.
- Mobile parity: every new page and dialog uses native mobile patterns with no
  hover-only or desktop-only required action.

## Verification

- From `apps/web`: `pnpm test`, `pnpm run typecheck`, `pnpm run lint`,
  `pnpm run i18n:check`, `pnpm run i18n:ratchet`
- `pnpm run i18n:zh-hant` for the Traditional Chinese pair

## Files Likely Touched

- `apps/web/src/boot-payload.ts`, `settings-routes.tsx`
- `apps/web/lib/api/domains/org-api.ts`, `apps/web/lib/types/org.ts`
- `apps/web/hooks/domains/org/`
- `apps/web/components/settings/organizations/`, `.../templates/`
- `apps/web/src/locales/*/`

## Inputs

- Spec: API surface, Permissions, State machine.
- Patterns: ADR 0021 Go-served SPA boot state; the `/mobile-parity` skill; the
  i18n rules in the root `AGENTS.md` (never translate a `===` token, never call
  `t()` at module scope, use `count` for plurals).

## Output Contract

Report the top-level prop-chain test for the boot `org` value, the i18n gate
results, the mobile parity evidence, and set this task plus its plan checkbox
to done.
