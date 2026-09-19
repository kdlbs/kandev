---
id: "01-exact-session-status"
title: "Exact Kubernetes session status"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/kubernetes-executor/spec.md"
---

# Task 01: Exact Kubernetes session status

- **Acceptance:** the existing Kubernetes sessions HTTP and WS reads accept a
  task ID plus optional session ID and return only matching authorized rows.
- **Acceptance:** filtering happens before task authorization and Kubernetes
  Pod GETs for unrelated rows; session-only filters are rejected and all
  existing unfiltered behavior remains compatible.
- **Acceptance:** exact UID, full ownership-label, recorded inventory, and
  task/session correlation checks remain unchanged.
- **Verification:**
  `cd apps/backend && go test ./internal/kubernetes -run 'Test.*Sessions.*(Filter|TaskSession)' -count=1 && go test ./internal/kubernetes -count=1 && git diff --check`
- **Files likely touched:**
  `apps/backend/internal/kubernetes/handlers.go`,
  `apps/backend/internal/kubernetes/sessions.go`,
  `apps/backend/internal/kubernetes/handlers_test.go`, and
  `apps/backend/internal/kubernetes/sessions_test.go`.
- **Dependencies:** None.
- **Parallelism:** sequential.
- **Inputs:** spec API/Permissions/scenarios; plan Backend section; existing
  `Handler.listSessions`, `sessionRow`, and fake client reactor patterns.
- **Output contract:** report RED/GREEN evidence, files changed, exact commands,
  trust-boundary preservation, blockers/risks, and synchronize task/plan status.

## Results

- RED: `cd apps/backend && go test -v ./internal/kubernetes -run 'Test(HTTP|WebSocket)ListSessions.*(Filter|SessionFilter)' -count=1` failed because both HTTP and WebSocket ignored the filters and the session-only HTTP request reached executor lookup.
- GREEN: the same focused command passed all three tests. `go test ./internal/kubernetes -run 'Test.*Sessions.*(Filter|TaskSession)' -count=1` and `go test ./internal/kubernetes -count=1` passed.
- Added `SessionFilter`, HTTP query and WebSocket payload parsing, pre-authorization/pre-Pod filtering, and BAD_REQUEST mapping for a session filter without a task. A well-formed task/session mismatch returns an empty list with no access-check or Kubernetes client action.
