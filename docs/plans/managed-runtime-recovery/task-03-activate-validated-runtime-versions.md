---
id: "03-activate-validated-runtime-versions"
title: "Activate validated runtime versions"
status: done
wave: 3
depends_on: ["01-build-exact-version-foundation", "02-route-active-host-runtime"]
plan: "plan.md"
spec: "../../specs/agents/requirements/runtime-updates.md"
---

# Task 03: Activate validated runtime versions

## Acceptance

- Preview returns the newest 50 stable versions plus active/current values,
  validates an optional target, and derives operation and exact command.
- POST accepts only `target_version`, re-resolves trusted npm metadata, and
  rejects malformed, prerelease, unpublished, disappeared, or package-like
  input before mutation.
- A candidate is prepared under its exact execution key, one failed preparation
  invalidates only that tree, and a successful ACP probe is required.
- Persistence happens before capability publication. Every failure preserves
  the previous active version and capability catalogue.
- Jobs and catalogue DTOs expose active version and operation while retaining
  current status, maintenance, output-bound, retention, and WebSocket behavior.

## TDD sequence

1. Extend fake-updater tests for catalogue parsing, operation classification,
   target request validation, and exact command/key construction.
2. Add orchestration tests for update, rollback, unknown-version repair,
   up-to-date no-op, targeted retry, failed probe, auth-required probe,
   persistence failure, and success ordering.
3. Add handler tests for query/body decoding and rejection before enqueue.
4. Implement DTOs, registry resolver, controller, job lifecycle, persistence,
   and catalogue projection in that order.
5. Run focused tests, backend lint, and generated/API compatibility checks used
   by the touched packages.

## Files likely touched

- `apps/backend/internal/agent/settings/dto/dto.go`
- `apps/backend/internal/agent/settings/controller/agent_update.go`
- `apps/backend/internal/agent/settings/controller/agent_update_job.go`
- `apps/backend/internal/agent/settings/controller/agent_update_test.go`
- `apps/backend/internal/agent/settings/controller/agent_discovery.go`
- `apps/backend/internal/agent/settings/handlers/handlers.go`
- `apps/backend/internal/agent/settings/handlers/agent_update_handlers_test.go`

## Verification

```bash
make -C apps/backend test ARGS='./internal/agent/settings/controller ./internal/agent/settings/handlers'
make -C apps/backend lint
```

## Risks

- Resolve registry metadata again after approval; do not trust a stale browser
  list as authorization.
- Do not publish candidate capabilities before the active selection is durable.
- Authentication-required is not a successful activation boundary.
- Keep raw registry credentials and unbounded output out of DTOs.

## Output contract

Record RED/GREEN evidence, ordering assertions, API examples, checks, and risks
in Results. Update this task and `plan.md` status.

## Results

RED covered catalogue projection, operation classification, target validation,
exact preparation retry, failed-probe preservation, persistence-before-
publication, and required request-body behavior. GREEN verification:

- `rtk go test ./internal/agent/managedruntime ./internal/agent/agents ./internal/agent/hostutility ./internal/agent/runtime/lifecycle ./internal/agent/settings/controller ./internal/agent/settings/handlers ./internal/backendapp` — 2,321 tests passed across 7 packages.
- `rtk make -C apps/backend lint` — 0 issues.

The controller re-resolves trusted npm metadata after approval, the candidate
probe does not replace live capabilities, and the active selection is saved
before capabilities are published. Failed preparation, probe, authentication,
or persistence leaves the previous active selection and capability cache in
place. Handler tests cover JSON target decoding and invalid-target responses.
