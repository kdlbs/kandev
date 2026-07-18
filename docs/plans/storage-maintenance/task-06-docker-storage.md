---
id: "06-docker-storage"
title: "Docker storage provider"
status: done
wave: 2
depends_on: ["02-storage-persistence"]
plan: "plan.md"
spec: "../../specs/system-page/storage-maintenance.md"
---

# Task 06: Docker storage provider

Analyze Docker disk use, clean positively owned stopped containers, and gate daemon-global actions.

## Acceptance

- Container cleanup is label- and inventory-scoped, retains running/unrelated containers, and uses
  the existing attached-volume removal behavior.
- Build-cache and unused-image analysis is read-only; deletion is rejected unless the persisted
  dedicated-daemon acknowledgment is still true immediately before the SDK call.
- No code path exposes global volume/network prune or shells out to `docker system prune`.

## Verification

```bash
cd apps/backend && go test ./internal/agent/docker ./internal/system/storage/dockerstore
```

## Files likely touched

- `apps/backend/internal/agent/docker/client.go`
- `apps/backend/internal/agent/docker/client_test.go`
- `apps/backend/internal/system/storage/dockerstore/provider.go`
- `apps/backend/internal/system/storage/dockerstore/provider_test.go`

## Dependencies

- Task 02 for settings validation and run result types.

## Inputs

- Spec Docker storage section
- Existing `kandev.managed=true` labels and `RemoveContainer(...RemoveVolumes: true)` behavior
- ADR 0009 container fail-closed rules

## Output contract

Report Docker SDK contracts/filters, capability degradation, scope guarantees, tests run,
blockers/risks, and update this task plus `plan.md` to done.
