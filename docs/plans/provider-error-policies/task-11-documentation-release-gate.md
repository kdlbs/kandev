---
id: "11-documentation-release-gate"
title: "Documentation release gate"
status: done
wave: 10
depends_on: ["10-policy-e2e"]
plan: "plan.md"
spec: "../../specs/platform/requirements/provider-error-recovery.md"
---

# Task 11: Documentation release gate

- **Acceptance:** Document both error classes, all policy controls, safe
  defaults, catalogue growth, Kanban/Office parity, and deferred model-based
  classification; then pass the package's backend, web, i18n, documentation,
  and diff checks.
- **Files likely touched:**
  `docs/public/{agents-and-profiles,tasks-and-workflows}.md`, feature-status or
  configuration docs if rollout behavior changes, plan/task result sections,
  and no production code unless a verification failure returns work to its
  owning task.
- **Dependencies:** Task 10.
- **Parallelism:** sequential final gate.
- **Inputs:** Provider Error Recovery Out of scope and scenarios; Dynamic Agent
  Routing public behavior; Task 10 evidence; `/docs-maintainer`.
- **Output contract:** Report public pages, copy validation, full exact command
  results/counts, E2E cleanup evidence, remaining risks, and synchronized
  task/plan status.
- **Verification:** `make -C apps/backend lint && cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web run lint && pnpm --filter @kandev/web run typecheck && cd web && pnpm run i18n:check && pnpm run i18n:ratchet && node --test scripts/validate-public-docs.test.mjs && cd ../.. && git diff --check`
- **Risks:** Documentation must not imply unknown strings are automatically
  classified or that model-based classification and telemetry routing exist.

## Results

Completed. Updated the public agent/profile and feature-status pages with both
error classes, policy controls, safe defaults, catalogue growth, Kanban/Office
scope, and deferred model-based classification. Telemetry routing remains
outside this package.

Verification:

- `make -C apps/backend lint` — `golangci-lint`: 0 issues.
- `pnpm run lint` — passed with no warnings.
- `pnpm run typecheck` — passed.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet` — passed.
- `node --test scripts/validate-public-docs.test.mjs` — 61 passed.
- `node scripts/validate-public-docs.mjs` — 41 published pages validated.
- `git diff --check` — passed.
