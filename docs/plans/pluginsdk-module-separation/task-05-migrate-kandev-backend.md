---
id: "05-migrate-kandev-backend"
title: "Migrate the Kandev backend to the SDK module"
status: pending
wave: 2
depends_on: ["04-publish-sdk-version"]
plan: "plan.md"
spec: "../../specs/plugins/spec.md"
---

# Task 05: Migrate the Kandev backend to the SDK module

- **Acceptance:** Backend runtime, host, fixture, installer, and tests import the
  new SDK/proto/manifest packages; old SDK/proto/manifest/packer copies are removed.
- **Acceptance:** `apps/backend/go.mod` requires the published SDK version and
  builds with both the root workspace and `GOWORK=off`.
- **Acceptance:** The E2E fixture package built with the new packer installs and
  activates through the existing plugin integration path.
- **Verification:** `cd apps/backend && go test -race ./internal/plugins/... ./cmd/plugin-fixture/...`
- **Verification:** `cd apps/backend && GOWORK=off go test ./internal/plugins/... ./cmd/plugin-fixture/...`
- **Verification:** `make -C apps/backend e2e-plugin-package` and the existing
  plugin package/install Playwright scenario through its documented target.
- **Files likely touched:** `apps/backend/go.mod`, `apps/backend/go.sum`,
  `apps/backend/internal/plugins/**`, `apps/backend/internal/backendapp/**`,
  `apps/backend/cmd/plugin-fixture/**`, `apps/backend/Makefile`, deleted
  `apps/backend/pkg/pluginsdk/**`, deleted `apps/backend/proto/kandev/plugin/v1/**`,
  deleted `apps/backend/cmd/plugin-pack/**`.
- **Dependencies:** Task 04's immutable SDK version.
- **Parallelism:** parallel-safe relative to Tasks 06–09 after Task 04 because it
  owns only the Kandev repository; sequential within this task due shared Go metadata.
- **Inputs:** SDK version recorded by Task 04 and compatibility tests from Task 02.
- **Output contract:** report import/removal inventory, workspace/offline module
  results, fixture artifact, E2E result, blockers/risks, and status updates.

## Results

Pending.
