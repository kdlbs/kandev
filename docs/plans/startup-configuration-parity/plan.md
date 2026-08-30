---
spec: docs/specs/platform/requirements/startup-configuration-parity.md
decision: docs/decisions/2026-08-20-startup-configuration-source-parity.md
created: 2026-08-20
status: complete
---

# Implementation Plan: Startup Configuration Parity

## Overview

Create one typed startup configuration contract for stable operator settings.
Add automatic home configuration discovery. Make the Go launcher, backend, and
managed agentctl processes use the same resolved values. Keep environment
variables as compatible overrides.

The work starts with the common catalog and source resolver because every other
task depends on those contracts. Launcher work follows so it can preserve the
selected file and its bootstrap values. Backend consumers then move from direct
environment reads to typed configuration. Agentctl settings use explicit child
configuration across all executor paths. The final product slice reports YAML
locks accurately in Message Queue settings and publishes the complete reference.

Implementation is sequential by default. Tasks 03 and 04 use different consumer
packages after Task 01 defines their fields, but both depend on shared startup
wiring from Task 02. No delegated execution is authorized by this plan.

## Configuration contract

The common configuration package owns a catalog entry for every stable startup
setting. An entry records its YAML key, environment aliases, sensitivity,
default policy, and owner. A separate exclusion list records internal,
generated, test, debug-only, deprecated, and externally managed variables.
Completeness tests compare the catalog with the audited public startup inventory.

File selection uses the working directory, resolved home, then `/etc/kandev`.
The first existing candidate wins. Errors do not fall through. A private child
handoff preserves the selected path when the launcher changes the backend
working directory.

The implementation keeps YAML values in typed configuration. It does not create
public environment variables from those values. Consumers accept typed values
or explicit startup options.

## Implementation waves

### Wave 1: Shared contract

- [x] [Task 01: Configuration catalog and source resolution](task-01-configuration-catalog-and-source-resolution.md)

### Wave 2: Launcher bootstrap

- [x] [Task 02: Launcher configuration consumption](task-02-launcher-configuration-consumption.md)

### Wave 3: Backend consumers

- [x] [Task 03: Backend security and lifecycle settings](task-03-backend-security-and-lifecycle-settings.md)
- [x] [Task 04: Backend capacity and service settings](task-04-backend-capacity-and-service-settings.md)

### Wave 4: Managed processes and product source reporting

- [x] [Task 05: Agentctl settings propagation](task-05-agentctl-settings-propagation.md)
- [x] [Task 06: Message queue configuration lock](task-06-message-queue-configuration-lock.md)

### Wave 5: Public contract

- [x] [Task 07: Public configuration reference](task-07-public-configuration-reference.md)

## Cross-task acceptance

- Every stable operator-facing startup environment variable has a canonical
  YAML key or a reviewed exclusion.
- Launch flags override environment values. Environment values override YAML.
  YAML overrides database, profile, or built-in defaults as applicable.
- The launcher and backend use one selected file and agree on bootstrap values.
- Working-directory configuration wins over home configuration. Home
  configuration wins over system configuration.
- An invalid or unreadable first candidate fails startup.
- A selected home file cannot relocate itself through `homeDir`.
- Secret values never appear in logs. Broad Unix file permissions produce a
  warning for secret-bearing files.
- Managed agentctl processes receive YAML-backed settings through Local,
  Worktree, Docker, Sprite, and SSH launch paths.
- Message Queue settings report `configuration` for a YAML lock on desktop and
  mobile.
- Existing environment-only deployments keep their behavior.

## Verification strategy

Each task follows TDD and records its RED and GREEN commands. The final combined
gate runs backend configuration, launcher, runtime flag, queue, and agentctl
tests. It also runs the focused frontend unit and responsive E2E tests, the
internationalization checks, the public documentation validator, and a backend
build.

The implementation turn must inspect current package paths before running each
command. If a package moved after this plan, update the task file with the exact
replacement instead of weakening the check.

## Risks

- Home discovery depends on the home path. Bootstrap resolution must not load a
  home file before it knows that path.
- A launcher working-directory change can make the backend select a different
  file. The private selected-path handoff must pin the same file.
- Current package-level environment reads run during initialization. They must
  move behind explicit startup configuration without introducing mutable global
  state.
- Agentctl runs through several executors. A missing propagation path can make
  behavior depend on executor choice.
- The runtime flag registry already owns feature and debug metadata. The new
  catalog must integrate with it instead of adding a second feature flag map.
- Environment parsing has historical fallback rules. YAML validation must fail
  clearly without silently changing those legacy environment rules.
- File mode checks differ by platform. Unix warnings need focused tests and a
  safe no-op on platforms without compatible permission bits.

## Out of scope

- `.env` loading, public `--config`, file merging, and live reload.
- A startup configuration editor in Settings.
- New keys for internal child wiring, generated tokens, packaging, mocks, E2E,
  debug-only controls, or UI-managed credentials.

## Verification results

- The common configuration, launcher, backend consumer, queue, agentctl, and
  lifecycle package suite passed 5,691 tests across 16 packages.
- The focused queue API and frontend unit suites passed 65 backend tests and 27
  frontend tests.
- The five-locale i18n check and public documentation validator passed.
- The final browser E2E, backend build, full repository verification, commit,
  and PR checks are recorded in the delivery session and task results.
