---
decision: docs/decisions/2026-08-01-architecture-lint-budgets.md
related_decisions:
  - docs/decisions/2026-08-01-global-run-scheduler-ownership.md
created: 2026-08-03
status: done
---

# Implementation Plan: Architecture Lint Dependency Ownership Follow-up

## Scope and requirements source

This is an internal repository-tooling change. The accepted architecture-lint
ADR explicitly says that this boundary has no product spec; the ADRs and the
requested rule contracts are therefore the requirements source for this plan.

Add exactly three modular, dependency-free architecture rules:

- `ARCH-RUN-SCHEDULER-OWNER`: only production code in
  `apps/backend/internal/backendapp/**` may import the shared
  `internal/runs/scheduler` package.
- `ARCH-RUNS-OFFICE-IMPORT`: production code in
  `apps/backend/internal/runs/**` may not import `internal/office` or a
  subpackage, while preserving the exact current transitional findings.
- `ARCH-FRONTEND-STATE-UI-IMPORT`: production files in
  `apps/web/lib/state/**` may not import `apps/web/components/**` or
  `apps/web/app/**`, resolving aliases and relative modules.

Do not add rules for TanStack Query, typed events, a WebSocket contract
catalog, broad compatibility keywords, or backend composition/setter budgets.

## Current-main audit

Initial audit base: `bb71233826549c96b72832bc4bba405c2baa91e8` (the merged
architecture-linter commit at planning time).

- Scheduler ownership: the only production import is the permitted
  `apps/backend/internal/backendapp/main.go:96`; the test import in
  `scheduler_test.go` is excluded. The new baseline must be empty.
- Runs/Office: exactly three production findings, each identified by path and
  import, are present:
  `apps/backend/internal/runs/repository/sqlite/run_events.go` importing
  `internal/office/models`, `runs.go` importing the same package, and
  `internal/runs/service/service.go` importing the same package.
- Frontend state/UI: exactly two production findings, both importing
  `@/components/app-sidebar/app-sidebar-constants`, in
  `apps/web/lib/state/slices/ui/app-sidebar-actions.ts` and
  `apps/web/lib/state/slices/ui/ui-slice.ts`. The matching UI test import is
  excluded.

The checked-in baseline for `ARCH-RUNS-OFFICE-IMPORT` will contain only those
three `{path, import}` entries. The checked-in baseline for
`ARCH-FRONTEND-STATE-UI-IMPORT` will contain only the two matching entries.
The scheduler-owner baseline will be present and empty. The existing shared
baseline comparison remains exact and shrink-only; no baseline-wide or
count-only exception is permitted.

The final current-main re-audit at `560e35982fbbe67dbb95b6a99967f3a64df67552` found the same
three runs-to-Office findings and the same two frontend state-to-UI findings; scheduler ownership
still has zero findings.

## Design

### Shared scanner helpers

- Extend `scripts/architecture_lint/rules/go_imports.py` with the narrow
  production/generated-source helpers needed by both Go rules. Preserve the
  existing import parser, exclude `_test.go`, and ignore standard generated
  files marked with the Go generated-code header.
- Add a frontend import tokenizer/helper module under
  `scripts/architecture_lint/rules/` that skips comments, quoted/template
  text, and arbitrary strings while recognizing static imports, type-only
  imports, export-from declarations, and string-literal dynamic `import()`
  expressions. Resolve `@/components/*` and `@/app/*` from the Vite root and
  normalize relative imports from the source file before checking the target
  directory.
- Keep rule-specific conditions in the rule modules. Shared CLI, baseline,
  diagnostics, and compatibility-ledger code remains unchanged except for
  registry/test plumbing required by the new rules.

### Rule modules and diagnostics

Each rule gets its own stable module, test module, baseline, registry entry,
and actionable diagnostic:

