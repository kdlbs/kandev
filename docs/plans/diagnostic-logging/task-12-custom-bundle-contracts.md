---
id: "12-custom-bundle-contracts"
title: "Custom bundle contracts and runtime index"
status: done
wave: 9
depends_on:
  - "07-diagnostic-bundle-backend"
  - "11-merge-risk-hardening"
plan: "plan.md"
spec: "../../specs/platform/requirements/diagnostic-logging.md"
---

# Task 12: Custom bundle contracts and runtime index

## Acceptance

- Bundle jobs accept unique `backend`, `frontend`, `runtime`, and `acp`
  sources; ACP requires one to ten unique session IDs, and coalescing includes
  the normalized session set.
- Capabilities and ACP-candidate endpoints use backend-authoritative debug
  state and existing task authorization: owners see their sessions, admins may
  see all, and foreign IDs are rejected without existence disclosure.
- `runtime/sessions.json` contains only the approved bounded session/runtime
  DTO fields, newest-first, with no titles, identities, arbitrary metadata,
  messages, prompts, tool payloads, files, or configuration.

## Verification

```bash
cd apps/backend
go test ./internal/system/logbundle ./internal/system ./internal/backendapp
go test -race ./internal/system/logbundle ./internal/system
```

## Files likely touched

- `apps/backend/internal/system/logbundle/job.go`
- `apps/backend/internal/system/logbundle/service.go`
- `apps/backend/internal/system/logbundle/archive.go`
- `apps/backend/internal/system/logbundle/handler.go`
- `apps/backend/internal/system/logbundle/runtime_index.go`
- `apps/backend/internal/system/logbundle/*_test.go`
- `apps/backend/internal/system/system.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/backendapp/helpers_test.go`

## Dependencies

- Task 07 provides the current owner-scoped job/archive service.
- Task 11 fixes the current resource ceilings this extension must preserve.

## Parallelism

Sequential. Task 13 depends on these source, authorization, job-key, and
manifest contracts and touches the same logbundle files.

## Inputs

- Spec: System Logs page and bundle contents, Diagnostic bundle jobs,
  Permissions, Failure modes, and runtime-index scenarios.
- Plan: Backend source and permission contracts.
- ADR: raw ACP is separate explicit consent; runtime metadata is field
  allow-listed rather than a database export.

## Risks

- Generic model/metadata serialization could silently add message-bearing or
  identity fields later; the DTO must be explicit and pinned by negative tests.
- Authorization must complete before filesystem/executor access and must not
  reveal whether a foreign session exists.
- No backend database migration is required or permitted for this extension.

## Output contract

Report request/response shapes, authorization behavior, runtime DTO fields,
source/job-key changes, exact tests/results, blockers/risks, and synchronize
this task plus `plan.md` status.

## Results

Implemented source/session contracts and runtime-index plumbing.

- Added backend-authoritative `backend`, `frontend`, `runtime`, and `acp`
  source normalization with deterministic selected-session coalescing keys.
- Added capability and ACP-candidate endpoints with debug gating and bounded
  allow-listed session records.
- Added the task-service adapter with owner/admin authorization before session
  metadata access and no generic task/session serialization.
- Added `runtime/sessions.json` archive output and manifest counts.
- Verification: `go test ./internal/system/logbundle ./internal/system ./internal/backendapp -count=1` (pass; 251 tests).
- Verification: `go test ./internal/backendapp -run TestDiagnosticSessionProvider -count=1` (pass).
