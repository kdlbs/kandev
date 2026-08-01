---
id: "06-migrate-plugin-template"
title: "Migrate the plugin template and publish PR artifacts"
status: pending
wave: 2
depends_on: ["04-publish-sdk-version"]
plan: "plan.md"
spec: "../../specs/plugins/spec.md"
---

# Task 06: Migrate the plugin template and publish PR artifacts

- **Acceptance:** A clean template checkout uses the exact SDK pseudo-version
  from Task 04, has no Kandev filesystem replacement, and tests/packages with
  `GOWORK=off`.
- **Acceptance:** Its PR workflow creates a unique manifest version, verifies an
  all-platform tarball, and uploads only that `.tar.gz` as a short-retention artifact.
- **Acceptance:** README instructions cover PR artifact download/manual upload
  and preserve the normal tag-triggered release flow.
- **Verification:** `GOWORK=off make test vet package`
- **Verification:** inspect the tarball for `manifest.yaml`, all declared server
  binaries, UI assets, and `checksums.txt`; assert the archived PR version.
- **Verification:** confirm the opened PR's Actions artifact can be downloaded
  and its inner `.tar.gz` installed on a disposable compatible Kandev instance.
- **Files likely touched:** external repository `kdlbs/kandev-plugin-template`:
  `go.mod`, `go.sum`, `server/**`, `Makefile`, `README.md`, `.github/workflows/**`.
- **Dependencies:** Task 04.
- **Parallelism:** parallel-safe after Task 04; owns the template repository only.
- **Inputs:** SDK identifiers from Task 04 and PR package version override from Task 02.
- **Output contract:** report dependency diff, artifact name/version/contents,
  install result, workflow URL, risks, and status updates.

## Results

Pending.
