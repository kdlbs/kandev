---
created: 2026-10-09
status: complete
requirements:
  - REQ-EXECUTORS-AGENTCTL-AUTH-MODES-001
system_design:
  - ../../specs/executors/system-design/agentctl-authentication-modes.md
legacy_specs: []
---

# Implementation plan: Sprite agentctl authentication

## Overview

Restore agent startup on a Sprite whose controller uses the existing tokenless mode. Deliver the correction and its regression tests in one sequential work order.

The requirement gap is narrow: existing specifications define authenticated standalone ownership but do not explicitly define compatibility with tokenless remote startup. The new capability pair records that existing contract.

## Evidence and root cause

The investigation inspected task `ab090d65-c726-47dc-b881-fa066cb4d12f`, session `704a4e0c-c0c0-4dad-aa36-e1ffd01d2cae`, on 2026-10-09.

| UTC time | Observed result |
| --- | --- |
| 13:59:09 | Task startup began |
| 14:02:08.872 | Environment preparation completed successfully |
| 14:02:10.201 | Agent configuration failed with HTTP 401 and `missing or invalid Authorization header` |
| 14:02:10.253 | Kandev preserved Sprite `kandev-bb0165b1-2d3` after bootstrap failure |
| 14:04:42 | Read-only probes returned health 200 and instance status 401 through the retained forward |

Evidence came from `/root/.kandev/logs/backend-logs.log`, task conversation records, and read-only HTTP probes. Log timestamps used Lisbon time, one hour ahead of UTC.

The Sprite client intentionally omits `WithAuthToken`. The control server allows tokenless instance creation, but production wiring attaches a nonnil empty credential source to the instance.

`instanceAuth` then requires a bearer header. Configuration fails before the model starts. The generic bootstrap classifier records `cause=unknown`.

The inconsistent wiring and middleware arrived in commit `e75c8b37c88` (PR #3467). Existing middleware tests did not exercise tokenless production factory wiring.

## Scope

### In scope

- Conditional credential-source attachment at agentctl startup.
- A regression through real agentctl control, instance, and agent-stream endpoints.
- Authenticated startup, credential rotation, and listener compatibility checks.
- Durable requirements and design for the existing authentication modes.

### Out of scope

- Bootstrap classification, frontend markup, translations, and recovery-card changes.
- New Sprite credentials, handshake protocols, feature flags, or persistence changes.
- Cloud provisioning changes or repair of the user's retained task and workspace.
- Changes to completed standalone-survival work orders or their recorded results.

## Technical approach

Update the server factory in `apps/backend/cmd/agentctl/main.go:run` to attach the rotating source only for nonempty startup `Config.AuthToken`.

Keep `internal/agentctl/server/api/auth.go` behavior unchanged. Keep the Sprite client's existing tokenless construction unchanged.

Add `apps/backend/cmd/agentctl/authentication_modes_test.go` to exercise real startup in a subprocess. Use temporary configuration and ephemeral ports.

The fixture must not duplicate the production factory. A helper subprocess can call `run` through an explicit test argument and environment marker. Bound startup, requests, and teardown.

Build the existing `cmd/mock-agent` fixture once per test into a temporary directory. Use a deterministic message prompt and assert output plus terminal completion.

The [design compatibility matrix](../../specs/executors/system-design/agentctl-authentication-modes.md#compatibility) defines coverage and unsupported-shape limits.

## Tests

| Acceptance | Test evidence |
| --- | --- |
| 001.1, 001.2 | New `TestAgentctlAuthenticationModes/tokenless` in `cmd/agentctl/authentication_modes_test.go` |
| 001.3, 001.4 | New `TestAgentctlAuthenticationModes/authenticated`, including bootstrap handshake and post-rotation instance creation |
| 001.4 | Existing credential rotation and stream-invalidation tests in `internal/agentctl/server/api` |
| 001.5 | Both new startup modes, with missing and incorrect credentials rejected in authenticated mode |
| 001.6 | Existing `TestListenHost` cases and `TestSpritesBuildInstanceResult` |

The required red test is `TestAgentctlAuthenticationModes/tokenless`: configuration must succeed, but the current code returns 401.

## E2E tests

The new Go subprocess test is the end-to-end evidence for this backend boundary. It uses production startup and a real ACP mock-agent turn.

No Playwright file or project applies because this package changes no browser flow. No paid provider or live Sprite is required.

An optional live smoke test must use a disposable Sprite with the corrected uploaded binary. Record it separately from deterministic regression results.

## Work orders

- [x] [Task 01: Restore tokenless agentctl startup](task-01-restore-tokenless-startup.md)

## Verification results

Implementation checks on 2026-10-09:

- The required pre-fix tokenless regression failed with HTTP 401 and `missing or invalid Authorization header` at instance configuration.
- `go test -trimpath ./cmd/agentctl -run '^TestAgentctlAuthenticationModes$' -count=1 -timeout=5m`: passed for tokenless and authenticated startup, including bootstrap, rotation, existing and new instances, and a completed mock-agent turn.
- `go test -trimpath -race ./cmd/agentctl ./internal/agentctl/server/api ./internal/agentctl/server/config -count=1 -timeout=10m`: passed.
- `go test -trimpath ./internal/agent/runtime/lifecycle -run '^TestSpritesBuildInstanceResult' -count=1 -timeout=3m`: passed.
- `go build -trimpath -o /tmp/kandev-agentctl-auth-check-b1668267 ./cmd/agentctl`: passed.
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all`: passed.
- Local `validateCoverage` preflight: passed with status `covered` for the production path and all four package artifacts.
- `git diff --check`: passed.

Design-package checks on 2026-10-09:

- `python3 scripts/list-docs.py validate`: passed, 368 decisions and 1498 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `validateCoverage` from `.github/scripts/pr-docs.cjs`: passed with status `covered` for the planned production path and all four artifacts.
- `git diff --check`: passed. Package artifacts were included with the implementation.
- PR review follow-up: the authenticated-mode test rejects an empty Bearer credential on both control and instance routes, and fixture cleanup bounds each instance deletion with a 15-second context. `go test -trimpath -race ./cmd/agentctl -run '^TestAgentctlAuthenticationModes' -count=1 -timeout=5m` passed. These updates affect test coverage and cleanup only; the authentication contract is unchanged.

The [work order](task-01-restore-tokenless-startup.md#verification) contains exact implementation commands and the local documentation preflight.

## Risks

- A guard based on request credentials can accidentally disable authenticated protection. Use resolved startup configuration only.
- A static token replacement can break rotation. Preserve the shared source for authenticated startup.
- A copied test factory can miss a production wiring regression. Enter through `run` in an isolated process.
- Existing Sprites can retain an older uploaded controller. A successful backend deployment alone does not prove recovery of those instances.

## Documentation impact

This package restores supported startup behavior without new operator settings or actions. Public documentation changes are not required for the correction.

The new requirement and design own the compatibility contract. The completed [standalone survival package](../agent-survival-across-restart/plan.md) remains historical evidence.
