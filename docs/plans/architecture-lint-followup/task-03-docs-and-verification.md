---
id: "03-docs-and-verification"
title: "Document and verify architecture rules"
status: done
wave: 3
depends_on:
  - "01-go-ownership-rules"
  - "02-frontend-state-ui-rule"
plan: "plan.md"
decision: "../../decisions/2026-08-01-architecture-lint-budgets.md"
---

# Task 03: Document and verify architecture rules

## Acceptance

- The architecture-lint rule inventory and accepted ADR describe all six
  enforced boundaries and retain the explicitly excluded contracts.
- Scoped backend and frontend guidance states scheduler ownership and the
  intended dependency directions without introducing implementation or product
  changes.
- The checked-in tree passes the architecture-lint regression suite, normal
  architecture lint, relevant backend/frontend checks, and diff hygiene.

## Verification

```bash
python3 scripts/lint-architecture.test.py
python3 scripts/lint-architecture.py --all
make lint-architecture
make -C apps/backend lint
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm run typecheck
git diff --check
```

## Files likely touched

- `docs/architecture-lint.md`
- `docs/decisions/2026-08-01-architecture-lint-budgets.md`
- `apps/backend/AGENTS.md`
- `apps/web/AGENTS.md`
- `docs/plans/architecture-lint-followup/plan.md`
- this task file's `## Results`

## Inputs and dependencies

- Accepted ADRs `2026-08-01-architecture-lint-budgets` and
  `2026-08-01-global-run-scheduler-ownership`.
- Completed scanner/baseline work from Tasks 01 and 02.

## Parallelism

Sequential final integration and verification.

## Results

- `python3 scripts/lint-architecture.test.py` — 41 tests passed.
- `python3 scripts/lint-architecture.py --all` — passed with no diagnostics.
- `python3 scripts/lint-architecture.py --all --baseline-base-ref 560e35982fbbe67dbb95b6a99967f3a64df67552 --allow-missing-base-baseline` — passed with no diagnostics.
- `make lint-architecture` — passed.
- `make -C apps/backend lint` — passed; `golangci-lint` reported 0 issues.
- `cd apps && pnpm --filter @kandev/web lint` — passed.
- `cd apps/web && pnpm run typecheck` — passed.
- `git diff --check` — passed.
- Final re-audit at `560e35982fbbe67dbb95b6a99967f3a64df67552`: scheduler ownership had zero findings; runs-to-Office had the exact three baseline entries; frontend state-to-UI had the exact two baseline entries.
