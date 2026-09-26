---
id: "07-e2e-docs-verification"
title: "Prove and document repository secrets"
status: done
wave: 5
depends_on: ["04-runtime-environment-propagation", "06-repository-binding-settings"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/repository-secrets.md"
---

# Task 07: Prove and Document Repository Secrets

## Acceptance

- Desktop E2E proves Global/Workspace filtering, repository binding persistence, conflict failure,
  deleted-ref failure, and local runtime/terminal injection without exposing plaintext in UI.
- `mobile-repository-secrets.spec.ts` proves the complete phone settings flow and no overflow.
- Container E2E proves Docker propagation and SSH approved-key forwarding plus the negative unrelated
  env case.
- Public docs accurately describe scope, inheritance, conflicts, snapshots/reset, terminal exposure,
  and SSH behavior.
- Broad backend/frontend/i18n checks pass and plan/task results record exact commands.

## Files likely touched

- `apps/web/e2e/tests/settings/repository-secrets.spec.ts`
- `apps/web/e2e/tests/settings/mobile-repository-secrets.spec.ts`
- `apps/web/e2e/tests/docker/repository-secrets.spec.ts`
- `apps/web/e2e/tests/ssh/repository-secrets.spec.ts`
- E2E API client/page helpers as needed
- `docs/public/agents-and-profiles.md`
- `docs/public/executors.md`
- `docs/public/authentication.md`
- `docs/public/security.md`
- This plan and task file result/status sections

## Inputs

- Completed backend/runtime Tasks 01–04.
- Completed frontend Tasks 05–06.
- E2E skill conventions and `apps/web/e2e/README.md` project routing.

## Dependencies

Tasks 04 and 06.

## TDD/E2E sequence

1. Add desktop and mobile browser scenarios against the real per-worker backend.
2. Add focused Docker and SSH container scenarios using their dedicated fixtures.
3. Fix only feature-owned testability gaps such as stable test IDs; do not duplicate backend policy
   in Playwright helpers.
4. Update public docs from verified behavior, then run broad checks and record results.

## Verification

```bash
make -C apps/backend test
make -C apps/backend lint
cd apps && pnpm --filter @kandev/web test -- --run
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
cd apps && pnpm run i18n:check && pnpm run i18n:ratchet
cd apps/web && pnpm e2e:run tests/settings/repository-secrets.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-repository-secrets.spec.ts
cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e --project=containers tests/docker/repository-secrets.spec.ts tests/ssh/repository-secrets.spec.ts
```

## Risks

- E2E diagnostics and assertions must never print secret plaintext.
- Container tests require Docker and belong only to the `containers` project; contributors without
  Docker still need deterministic unit coverage for the transport matrix.
- Public docs must distinguish user-global from instance-global when authentication is enabled.

## Output contract

Report each desktop/mobile/runtime scenario, exact command results, any Docker availability limit,
documentation updated, files changed, and residual risks. Mark the spec shipped and plan/tasks done
only when implementation and required verification genuinely pass.

## Result

Added desktop, mobile, Docker, and SSH repository-secret E2E coverage plus public documentation for
scope, inheritance, fail-closed conflicts, snapshots, and SSH forwarding. Successful verification:

- `make -C apps/backend test` — passed.
- `make -C apps/backend lint` — passed with 0 issues.
- Full web Vitest — 1,089 files passed; 8,309 tests passed and 4 skipped.
- Focused web tests — 6 files and 41 tests passed; typecheck and lint passed.
- `pnpm run i18n:pseudo`, `i18n:check`, and `i18n:ratchet` — passed.
- `pnpm run build:vite` — passed.
- Chromium repository-secret E2E — 3 passed; mobile-chrome — 1 passed.
- Containers Docker repository-secret E2E — 1 passed; SSH repository-secret E2E — 1 passed.
- Public-doc validation — 58 tests passed and 41 published pages validated.

Fresh desktop and mobile PR assets were generated from synthetic data, visually inspected, and
compressed. The env-gated PostgreSQL tests remain available but were skipped because this workspace
has no `KANDEV_TEST_POSTGRES_DSN`.
