# Coordinator Task Authority Plan

## Objective

Deliver an explicit, revocable operator-granted capability system that allows a
"coordinator" task to orchestrate the board — stop/interrupt unrelated tasks,
attach workspace sources, and read documents/relations — without requiring a
parent/child topology. Default-off, flag-gated, audited, and fail-closed.

## Work Orders

| #   | Area          | Description                                                                   | Done |
| --- | ------------- | ----------------------------------------------------------------------------- | ---- |
| 01  | Persistence   | SQLite schema, interfaces, repository, dialect parity                         | ✓    |
| 02  | Authority     | `internal/coordinator/` central authority, capability check, flag gate, audit | ✓    |
| 03  | Call sites    | `stop_task.go`, `handlers.go`, `task_target_access.go`, `handoff_service.go`  | ✓    |
| 04  | Agent surface | MCP server descriptions, sysprompt injection                                  | ✓    |
| 05  | Operator API  | Gin handlers: grant CRUD, audit query                                         | ✓    |
| 06  | Operator UI   | Settings tab: grants table, grant dialog, revoke, audit viewer                | ✓    |
| 07  | Docs          | Public operator docs                                                          | ✓    |
| 08  | Specs & Plan  | Requirements, system design, ADR, plan file                                   | ✓    |
| 09  | Runtime QA    | Task-owned exact-head auth-enabled: grant/revoke/denied-after-revoke/audit     | ✓    |
| 10  | CI fixup      | Frontend test fix, thread resolution, credential audit, push                   | ✓    |

## Dependencies

- Runtime flag `features.coordinatorTaskAuthority` must be OFF in all shipped profiles.
- Operator API and UI are independent of each other.
- Docs depend on having the full API surface to document.

## Validation Record

| Item | Evidence |
|------|----------|
| **PR** | https://github.com/kdlbs/kandev/pull/3048 (Draft, no merge) |
| **Head** | `68a178c25c105af24062c96077db0ab168309f65` pushed to `yattdev:feature/grant-coordinator-ma-nnw` |
| **CI** | All checks green (33 successes, 13 skipped, 0 failures). E2E shards 2-14 still in_progress (unrelated). |
| **Threads** | 0 unresolved |
| **Mergeable** | MERGEABLE |
| **QA image** | `kandev-qa-auth:77fb39625` (based on exact build of commits through 77fb39625) |
| **QA container** | `kandev-qa-auth-77fb39625` on `http://192.168.50.131:8084`, restart=unless-stopped, auth-enabled |
| **Auth accounts** | Admin: `admin@test.local`, Member: `member@test.local` (secrets set via `/api/v1/auth/setup` and `/api/v1/users`) |
| **Feature flag** | `KANDEV_FEATURES_COORDINATOR_TASK_AUTHORITY=true` |
| **Grant via API** | ✅ Admin POST `workspaces/:id/coordinator-grants` → 201 |
| **Duplicate guard** | ✅ Same scope → 409 Conflict |
| **Member denial** | ✅ Member user POST/DELETE → 403 Forbidden |
| **Revoke** | ✅ Admin DELETE → 200 `{"revoked":true}` |
| **Re-create after revoke** | ✅ 201 (partial unique index `WHERE revoked_at IS NULL`) |
| **Workflow scope** | ✅ Workflow-scoped grant created with validation |
| **Audit DB schema** | ✅ `task_coordinator_audit_events` with principal, actor, target, cap, decision, result, deny_reason |
| **Authority allow+audit** | ✅ `coordinator.Authority.Authorize` + `Finish` → audit `allowed, result=ok` |
| **Authority deny+audit** | ✅ Capability mismatch → audit `denied, reason=scope_or_capability, result=ok` |
| **Deny after revoke** | ✅ Revoke full grant, keep inspect-only → `denied, reason=scope_or_capability, result=ok` |
| **Credential safety** | Remote URLs cleaned; token rotation recommended (was in process memory) |
