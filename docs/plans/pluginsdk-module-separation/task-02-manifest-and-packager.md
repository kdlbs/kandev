---
id: "02-manifest-and-packager"
title: "Extract manifest validation and plugin packaging"
status: done
wave: 1
depends_on: ["01-sdk-protocol-module"]
plan: "plan.md"
spec: "../../specs/plugins/spec.md"
---

# Task 02: Extract manifest validation and plugin packaging

- **Acceptance:** `pluginsdk/manifest` owns the manifest model and validation
  tests, and `pluginsdk/pluginpack` plus `pluginsdk/cmd/plugin-pack` build packages
  without importing Kandev backend internals.
- **Acceptance:** The pack command can override only the archived manifest
  version for PR builds while preserving the committed manifest and generating
  valid `checksums.txt`.
- **Acceptance:** A package produced by the new command passes the existing
  backend install/checksum/platform validation.
- **Verification:** `cd pluginsdk && GOWORK=off go test -race ./manifest/... ./pluginpack/... ./cmd/plugin-pack/...`
- **Verification:** `make -C apps/backend e2e-plugin-package` followed by the
  targeted `pkgtar.Install` compatibility test named during implementation.
- **Files likely touched:** `pluginsdk/manifest/**`, `pluginsdk/pluginpack/**`,
  `pluginsdk/cmd/plugin-pack/**`, `apps/backend/internal/plugins/pkgtar/*_test.go`,
  `apps/backend/Makefile`.
- **Dependencies:** Task 01.
- **Parallelism:** sequential; shares module metadata and the protocol package
  established in Task 01.
- **Inputs:** plugin package format in the spec, current
  `internal/plugins/manifest`, `internal/plugins/pkgtar/pkgtartest`, and
  `cmd/plugin-pack` behavior.
- **Output contract:** report archive contents, version-override behavior,
  compatibility-test result, exact commands, risks, and status updates.

## Results

Added standalone `pluginsdk/manifest` and `pluginsdk/pluginpack` packages,
including the complete manifest model/validation tests and the deterministic
checksummed tar.gz writer. Added `pluginsdk/cmd/plugin-pack`, which walks a
plugin package directory, filters non-host binaries when requested, and can
override only the archived manifest version with `-version`. The source
`manifest.yaml` remains untouched; the override is applied to an in-memory
YAML node before checksums are generated.

The standalone command and tests import only the new module packages and
external YAML/Testify dependencies; no Kandev backend or `internal/`
package is required. The backend-owned copies remain for the staged migration.

Verification completed:

- `cd pluginsdk && GOWORK=off go test -race ./manifest/... ./pluginpack/... ./cmd/plugin-pack/...` (pass)
- `cd pluginsdk && GOWORK=off go test -race ./...` and `GOWORK=off go vet ./...` (pass)
- `make -C apps/backend e2e-plugin-package` (pass)
- `cd apps/backend && GOWORK=off go test ./internal/plugins/pkgtar ./cmd/plugin-pack -count=1` (pass)
- `TestStandalonePluginPack_InstallCompatibility` runs the new module
  packer and feeds its `-version pr-compatibility` archive through the
  real backend `pkgtar.Install` (pass).
- `git diff --check` (pass)

The compatibility bridge is intentionally retained until the backend
installer/SDK migration removes the duplicate host-side packer.
