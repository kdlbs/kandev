---
id: "01-type-azure-devops-slice-setter"
title: "Type Azure DevOps slice setter"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-AZURE-DEVOPS-SLICE-COMPOSITION-001
acceptance_criteria:
  - AC-PLATFORM-AZURE-DEVOPS-SLICE-COMPOSITION-001.1
  - AC-PLATFORM-AZURE-DEVOPS-SLICE-COMPOSITION-001.2
system_design:
  - ../../specs/platform/system-design/azure-devops-slice-root-composition.md
---

# Task 01: Type Azure DevOps slice setter

## Outcome

Compose the Azure DevOps slice with the root store's recipe setter directly,
without assertions, while preserving its state behavior and unrelated root
state references.

## Scope and exclusions

Owned files:

- `apps/web/lib/state/slices/azure-devops/azure-devops-slice.ts`
- `apps/web/lib/state/slices/azure-devops/types.ts`
- `apps/web/lib/state/slices/azure-devops/azure-devops-slice.test.ts`
- The `createAzureDevOpsSlice` call in `apps/web/lib/state/store.ts`
- The exact Azure DevOps entry in
  `config/architecture-lint/frontend_root_state_cast.json`
- The DEP-01 entry in `docs/architecture-maintenance/dependency-cleanup.md`
- This requirement, system design, plan, and work order

Do not change Auth, Features, System, provider composition, other root-state
references, state shape, or rendered behavior. Do not add casts elsewhere or
introduce a generic slice abstraction.

## Acceptance

- `AC-PLATFORM-AZURE-DEVOPS-SLICE-COMPOSITION-001.1`: The slice creator accepts
  only the recipe setter capability it uses, and the root-store call type-checks
  without a type assertion or broader root-state cast.
- `AC-PLATFORM-AZURE-DEVOPS-SLICE-COMPOSITION-001.2`: Tests preserve default
  state, pull-request and work-item insert/update/reset behavior, and unrelated
  root-state references for isolated and root-store composition.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run lib/state/slices/azure-devops/azure-devops-slice.test.ts lib/state/store.test.ts lib/state/hydration/hydrator.test.ts
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run lint
make lint-architecture
python3 scripts/lint-architecture.test.py
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Results

- Focused slice, root-store, and hydration tests passed: 3 files, 47 tests.
- `cd apps/web && pnpm run typecheck` passed.
- `cd apps/web && pnpm run lint` passed.
- `make lint-architecture` passed; `python3 scripts/lint-architecture.test.py`
  passed all 99 tests using a task-specific `TMPDIR` because the shared `/tmp`
  filesystem was full.
- `python3 scripts/list-docs.py validate` and
  `python3 scripts/lint-spec-files.py --all` passed after the DEP-01 delivery
  record update.
- `git diff --check` passed.
- PR: [#4009](https://github.com/kdlbs/kandev/pull/4009). Refresh exact-head
  CI and review status before merging; the parent task owns integration review.
