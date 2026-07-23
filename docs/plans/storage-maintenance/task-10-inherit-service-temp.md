---
id: "10-inherit-service-temp"
title: "Inherit the service temp environment"
status: done
wave: 6
depends_on: []
plan: "plan.md"
spec: "../../specs/system-page/storage-maintenance.md"
---

# Task 10: Inherit the service temp environment

Stop replacing `TMPDIR`, `TMP`, and `TEMP` for host-local agent instances and remove the superseded
uncommitted ownership/maintenance implementation.

## Acceptance

- Agentctl does not create `<system-temp>/kandev-agent/<instance>` or inject per-instance
  `TMPDIR`, `TMP`, or `TEMP`; configured service values are inherited unchanged and unset values stay
  unset at Kandev's environment-construction boundary.
- Existing agent stop/reap behavior remains intact, but there is no per-instance temp marker,
  descriptor, cleanup snapshot, Storage provider/API/UI action, or agent-temp E2E scenario in the
  final diff.
- `GOCACHE` behavior is unchanged: the existing managed cache is injected only when enabled, and
  no new Go cache is placed beneath the inherited temp directory.
- Red-first tests prove inherited/set/unset values and prove starting an instance does not create a
  `kandev-agent` root.

## Verification

```bash
cd apps/backend && go test ./internal/agentctl/server/process ./internal/agentctl/server/instance ./internal/agent/runtime/lifecycle ./internal/backendapp ./internal/system/storage/... ./internal/task/service
cd apps && pnpm --filter @kandev/web test -- --run components/settings/system/storage hooks/domains/system/use-storage-maintenance.test.tsx
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/process/manager_temp_test.go`
- superseded agent-temp DTO, lifecycle, task cleanup, Storage provider, frontend, and E2E files in
  the current uncommitted diff

## Dependencies

None.

## Inputs

- Spec: Agent session temporary data and scenarios
- ADR 0045 inherited-temp amendment
- Current `git diff`, whose marker/provider/UI changes are superseded rather than additive

## Output contract

Report the exact inherited environment behavior, removed superseded surfaces, red/green tests,
commands/results, blockers, and update this task plus the serialized plan checkbox when complete.
