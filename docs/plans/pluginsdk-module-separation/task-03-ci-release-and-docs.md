---
id: "03-ci-release-and-docs"
title: "Wire SDK validation, release metadata, and author documentation"
status: done
wave: 1
depends_on: ["02-manifest-and-packager"]
plan: "plan.md"
spec: "../../specs/plugins/spec.md"
---

# Task 03: Wire SDK validation, release metadata, and author documentation

- **Acceptance:** Kandev CI runs SDK tidy, format, vet, test, proto-cleanliness,
  and producer/installer compatibility checks whenever `pluginsdk/**` changes;
  relevant backend/E2E jobs also include the new path.
- **Acceptance:** A `pluginsdk/v*` tag is validated as a Go submodule release,
  and the repository records the local multi-module workflow without making
  plugin repos depend on it.
- **Acceptance:** Public authoring docs, contract docs, root guidance, and the
  plugin skill use `github.com/kdlbs/kandev/pluginsdk` and its pack command.
- **Verification:** `cd pluginsdk && GOWORK=off go vet ./... && GOWORK=off go test ./...`
- **Verification:** run the repository workflow/path-filter validation available
  for changed Actions files, plus `git diff --check`.
- **Files likely touched:** `go.work`, `go.work.sum`, `.gitignore`, `.github/workflows/**`,
  `docs/public/plugins-authoring.md`, `docs/public/plugins-manifest.md`,
  `docs/plans/plugins/GRPC-CONTRACT.md`, `AGENTS.md`,
  `.agents/skills/create-kandev-plugin/SKILL.md`.
- **Dependencies:** Tasks 01–02.
- **Parallelism:** sequential; finalizes the initial Kandev PR and its public
  instructions.
- **Inputs:** ADR-2026-08-01, Go submodule tag rules, existing backend/E2E
  workflows, and the repository's release conventions.
- **Output contract:** list workflow triggers, docs updated, exact validation,
  external release steps still pending, risks, and status updates.

## Results

Added the initial Kandev-side SDK workflow and release-tag validation:

- `.github/workflows/pluginsdk.yml` checks SDK formatting, module tidy
  state, vet/tests, deterministic protobuf generation, and the standalone
  packer against the real backend installer on SDK/backend path changes.
- `.github/workflows/pluginsdk-release.yml` validates strict
  `pluginsdk/vX.Y.Z` tags and tests the nested module with `GOWORK=off`.
- Backend and E2E workflow path filters now include `pluginsdk/**`,
  `go.work`, and `go.work.sum`.
- The committed root `go.work` lists `apps/backend` and `pluginsdk`.
  Its workspace-only genproto replacements keep the current backend's
  pre-split Viper dependency and the SDK's split gRPC dependencies from
  producing ambiguous imports; published modules remain unaffected and are
  verified with `GOWORK=off`. `go.work.sum` records the workspace graph.
- Public authoring docs, the frozen gRPC contract, `AGENTS.md`, and the
  `create-kandev-plugin` skill now use the standalone SDK import, pinned
  module guidance, and `pluginsdk/cmd/plugin-pack`. The packer's PR-only
  `-version` override is documented.

Verification completed:

- `cd pluginsdk && GOWORK=off go vet ./... && GOWORK=off go test ./...` (pass)
- `cd pluginsdk && GOWORK=off go mod tidy && git diff --exit-code -- go.mod go.sum` (pass)
- `make -C pluginsdk proto` followed by matching SHA-256 hashes (pass)
- `cd apps/backend && go test ./...` with the committed workspace active
  (pass)
- YAML parsing of both new workflow files and `git diff --check` (pass)

The first `pluginsdk/v0.1.0` tag and its merge-commit pseudo-version remain
an explicit post-merge checkpoint in Task 04; no release tag was created here.