| Rule | Scanner | Baseline | Diagnostic direction |
| --- | --- | --- | --- |
| `ARCH-RUN-SCHEDULER-OWNER` | `run_scheduler_owner.py` | `run_scheduler_owner.json` | Construct the one backend-wide scheduler in `internal/backendapp`; other packages consume runs services/interfaces. |
| `ARCH-RUNS-OFFICE-IMPORT` | `runs_office_import.py` | `runs_office_import.json` | Office adapters may depend on generic `runs`; generic `runs` must not depend on Office implementations. |
| `ARCH-FRONTEND-STATE-UI-IMPORT` | `frontend_state_ui_import.py` | `frontend_state_ui_import.json` | Components/routes may consume state; state must remain below UI/app layers and use dependency-neutral modules for shared values. |

Finding identity is exact and stable: `{path, import}` for import rules. The
frontend scanner reports the source module specifier while using the resolved
path only for classification. Diagnostics remain deterministically ordered by
the existing CLI.

### Documentation updates

- Update `docs/architecture-lint.md` with all six enforced rules and their
  intended seams.
- Amend `docs/decisions/2026-08-01-architecture-lint-budgets.md` so its rule
  inventory names the three new accepted boundaries while retaining the
  explicit out-of-scope contracts.
- Update `apps/backend/AGENTS.md` with the backend-wide scheduler ownership
  and the `runs`-over-Office dependency direction.
- Update `apps/web/AGENTS.md` with the production state-to-UI dependency
  direction.
- No public documentation, product spec, or new ADR is needed: this is an
  internal enforcement of already accepted architecture decisions.

## Tests

Tests are added before implementation and run in focused red-green-refactor
cycles.

- Go rule tests cover allowed composition-root imports, forbidden imports,
  block/aliased imports, test exclusion, generated-code exclusion, the empty
  scheduler baseline, exact grandfathered `runs -> office` entries, new debt
  rejection beside existing debt, and stale-entry cleanup.
- Frontend rule tests cover static and type-only imports, export-from,
  dynamic `import()`, `@/` aliases, relative resolution from nested state
  files, permitted local imports, comments/arbitrary strings, test/fixture/
  generated exclusions, exact two-entry baseline regression, and new-dependency
  rejection.
- Existing modular-layout, deterministic-ordering, CLI, and baseline growth
  tests continue to exercise every registered rule. Add rule IDs and baseline
  fixtures through `scripts/architecture_lint_tests/support.py` without
  weakening existing rules.

## Verification

Exact checks for the implementation package:

```bash
python3 scripts/lint-architecture.test.py
python3 scripts/lint-architecture.py --all
make lint-architecture
make -C apps/backend lint
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm run typecheck
git diff --check
```

The final report records the exact commands and results, the current base/head
SHAs, and confirms that no E2E or screenshot run is required because this
changes repository tooling and documentation rather than the product UI.

## Implementation waves

Sequential tasks are required because the small rule registry and test fixture
are shared:

- [x] [Task 01 — Go ownership rules](task-01-go-ownership-rules.md) — done
- [x] [Task 02 — Frontend state/UI rule](task-02-frontend-state-ui-rule.md) — done
- [x] [Task 03 — Documentation and final integration](task-03-docs-and-verification.md) — done

## Out of scope

- Moving sidebar constants or reorganizing state/components.
- Migrating the runs implementation out of Office.
- TanStack Query, typed-event, WebSocket catalog, broad compatibility, or
  backend composition rules.
- Changing product behavior, runtime scheduling, frontend behavior, or the
  existing compatibility ledger.

## Verification Results

- `python3 scripts/lint-architecture.test.py` — 41 tests passed.
- `python3 scripts/lint-architecture.py --all` — passed with no diagnostics.
- `python3 scripts/lint-architecture.py --all --baseline-base-ref 560e35982fbbe67dbb95b6a99967f3a64df67552 --allow-missing-base-baseline` — passed with no diagnostics.
- `make lint-architecture` — passed.
- `make -C apps/backend lint` — passed; `golangci-lint` reported 0 issues.
- `cd apps && pnpm --filter @kandev/web lint` — passed.
- `cd apps/web && pnpm run typecheck` — passed.
- `git diff --check` — passed.
