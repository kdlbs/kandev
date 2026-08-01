---
id: "01-sdk-protocol-module"
title: "Create the standalone SDK and protocol module"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/plugins/spec.md"
---

# Task 01: Create the standalone SDK and protocol module

- **Acceptance:** `pluginsdk/` declares module
  `github.com/kdlbs/kandev/pluginsdk` and contains the public SDK plus generated
  `kandev.plugin.v1` code with no import of `apps/backend` or `internal/` packages.
- **Acceptance:** The moved SDK tests pass independently with `GOWORK=off`.
- **Acceptance:** Proto regeneration is deterministic and leaves the worktree clean.
- **Verification:** `cd pluginsdk && GOWORK=off go mod tidy && git diff --exit-code -- go.mod go.sum`
- **Verification:** `cd pluginsdk && GOWORK=off go test -race ./...`
- **Verification:** run the module proto target, then `git diff --exit-code -- pluginsdk/proto`
- **Files likely touched:** `pluginsdk/go.mod`, `pluginsdk/go.sum`,
  `pluginsdk/*.go`, `pluginsdk/*_test.go`, `pluginsdk/proto/**`,
  `pluginsdk/proto/buf.yaml`, `pluginsdk/proto/buf.gen.yaml`,
  `pluginsdk/Makefile`.
- **Dependencies:** None.
- **Parallelism:** sequential; establishes the module and generated contract used
  by every later task.
- **Inputs:** plugin spec Host API sections, ADR 0043, ADR 0047, ADR 0048,
  ADR 0050, `docs/plans/plugins/GRPC-CONTRACT.md`, and ADR-2026-08-01.
- **Output contract:** summarize moved APIs, module dependencies, generated-code
  result, exact tests, blockers/risks, and synchronize task/plan status.

## Results

Implemented the standalone `github.com/kdlbs/kandev/pluginsdk` module while
retaining the existing backend copy for the staged host migration. The new
module contains the author-facing SDK (`Plugin`, `Host`, data DTOs, transport
and conversion helpers), its tests, and the generated
`kandev.plugin.v1` protocol under `pluginsdk/proto/`. The proto source now
uses the standalone module's `go_package`, and `pluginsdk/Makefile` provides a
reproducible generation target using pinned generator versions.

Dependencies are limited to the SDK transport/test surface: HashiCorp
`go-plugin`, gRPC, protobuf, and Testify (with Go-managed indirect
dependencies). The module has no imports from `apps/backend`, the old SDK
path, or any `internal/` package.

Verification completed:

- `cd pluginsdk && GOWORK=off go mod tidy && git diff --exit-code -- go.mod go.sum`
- `cd pluginsdk && GOWORK=off go test -race ./...` (pass)
- `cd pluginsdk && GOWORK=off go vet ./...` (pass)
- `make -C pluginsdk proto` (pass); a second generation produced identical
  SHA-256 hashes for the proto source and both generated files
- `git diff --check` (pass)

The old backend SDK/proto copies remain intentionally until the immutable SDK
tag checkpoint and later backend migration task.
