---
id: "02-versioned-policy-document"
title: "Versioned policy document"
status: pending
wave: 2
depends_on: ["01-shared-error-catalogue"]
plan: "plan.md"
spec: "../../specs/platform/provider-error-recovery.md"
---

# Task 02: Versioned policy document

- **Acceptance:** Replace generic dynamic candidate rule maps with typed,
  versioned transient and hard policies; validate retry/wait bounds and
  exhausted outcomes; normalize legacy maps; and return field-addressable
  errors plus the canonical document from profile CRUD.
- **Files likely touched:**
  `apps/backend/internal/agent/settings/{dto,models,controller,store}/**`,
  `apps/backend/internal/agent/runtime/dynamic/types.go`, profile config
  resolution, API handlers, SQLite migrations/tests, and
  `apps/web/lib/{types,api/domains}/**`.
- **Dependencies:** Task 01.
- **Parallelism:** sequential because it fixes the API consumed by runtime and
  UI work.
- **Inputs:** Provider Error Recovery Per-class policy, Data model, and API;
  current `rules_json`, DTO normalization, and optimistic profile versioning.
- **Output contract:** Report the canonical JSON shape, numeric/duration limits,
  legacy mapping including conflicting per-code rules, migration strategy,
  API errors, files changed, exact commands/results, risks, and synchronized
  task/plan status.
- **Verification:** `cd apps && pnpm install --frozen-lockfile && cd backend && go test -tags fts5 ./internal/agent/settings/... ./internal/task/repository/sqlite/... && cd ../web && pnpm test -- --run lib/api/domains/agent-profile-normalize.test.ts`
- **Risks:** Existing profiles must preserve candidate order, enabled state,
  and immediate fallback behavior. Invalid legacy data must fail closed instead
  of receiving permissive defaults.

## Results

Pending.
