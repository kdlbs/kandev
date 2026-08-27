---
id: "06-backend-sub-setting-field"
title: "Backend sub-setting field"
status: done
wave: 5
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/agent-todo-list-panel.md"
---

# Task 06: Backend sub-setting field

Add `ShowTodoListPanelOnlyWhenNotEmpty` (wire
`show_todo_list_panel_only_when_not_empty`) to the per-user settings round
trip on the backend, mirroring `ShowTodoListPanel` at every call site. The
field is merged independently of `ShowTodoListPanel`, so a PATCH can persist
it while the master preference is off.

- **Acceptance:**
  1. `GET /api/v1/user/settings` returns
     `show_todo_list_panel_only_when_not_empty: false` for a fresh user;
     `PATCH /api/v1/user/settings` with
     `{"show_todo_list_panel_only_when_not_empty": true}` persists and returns
     `true` on the next `GET`, with `show_todo_list_panel` untouched.
  2. The boot/hydration payload includes
     `showTodoListPanelOnlyWhenNotEmpty` beside `showTodoListPanel`.
  3. `go vet`/`go build` pass for the touched packages; new tests go red
     before implementation and green after.
- **Verification:** `cd apps/backend && go test ./internal/user/... ./internal/backendapp/...`
  (note: `internal/backendapp` has 2 pre-existing failures documented in
  Task 01's Results — GPG commit-signing environment issue unrelated to this
  change; re-confirm they are the same two before attributing anything here).
- **Files likely touched:**
  - `apps/backend/internal/user/models/models.go`
  - `apps/backend/internal/user/dto/dto.go`
  - `apps/backend/internal/user/service/service.go`
  - `apps/backend/internal/user/controller/controller.go`
  - `apps/backend/internal/user/store/sqlite.go`
  - `apps/backend/internal/user/store/sqlite_test.go`
  - `apps/backend/internal/user/service/service_test.go`
  - `apps/backend/internal/backendapp/boot_state_routes.go`
- **Dependencies:** None (backend-only; the frontend consumes the wire field
  in Task 07).
- **Parallelism:** `sequential` (Iteration 2 runs sequentially in the primary
  conversation).
- **Inputs:** Spec Data model / API surface sections
  (`docs/specs/ui/requirements/agent-todo-list-panel.md`); plan's "Iteration 2 additions"
  Backend section (`plan.md`); Task 01's diff as the exact pattern (add the
  new field beside `ShowTodoListPanel` at every one of its call sites).

## Results

Red first: added `TestApplyBasicSettingsTodoListPanelOnlyWhenNotEmpty`
(service_test.go, 3 subtests: omission preserves / explicit replaces /
explicit false disables) and `TestScanUserSettingsTodoListPanelOnlyWhenNotEmptyDefault`
+ `TestTodoListPanelOnlyWhenNotEmptyRoundTripThroughMarshalAndScan`
(sqlite_test.go). `go test ./internal/user/service/ ./internal/user/store/`
failed to compile with `unknown field ShowTodoListPanelOnlyWhenNotEmpty` at
every reference — confirmed red.

Implemented `ShowTodoListPanelOnlyWhenNotEmpty` /
`show_todo_list_panel_only_when_not_empty` across `models.go`, `dto.go`
(DTO, update-request, `FromUserSettings`), `service.go`
(`UpdateUserSettingsRequest`, `applyTaskActionPreferences`, response map),
`controller.go` (DTO→service conversion), `store/sqlite.go`
(`marshalUserSettingsPayload`, `defaultUserSettings`, scan payload+apply),
and `boot_state_routes.go` (boot payload
`showTodoListPanelOnlyWhenNotEmpty`). Ran `gofmt -w` on all touched files.

Commands:
- `cd apps/backend && go test ./internal/user/...` → all packages `ok`
  (controller, dto, handlers, service, store).
- `cd apps/backend && go test ./internal/backendapp/...` → `ok`; the two
  pre-existing failures recorded in Task 01 (`TestDetectBranchRemote_*`,
  GPG signing env issue) did **not** reproduce in this environment, and no
  new failures appeared.
- `cd apps/backend && golangci-lint run ./internal/user/... ./internal/backendapp/`
  → clean.

Lint note: adding the 36-char key to the boot-state map literal makes gofmt
re-align the whole key/value block, which trips a column-position-sensitive
goconst quirk that then reports pre-existing repeated map keys (`workflowId`,
`steps`, `tasks` in `boot_state.go`) — verified reproducible only with the
re-aligned columns and absent with HEAD's columns. Resolved by keeping the
original block's alignment and placing the new key in its own
comment-led group (gofmt-stable, lint-clean); extracted constants were
rejected because the quirk merely moves to the next repeated key
(whack-a-mole). Full `make lint` passes.

Blockers/risks: none.
