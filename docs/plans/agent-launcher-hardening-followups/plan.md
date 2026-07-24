---
spec: docs/specs/office/agents.md
created: 2026-07-24
status: done
---

# Implementation Plan: Agent Launcher Hardening Follow-ups

## Overview

Deliver an explicitly limited per-boot settings interlock while preserving the
long-term operator boundary in
`docs/decisions/2026-07-24-operator-owned-agent-launcher-settings.md`. In
parallel, remove lossy command-string execution, then make restart and launch
rollback ordering fail safely. Finish with the verified review cleanups,
mobile settings coverage, and public documentation that accurately distinguishes
risk reduction from authentication.

## Backend

### Interim settings interlock

- Add a process-random boot token and constant-time validation middleware under
  `apps/backend/internal/common/httpmw/`.
- Generate the token once in `apps/backend/internal/backendapp/main.go`, pass it
  through `routeParams`, and expose it only as the existing unauthenticated SPA
  runtime payload.
- Apply the interlock to state-changing agent/settings routes in
  `apps/backend/internal/agent/settings/handlers/handlers.go`, including agent
  creation/update/delete, custom TUI creation, profile creation/update/delete,
  MCP configuration mutation, and agent installation.
- Reject requests carrying a bearer credential on those routes so the injected
  Office runtime JWT is never accepted as the UI interlock. Missing/wrong boot
  tokens fail closed.

### Structured command transport

- Extend lifecycle execution state and the lifecycle-to-agentctl configure
  request with `agent_args` and `continue_args`.
- Dual-write structured argv and legacy display strings. Agentctl prefers a
  present structured field, rejects present-but-invalid argv, and uses
  `strings.Fields` only when the structured field is absent.
- Preserve argv through initial launch, continue/one-shot execution,
  restart/context reset, standalone, Docker/remote configuration, and recovery.
- Validate argv through the shared executable-token rules: non-empty argv and a
  non-empty, non-flag-like `argv[0]`.

### Restart and rollback safety

- Build and validate replacement initial/continue argv before closing streams,
  stopping the existing process, changing execution state, or configuring
  agentctl.
- When execution construction fails after an executor instance exists, stop the
  instance exactly once and never register/configure/start the failed execution.

## Frontend

### Interlock transport

- Add the boot token to the typed runtime payload in
  `apps/web/src/boot-payload.ts`.
- Centralize the mutation header builder in the web API layer and apply it to
  every state-changing agent/profile action, preserving existing desktop and
  mobile save flows without layout changes.

### Review cleanups

- Replace the fixed timeout in `command-preview-card.test.tsx` with controlled
  promises and `waitFor`.
- Normalize E2E `AgentProfile.commandPrefix` typing and remove wire-shape casts
  from the mobile settings test.

Mobile design contract: the existing mobile agent-profile route, fields, save
action, scroll owner, and touch targets remain unchanged. Only the request
transport and test typing change, so the existing
`mobile-agent-profile-config-selector.spec.ts` remains the mobile parity proof.

## Tests

- Interlock middleware: missing, wrong, valid, empty-server-token, and bearer
  rejection.
- Settings routes: Office bearer denied; omitted token denied; valid UI token
  succeeds; guarded operations make zero writes on rejection.
- Structured argv: spaces, quotes, empty non-executable arguments,
  Windows/backslash paths, prefix arguments, CLI flag values, initial,
  continue, reset, JSON compatibility, and legacy absence fallback.
- Restart: resolver failure and malformed persisted prefix cause no
  Stop/Configure/Start call and no state mutation.
- Launch rollback: instance is stopped once, absent from execution store, and
  never configured/started after construction failure.
- Preview validation: empty/flag-like prefix executable rejected using the same
  semantics as save/launch.
- Profile creation validation: bad prefixes produce zero store/profile writes.

## E2E Tests

- Rebuild backend and Vite production artifacts.
- Run the existing mobile agent-profile settings test through create, set
  prefix, save, clear, and save again. The browser must carry the boot token
  automatically; direct E2E API setup helpers receive the token through their
  controlled test bootstrap where mutations are required.

## Implementation Waves

Wave 1 (parallel):

- [x] [task-01-interim-settings-interlock](task-01-interim-settings-interlock.md)
- [x] [task-02-structured-argv-transport](task-02-structured-argv-transport.md)

Wave 2:

- [x] [task-03-restart-and-launch-rollback](task-03-restart-and-launch-rollback.md)

Wave 3:

- [x] [task-04-review-cleanups-e2e-docs](task-04-review-cleanups-e2e-docs.md)

## Risks

- The boot token is deliberately replayable by an intentional agent and must
  never be described as operator authentication.
- Structured-field presence must be distinguishable from an absent JSON field;
  a present invalid slice must fail instead of silently falling back.
- Lifecycle task ownership overlaps between tasks 02 and 03, so they run in
  separate waves.
- E2E helpers that mutate settings must use the same guarded transport without
  weakening production middleware.

## Verification

Targeted commands are owned by each task. After all tasks:

```bash
make fmt
make typecheck test lint
cd apps/web && pnpm e2e:run tests/settings/mobile-agent-profile-config-selector.spec.ts
```

Commit through active hooks, run delegated full `/verify`, then push and open a
PR against `main`.
