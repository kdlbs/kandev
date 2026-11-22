---
id: "01-bounded-file-access"
title: "Bounded reads and external permission"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-FILE-VIEWERS-002
  - REQ-PLUGINS-FILE-VIEWERS-006
acceptance_criteria:
  - AC-PLUGINS-FILE-VIEWERS-002.1
  - AC-PLUGINS-FILE-VIEWERS-002.2
  - AC-PLUGINS-FILE-VIEWERS-002.3
  - AC-PLUGINS-FILE-VIEWERS-002.4
  - AC-PLUGINS-FILE-VIEWERS-002.5
  - AC-PLUGINS-FILE-VIEWERS-006.1
  - AC-PLUGINS-FILE-VIEWERS-006.2
  - AC-PLUGINS-FILE-VIEWERS-006.3
  - AC-PLUGINS-FILE-VIEWERS-006.4
  - AC-PLUGINS-FILE-VIEWERS-006.5
  - AC-PLUGINS-FILE-VIEWERS-006.6
system_design:
  - ../../specs/plugins/system-design/file-viewers.md
---

# Task 01: Bounded reads and external permission

## Summary

Implement host-owned external policy storage and revisioned admission with bounded
raw-byte streaming to the browser. Enforce the selected execution identity and
single-file approval. This is the prerequisite transport, with API-level proof.

## In scope

Own new preview service, host-only policy handlers, executor transport, cancellation,
resource reservations, and a test consumer. Use existing authenticated request and
opaque-installation patterns. Do not change existing text reads.

## Out of scope

Other work orders, publication, and changes to unrelated editor behavior.

## Acceptance

- No external byte is read or delivered without current policy or a valid one-use approval.
- Cross-scope, stale, replaced-target, special-file, and over-limit requests fail closed.
- Streaming and cancellation pass local/remote tests without unbounded buffers or queues.

## Verification

Run from the Kandev repository root unless specified otherwise. New suite names
are implementation deliverables, not existing test evidence.

```bash
(cd apps/backend && go test ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/backendapp ./internal/plugins -run 'TestFilePreview|TestPreparePluginFilePreview' -count=1)
(cd apps/backend && go test -race ./internal/plugins ./internal/agentctl/server/process -run 'TestFilePreview|TestPreparePluginFilePreview' -count=1)
(cd apps/backend && go test ./internal/agentctl/server/process -run '^$' -fuzz FuzzFilePreviewPath -fuzztime=30s)
```

## Files likely touched

apps/backend/internal/agentctl/server/process/workspace_files.go;
apps/backend/internal/agentctl/server/api/; internal/agent/runtime/agentctl/;
internal/task/handlers/; internal/plugins/ (backend-relative paths).

## Dependencies

None

## Risks

Path races, approval replay, distributed admission, and missing executor test hosts.

## Parallelism

Sequential. No delegation authorized.

## Inputs

- [Requirements](../../specs/plugins/requirements/file-viewers.md).
- [Design](../../specs/plugins/system-design/file-viewers.md).
- [Security matrix](../../reviews/file-viewers-security.md).
- Existing registration, approval, file-reader, and marketplace tests beside the scoped source.

## Results

Implementation is present. See the [implementation record](../../reviews/file-viewers-implementation.md)
for completed checks and remaining validation gates. No publication is authorized.
