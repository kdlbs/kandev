---
id: "01-backend-settings-field"
title: "Backend settings field"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/agent-todo-list-panel.md"
---

# Task 01: Backend settings field

Add `ShowTodoListPanel` to the per-user settings round trip on the backend,
mirroring `ShowTranscriptAutoScrollControl` at every call site.

- **Acceptance:**
  1. `GET /api/v1/user/settings` returns `show_todo_list_panel: false` for a
     fresh user; `PATCH /api/v1/user/settings` with
     `{"show_todo_list_panel": true}` persists and returns `true` on the next
     `GET`.
  2. `go vet`/`go build` pass for the touched packages.
- **Verification:** `cd apps/backend && go test ./internal/user/... ./internal/backendapp/...`
- **Files likely touched:**
  - `apps/backend/internal/user/models/models.go`
  - `apps/backend/internal/user/dto/dto.go`
  - `apps/backend/internal/user/service/service.go`
  - `apps/backend/internal/user/controller/controller.go`
  - `apps/backend/internal/user/store/sqlite.go`
  - `apps/backend/internal/user/store/sqlite_test.go`
  - `apps/backend/internal/user/service/service_test.go`
  - `apps/backend/internal/backendapp/boot_state_routes.go`
- **Dependencies:** None.
- **Parallelism:** `parallel-safe` (disjoint from Task 02's frontend-only files).
- **Inputs:** Spec Data model / API surface sections
  (`docs/specs/ui/requirements/agent-todo-list-panel.md`); plan's Backend section
  (`plan.md`); existing `ShowTranscriptAutoScrollControl` call sites as the
  exact pattern.

## Results

Added `ShowTodoListPanel`/`show_todo_list_panel` across `models.go`,
`dto.go` (DTO, update-request, `FromUserSettings`), `service.go`
(`UpdateUserSettingsRequest`, `applyTaskActionPreferences`,
`publishUserSettingsEvent`), `controller.go` (DTO→service conversion),
`sqlite.go` (`marshalUserSettingsPayload`, `defaultUserSettings`,
`scanUserSettings` payload+apply), and `boot_state_routes.go`, mirroring
`ShowTranscriptAutoScrollControl` at every site. Added
`TestApplyBasicSettingsTodoListPanel` (service_test.go),
`TestScanUserSettingsTodoListPanelDefault` and
`TestTodoListPanelSettingRoundTripThroughMarshalAndScan` (sqlite_test.go);
confirmed red (compile failure) before implementing, green after.

Command: `cd apps/backend && go test ./internal/user/... ./internal/backendapp/...`
Result: `internal/user/{controller,dto,service,store}` all `ok`.
`internal/backendapp` has 2 pre-existing failures
(`TestDetectBranchRemote_ReturnsConfiguredUpstream`,
`TestDetectBranchRemote_NoUpstreamFallsBackToOrigin`) caused by a local GPG
commit-signing environment issue in `worktree_test.go`, unrelated to this
change (no file this task touched is referenced by those tests) — not fixed
here, out of scope.
