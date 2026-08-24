---
id: "06-update-awareness-e2e"
title: "Prove desktop and mobile flows"
status: complete
wave: 5
depends_on: ["05-settings-update-indicator"]
plan: "plan.md"
spec: "../../specs/agents/requirements/runtime-updates.md"
---

# Task 06: Prove desktop and mobile flows

## Acceptance

- Desktop E2E proves the update dot, authoritative preview, no pre-approval
  mutation, successful dot removal, persisted selection, and partial-unknown
  behavior through the UI.
- Mobile E2E taps the dotted 44 px trigger, completes the same update flow in
  the existing drawer, and proves containment and no document horizontal
  overflow.
- Tests use deterministic status/preview fixtures, stable test IDs, causal
  waits, and no raw sleeps.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile && cd web && pnpm e2e:run tests/settings/agent-runtime-update.spec.ts && pnpm e2e:run --project mobile-chrome tests/settings/mobile-agent-runtime-update.spec.ts
```

## Files likely touched

- `apps/web/e2e/tests/settings/agent-runtime-update.spec.ts`
- `apps/web/e2e/tests/settings/mobile-agent-runtime-update.spec.ts`
- `apps/web/e2e/tests/settings/agent-runtime-update-helpers.ts`

## Dependencies

Task 05.

## Parallelism

Parallel-safe with Task 07 after Tasks 01-05. This task owns only E2E files.

## Inputs

- Spec scenarios for update, unknown status, persistence, and mobile parity
- `/e2e` fixture and cleanup rules
- Existing runtime-update desktop/mobile specs and helper routes

## Output contract

Report RED/GREEN evidence, exact discovered test counts and commands, rendered
artifact paths if produced, cleanup evidence, risks, and synchronized task/plan
status.

## Results

Complete. Desktop coverage proves the blue dot, authoritative preview, no
pre-approval POST, successful dot removal, persisted active selection after
reload, unknown-status usability, and structural return-to-default behavior.
Mobile coverage proves the dotted 44 px trigger, drawer update flow, contained
scrolling, and no document horizontal overflow.

Verification: the production-build desktop run
`pnpm e2e:run --project chromium tests/settings/agent-runtime-update.spec.ts`
passed 15/15, and the production-build mobile run
`pnpm e2e:run --project mobile-chrome
tests/settings/mobile-agent-runtime-update.spec.ts` passed 4/4. No raw sleeps
were added and the fixture uses causal waits.

Follow-up review coverage adds the complete backend projection and successful
default-reset state, including removal of the stale active-version display.
