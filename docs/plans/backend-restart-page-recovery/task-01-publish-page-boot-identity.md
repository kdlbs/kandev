---
id: "01-publish-page-boot-identity"
title: "Publish page boot identity"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001
acceptance_criteria:
  - AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.1
system_design:
  - ../../specs/platform/system-design/backend-restart-page-recovery.md
---

# Task 01: Publish Page Boot Identity

## Summary

Add the existing backend process identity to the application boot payload. Parse
it as the original document identity in the frontend.

## In scope

- Add `bootId` to backend and frontend runtime payload shapes.
- Source the value from the existing `info.Service` instance.
- Cover HTML boot and `/api/v1/app-state` boot paths.
- Accept a missing field for compatibility with partial fixtures.
- Add tests with a TDD red phase.

## Out of scope

- Identity comparison and WebSocket observation.
- The visible reload alert.
- Changes to `GET /health` or boot ID generation.

## Acceptance

- Each boot path returns the same process ID as `/api/v1/system/info`.
- The frontend parser keeps a valid non-empty ID.
- Missing or malformed values do not break application boot.

## Verification

```bash
make -C apps/backend test
cd apps && pnpm --filter @kandev/web test -- --run src/boot-payload.test.ts
```

## Files likely touched

- `apps/backend/internal/webapp/payload.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/backendapp/helpers_test.go`
- `apps/web/src/boot-payload.ts`
- `apps/web/src/boot-payload.test.ts`

## Dependencies

None.

## Risks

- HTML boot and API fallback payloads must use the same service instance.
- A required frontend field breaks older fixtures and partial boot tests.

## Parallelism

`sequential`

## Inputs

- Acceptance criterion 001.1.
- System-design section `Identity contract`.
- Existing `info.Service.Info().BootID` process identity.

## Results

Implemented the process-scoped boot ID in the backend runtime payload and
frontend parser. Both the HTML and `/api/v1/app-state` paths use the shared
payload builder, while missing or malformed IDs remain optional.

Verification passed:

- `make -C apps/backend test`: all backend packages passed.
- `pnpm --filter @kandev/web test -- --run src/boot-payload.test.ts`: passed.
