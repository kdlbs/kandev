---
id: "03-dynamic-profile-management"
title: "Dynamic profile management"
status: completed
wave: 3
depends_on: ["02-virtual-profile-foundation"]
plan: "plan.md"
spec: "../../specs/agents/requirements/dynamic-agent-routing.md"
---

# Task 03: Dynamic profile management

- **Acceptance:** Add transactional dynamic CRUD, validation, optimistic
  versions, and typed dependency conflicts, confirmed disable/delete retains
  stale logical and candidate references under existing profile-in-use policy.
  Reject dynamic, rich Office, non-launchable, and `AutoFallback=true`
  candidates while allowing explicit `FallbackModel` policy.
- **Files likely touched:** `apps/backend/internal/agent/settings/controller/profile_crud*.go`,
  `apps/backend/internal/agent/settings/handlers/*profile*.go`,
  `apps/backend/internal/agent/settings/store/*`,
  `apps/backend/internal/agent/settings/dto/dto.go`.
- **Dependencies:** Task 02.
- **Parallelism:** sequential.
- **Inputs:** Spec Profile configuration, Transparent profile execution, and
  Dynamic profile configuration, Tasks 01 and 02, existing profile dependency
  and optimistic-update patterns.
- **Output contract:** Report validation rules, dependency behavior, files
  changed, exact commands and results, blockers, risks, and synchronized task
  and plan status.
- **Verification:** `cd apps/backend && go test -tags fts5 ./internal/agent/settings/controller/... ./internal/agent/settings/handlers/... ./internal/agent/settings/store/...`
- **Risks:** Force confirmation must not bypass dependency lookup failures or silently rewrite bindings.

## Results

Completed. Added transactional dynamic CRUD, candidate validation, optimistic
versions, typed dependency conflicts, and stale-reference preservation. Dynamic,
rich Office, non-launchable, and `AutoFallback` candidates are rejected while
explicit fallback-model policy remains valid.

Verification:

- `go test -tags fts5 ./internal/agent/settings/controller/... ./internal/agent/settings/handlers/... ./internal/agent/settings/store/...`

The command passed. Dedicated API/E2E dependency coverage remains pending.
