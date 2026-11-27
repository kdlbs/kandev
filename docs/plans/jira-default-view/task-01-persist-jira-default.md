---
id: 01-persist-jira-default
title: Persist the Jira default preference
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-INTEGRATIONS-JIRA-DEFAULT-VIEW-001
acceptance_criteria:
  - AC-INTEGRATIONS-JIRA-DEFAULT-VIEW-001.1
  - AC-INTEGRATIONS-JIRA-DEFAULT-VIEW-001.3
system_design:
  - ../../specs/integrations/system-design/jira-default-view.md
---

# Task 01: Persist the Jira default preference

## Summary

Expose one portable, per-user default view ID through the existing user-settings contract. This task provides storage and transport only; it does not select a view in the browser.

## Scope and likely files

- `apps/backend/internal/user/models/models.go`
- `apps/backend/internal/user/dto/dto.go` and `dto/dto_test.go`
- `apps/backend/internal/user/controller/controller.go`
- `apps/backend/internal/user/service/service.go` and `service/service_test.go`
- `apps/backend/internal/user/store/sqlite.go` and `sqlite_test.go`
- `apps/backend/internal/settingscatalog/defaults.go` and relevant catalog tests
- `apps/web/lib/types/http-user-settings.ts`
- Regenerated `apps/web/lib/settings-discovery/contract.generated.json`

## Exclusions

- No Jira connection-setting change, dedicated database column, or URL contract.

## Implementation acceptance

1. GET returns the user's saved ID, and PATCH distinguishes omitted, nonempty, and empty-string values without changing other settings.
2. The preference survives a SQLite round-trip and appears in settings discovery as a caller-owned string.
3. Existing user records return the empty default without a migration.

## TDD and verification

First add tests for omitted/set/clear requests and SQLite round-trip; confirm the new assertions fail before implementation. Then implement the contract and run:

```bash
cd apps/backend && go test ./internal/user/dto ./internal/user/service ./internal/user/store ./internal/settingscatalog
cd apps/backend && go run ./cmd/settings-catalog
node --test scripts/settings-contract-ci.test.mjs
```

Run the last command from the repository root.

Red: the focused backend test command failed to compile because the expected `JiraDefaultViewID` model and DTO fields were absent; the catalog assertion also reported that `user_settings.jira_default_view_id` was missing.

Green:

```text
cd apps/backend && go test ./internal/user/dto ./internal/user/service ./internal/user/store ./internal/settingscatalog ./internal/user/controller
PASS
cd apps/backend && go run ./cmd/settings-catalog
PASS
node --test scripts/settings-contract-ci.test.mjs
3 passed, 0 failed
```

## Dependencies and risks

No prior work order. Existing JSON payload persistence must include the new field on both save and load; a response-only DTO change would silently lose it.

## Result

The scalar preference now flows through the user model, GET/PATCH DTOs, controller and service, SQLite JSON payload, settings catalog, generated web contract, and frontend HTTP settings types. Omitted PATCH fields preserve the prior value, nonempty IDs are trimmed, and an empty ID clears the preference. Existing payloads without the field decode to the empty default without a schema migration.
