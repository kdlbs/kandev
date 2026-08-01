---
id: "08-migrate-provider-usage"
title: "Migrate the provider-usage plugin and publish PR artifacts"
status: pending
wave: 2
depends_on: ["04-publish-sdk-version", "06-migrate-plugin-template"]
plan: "plan.md"
spec: "../../specs/plugins/spec.md"
---

# Task 08: Migrate the provider-usage plugin and publish PR artifacts

- **Acceptance:** `kdlbs/kandev-plugin-provider-usage` pins the Task 04 SDK
  commit, removes its local Kandev replacement, and passes repository tests with
  `GOWORK=off`.
- **Acceptance:** The plugin adopts the template's unique-version all-platform
  PR package workflow without changing release-version behavior.
- **Acceptance:** The downloaded PR tarball installs and its declared plugin
  behavior is smoke-tested on a disposable compatible Kandev instance.
- **Verification:** run the repository's documented format, vet, test, and
  `GOWORK=off make package` targets.
- **Verification:** inspect archive platforms/version/checksums and record the
  GitHub Actions artifact plus disposable install result.
- **Files likely touched:** external repository
  `kdlbs/kandev-plugin-provider-usage`: `go.mod`, `go.sum`, Go imports,
  `Makefile`, `README.md`, `.github/workflows/**`.
- **Dependencies:** Tasks 04 and 06.
- **Parallelism:** parallel-safe with Tasks 07 and 09 after the template pattern is merged.
- **Inputs:** migrated template workflow and SDK identifiers from Task 04.
- **Output contract:** report tests, artifact/install evidence, files, risks,
  and status updates.

## Results

Pending.
