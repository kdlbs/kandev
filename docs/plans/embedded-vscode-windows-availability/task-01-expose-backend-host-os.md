---
id: "01-expose-backend-host-os"
title: "Expose backend host OS"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/embedded-vscode-windows-availability.md"
---

# Task 01: Expose backend host OS

## Acceptance

- Every SPA boot runtime includes `hostOS` from the running Kandev backend's `runtime.GOOS`.
- Embedded/static, dev, and `/api/v1/app-state` fallback payloads use the same runtime config.
- The frontend boot parser exposes valid string host values and ignores invalid values.

## Verification

Use TDD: add the Go serialization and frontend parsing assertions, observe them fail, implement the
boot contract, then rerun:

```bash
make -C apps/backend test
cd apps && pnpm --filter @kandev/web test -- src/boot-payload.test.ts
```

## Files likely touched

- `apps/backend/internal/webapp/payload.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/backendapp/helpers_test.go`
- `apps/web/src/boot-payload.ts`
- `apps/web/src/boot-payload.test.ts`

## Dependencies

None.

## Parallelism

Sequential. This task owns the backend-to-frontend contract consumed by Task 02.

## Inputs

- Spec sections: **What**, **API surface**, scenarios 1, 4, and 5.
- Plan sections: **Boot runtime host platform**, **Boot payload parsing**, and **Risks**.
- Existing patterns:
  - `apps/backend/internal/webapp/payload.go`
  - `apps/backend/internal/backendapp/helpers.go`
  - `apps/backend/internal/system/info/info.go`
  - `apps/web/src/boot-payload.ts`

## Risks

- Do not use request user-agent data; the value must be stable for every visitor to one backend.
- Keep the embedded handler and fallback JSON payload consistent.

## Output contract

Report the contract implemented, files changed, red and green commands/results, blockers or risks,
and update this task to `done` plus its checkbox/status in `plan.md`.
