---
id: "09-migrate-kandy"
title: "Migrate Kandy and publish PR artifacts"
status: pending
wave: 2
depends_on: ["04-publish-sdk-version", "06-migrate-plugin-template"]
plan: "plan.md"
spec: "../../specs/plugins/spec.md"
---

# Task 09: Migrate Kandy and publish PR artifacts

- **Acceptance:** `kdlbs/kandev-plugin-kandy` pins the Task 04 SDK commit,
  removes `replace github.com/kandev/kandev => ../kandev/apps/backend`, and
  passes Go and UI tests with `GOWORK=off`.
- **Acceptance:** Kandy's build workflow uploads a uniquely versioned,
  all-platform PR `.tar.gz`, while tag releases retain the manifest/Makefile
  release version and existing release assets.
- **Acceptance:** The downloaded PR package upgrades or installs on a disposable
  compatible Kandev instance and Kandy's state, webhook, and top-bar UI smoke
  checks pass without touching a primary Kandy ledger.
- **Verification:** `GOWORK=off make test vet package`
- **Verification:** inspect archive platforms/version/checksums and record the
  GitHub Actions artifact plus disposable lifecycle/UI smoke result.
- **Files likely touched:** external repository `kdlbs/kandev-plugin-kandy`:
  `go.mod`, `go.sum`, `server/**`, `Makefile`, `README.md`,
  `.github/workflows/build.yml`, `.github/workflows/ci.yml`,
  `.github/workflows/release.yml` if shared packaging inputs change.
- **Dependencies:** Tasks 04 and 06.
- **Parallelism:** parallel-safe with Tasks 07–08 after the template pattern is merged.
- **Inputs:** migrated template workflow, Task 04 SDK identifiers, and Kandy's
  existing Go/UI/release checks.
- **Output contract:** report tests, artifact/install evidence, state-safety
  boundary, files, risks, and status updates.

## Results

Pending.
