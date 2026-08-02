---
id: "07-diagnostic-bundle-backend"
title: "Diagnostic bundle backend"
status: completed
wave: 3
depends_on: ["01-backend-log-sinks"]
plan: "plan.md"
spec: "../../specs/platform/diagnostic-logging.md"
---

# Task 07: Diagnostic bundle backend

## Acceptance

- Authenticated users can create identity-owned bundle jobs for any non-empty
  subset of `backend` and `frontend`, inspect status, upload bounded frontend
  chunks, and download ready/partial ZIPs.
- Backend-only jobs copy the strict retained daily file set and build without
  waiting for a browser.
- Frontend jobs send `system.logs.capture_requested` only to WebSocket clients
  matching the caller identity, collect for at most 15 seconds, deduplicate by
  browser profile, atomically bind the first per-tab capture stream for each
  browser, and validate sequential chunk/body/browser bounds.
- ZIPs contain only server-owned paths under `backend/`, `frontend/`, and
  `manifest.json`; source symlinks, path traversal, and non-regular files are
  rejected.
- Jobs, working files, and ZIPs are owner-only. Collection/build has a
  five-minute hard lifetime; ready/partial ZIPs expire after 15 minutes.
- Enforce one active job per identity, eight collecting/queued process-wide,
  and one builder. Equivalent source sets coalesce; conflicts return `429` and
  global saturation `503`, both with `Retry-After: 5`.
- Enforce four browsers, 20 MiB/profile, 80 MiB frontend, 160 MiB backend,
  256 MiB uncompressed payload, and 384 MiB temporary-disk limits. Backend
  truncation selects newest byte ranges and makes the manifest/job partial.
- ZIP payloads use `Store` and 1 MiB yielded chunks rather than deflate.

## Verification

```bash
cd apps/backend
go test ./internal/system/logbundle ./internal/system ./internal/gateway/websocket
```

```bash
cd apps/backend
go test -race ./internal/system/logbundle ./internal/gateway/websocket
```

## Files likely touched

- `apps/backend/internal/system/logbundle/service.go`
- `apps/backend/internal/system/logbundle/job.go`
- `apps/backend/internal/system/logbundle/archive.go`
- `apps/backend/internal/system/logbundle/handler.go`
- `apps/backend/internal/system/logbundle/*_test.go`
- `apps/backend/internal/system/system.go`
- `apps/backend/internal/gateway/websocket/hub.go`
- `apps/backend/internal/gateway/websocket/hub_broadcast_test.go`
- `apps/backend/internal/backendapp/helpers.go`

## Dependencies

- Task 01 provides the strict active/dated backend file set.

## Parallelism

Parallel-safe with Task 04 after Task 01. It owns backend bundle/gateway files;
Task 04 owns frontend toast files.

## Inputs

- Spec: System Logs page and bundle contents, Diagnostic bundle jobs,
  Permissions, Failure modes, and bundle Scenarios.
- Plan: Diagnostic bundle backend.
- ADR: files are authoritative, capture is explicit, agents select sources.
- Existing patterns: system route composition, WebSocket identity routing,
  owner-scoped jobs, and bounded archive/file services.

## Risks

- The bundle owner comes only from authenticated context; browser IDs are
  dedupe hints and cannot authorize access.
- Capture crosses WebSocket notification and HTTP chunk upload. Late,
  duplicate, disconnected, expired, and out-of-order responses must converge
  deterministically.
- A browser ID is claimed atomically by one `capture_stream_id`; losing streams
  are acknowledged before entry decoding so duplicate tabs cannot amplify CPU.
- ZIP entry names and source paths must never use client-supplied path data.
- Background cleanup must stop and join without goroutine leaks.
- Bundle work must not run expensive filesystem or archive work in request
  handlers or permit an unbounded number of concurrent exports.
- Admission checks temporary free space before accepting a job; cancellation
  removes partial files without waiting for the 15-minute ready lease.

## Output contract

Report the final job state machine, identity routing, limits, archive layout,
files changed, exact test commands/results, blockers/risks, and update this task
plus `plan.md` status in the same conversation.
