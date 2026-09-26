---
id: "02-versioned-policy-document"
title: "Versioned policy document"
status: done
wave: 2
depends_on: ["01-shared-error-catalogue"]
plan: "plan.md"
spec: "../../specs/platform/requirements/provider-error-recovery.md"
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

- Canonical policy JSON is `{"version":1,"transient":{...},"hard":{...}}`.
  Each class contains `retry`, `wait_for_reset`, and `on_exhausted` fields.
- Backend limits are one through ten retries, one second through one hour for
  the initial interval, and one second through seven days for reset waits.
  Disabled sections persist zero numeric values. Outcomes are `skip` and
  `stop`.
- Legacy `try_next`, `stop`, and `retry_same` rules normalize to both classes;
  `retry_same` becomes one five-second retry followed by stop. Per-code rules
  override the mapped class, while conflicting rules in one class are rejected
  with a candidate and policy field in the error message.
- The existing `dynamic_agent_routes.rules_json` column is reused for the
  canonical document. Reads accept legacy maps and return only normalized
  policies. Candidate order and enabled state remain unchanged.
- Added backend DTO/controller normalization, frontend camelCase/snake_case
  normalization, and the shared save-helper payload path.
- Checks passed:
  `cd apps/backend && go test -tags fts5 ./internal/agent/settings/... ./internal/task/repository/sqlite/...`
  (954 tests); `cd apps/web && pnpm test -- --run
  lib/api/domains/agent-profile-normalize.test.ts` (18 tests); and
  `cd apps/web && pnpm run typecheck`.
