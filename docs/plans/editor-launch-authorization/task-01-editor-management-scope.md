---
id: "01-editor-management-scope"
title: "Editor management requires org.config.manage"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AUTH-ROLES-AND-SCOPES-001
acceptance_criteria:
  - AC-AUTH-ROLES-AND-SCOPES-001.1
  - AC-AUTH-ROLES-AND-SCOPES-001.7
system_design:
  - ../../specs/auth/system-design/editor-authorization-boundary.md
---

# Task 01: Editor management requires org.config.manage

## Summary

Mount `authz.RequireOrgScope(authz.ScopeOrgConfigManage)` on the editor
create, update and delete routes. `GET /api/v1/editors` stays open.

## In scope

- `apps/backend/internal/editors/handlers/handlers.go`: in `RegisterRoutes`,
  build `manageConfig := authz.RequireOrgScope(authz.ScopeOrgConfigManage)`
  and pass it as the first handler of `POST /editors`, `PATCH /editors/:id` and
  `DELETE /editors/:id`, as `prompts/handlers/handlers.go:38` does.
- New `apps/backend/internal/editors/handlers/editor_org_scope_test.go`. Build
  a gin router that sets an identity with `authn.SetOnGin` (pattern:
  `prompts/handlers/handlers_test.go:29-50`) and calls the real
  `RegisterRoutes` over a controller backed by an in-memory editor repository.

## Out of scope

Open routes (task 02), argument handling (task 03), UI (task 04), the
settings-catalog path (already guarded), and the scope completeness test.

## Acceptance

1. As a real `member` and as a `guest`, `POST`, `PATCH` and `DELETE` return 403
   with `"scope":"org.config.manage"`, and the repository records no write.
   The same requests as a real `admin` succeed. This is the regression test
   that fails before the change.
2. `GET /api/v1/editors` returns 200 for a real `member`.
3. With a synthetic identity (authentication off), all four routes behave as
   before.

## Verification

Write the test first and confirm that the member case returns 200 before the
guard is added.

```bash
(cd apps/backend && go test -count=1 ./internal/editors/handlers/... ./internal/authz/...)
git ls-files --modified --others --exclude-standard -z -- 'apps/backend/**/*.go' | xargs -0 -r bash scripts/lint-go-changed
```

Mutation: drop `manageConfig` from each route in turn; the matching subtest
must fail.

## Likely files

- `apps/backend/internal/editors/handlers/handlers.go`
- `apps/backend/internal/editors/handlers/editor_org_scope_test.go` (new)

## Results

Editor create, update and delete routes now require `org.config.manage`; listing remains open. The route regression covers member and guest denials without repository writes, plus admin and synthetic-identity success.

Verification: `go test -count=1 ./internal/editors/handlers/... ./internal/authz/...` passed.
