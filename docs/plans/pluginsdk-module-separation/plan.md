---
spec: docs/specs/plugins/spec.md
decision: docs/decisions/2026-08-01-standalone-plugin-sdk-module.md
created: 2026-08-01
status: draft
---

# Implementation Plan: Standalone Plugin SDK Module

## Overview

Extract Kandev's public plugin-authoring contract into the nested Go module
`github.com/kdlbs/kandev/pluginsdk`, including protocol stubs, manifest
validation, and package production. Merge and publish that module before
changing consumers, then migrate the Kandev backend, the official plugin
template, and each catalog plugin to an immutable SDK version and installable
PR artifacts. The staged order avoids requiring a module version that cannot
exist until its source is merged.

---

## Module Boundary

### Public SDK and protocol

- Add `pluginsdk/go.mod` and `pluginsdk/go.sum` with only the dependencies used
  by the authoring surface.
- Move/copy the current `apps/backend/pkg/pluginsdk` implementation and tests to
  the module root.
- Move the `kandev.plugin.v1` proto source and generated Go files under
  `pluginsdk/proto/kandev/plugin/v1`; change `go_package` and SDK imports to
  `github.com/kdlbs/kandev/pluginsdk/proto/kandev/plugin/v1`.
- Give the module its own reproducible proto-generation target and committed
  generated-output check.

### Manifest and package production

- Move the public manifest model and validators from
  `apps/backend/internal/plugins/manifest` to `pluginsdk/manifest` so authors and
  the host use one contract.
- Extract the archive writer from the production use of
  `internal/plugins/pkgtar/pkgtartest` into `pluginsdk/pluginpack`; retain the
  backend installer and its defensive extraction/verification under
  `apps/backend/internal/plugins/pkgtar`.
- Move `cmd/plugin-pack` to `pluginsdk/cmd/plugin-pack` and add a package-version
  override that changes only the staged/archive manifest. This lets PR CI make
  unique installable versions without editing committed release metadata.

### Kandev consumption

- After the initial SDK merge and version checkpoint, update backend imports to
  `github.com/kdlbs/kandev/pluginsdk`, its `manifest` package, and its generated
  proto package.
- Pin `apps/backend/go.mod` to the first published SDK tag; commit a root
  `go.work` so source builds exercise the colocated module while `GOWORK=off`
  proves the published dependency is complete.
- Remove the superseded backend SDK/proto/manifest/packer copies only after all
  backend, fixture, package, and installer compatibility tests pass.

---

## CI and Release Workflow

- Add an SDK workflow covering tidy state, formatting, vet, unit tests,
  generated-proto cleanliness, and package-producer/host-installer compatibility.
- Expand Kandev backend and E2E workflow path filters so `pluginsdk/**` and
  `go.work` changes trigger relevant jobs.
- Add a tag validation workflow for `pluginsdk/v*`; Go submodule releases need
  the `pluginsdk/` tag prefix.
- Treat tag creation as an explicit post-merge checkpoint. Record the merge SHA,
  semantic tag, and pseudo-version before consumer PRs start.

---

## Plugin Repository Migrations

The initial migration scope is the official template and every repository in
`plugin-registry/plugins.yaml`: `kandev-plugin-session-cost`,
`kandev-plugin-provider-usage`, and `kandev-plugin-kandy`.

For each repository:

- Replace `github.com/kandev/kandev/pkg/pluginsdk` imports with
  `github.com/kdlbs/kandev/pluginsdk`.
- Remove the local Kandev `replace` and pin the exact SDK merge commit as the Go
  pseudo-version requested for the rollout.
- Change packaging to
  `github.com/kdlbs/kandev/pluginsdk/cmd/plugin-pack` and support an explicit PR
  package version without changing committed release metadata.
- Keep normal release packages named `<id>-<manifest-version>.tar.gz`.
- On `pull_request`, run verification, build the all-platform package, inspect
  its manifest/checksum/platform contents, and upload the `.tar.gz` as a
  short-retention GitHub Actions artifact named with PR number and commit.
- Use `pull_request`, `contents: read`, no repository secrets, and no automatic
  installation of PR code. Document downloading the artifact and manually
  uploading the inner `.tar.gz` to a disposable Kandev instance.

---

## Documentation

- Update `docs/public/plugins-authoring.md`, plugin contract references, root
  `AGENTS.md`, and `.agents/skills/create-kandev-plugin/SKILL.md` to use the new
  module and packaging command.
- Update `kdlbs/kandev-plugin-template` documentation and workflows so new
  plugins inherit commit/tag pinning and PR artifact publication.
- Document that GitHub Actions artifact URLs are authenticated/wrapped and are
  intended for download plus file upload, not Kandev's unauthenticated
  install-by-URL endpoint.

---

## Tests

- **What:** SDK Go API and proto conversions remain behaviorally equivalent.
  **File:** moved `pluginsdk/*_test.go` plus generated-code checks.
  **How:** `cd pluginsdk && go test -race ./...` and regenerate proto with a
  clean diff.
- **What:** Public manifest validation accepts/rejects the same fixtures as the
  current host implementation. **File:** `pluginsdk/manifest/*_test.go`.
  **How:** table-driven validation tests migrated intact before deleting the
  backend package.
- **What:** New `plugin-pack` output, including a PR version override, installs
  through the real backend verifier. **File:** SDK packer tests and
  `apps/backend/internal/plugins/pkgtar/*_test.go` compatibility fixture.
  **How:** produce a tarball with the SDK command and pass it to `pkgtar.Install`.
- **What:** Kandev builds both in workspace mode and from the pinned SDK module.
  **File:** CI workflow and backend module metadata.
  **How:** targeted backend plugin tests normally, then with `GOWORK=off` after
  the SDK version exists.
- **What:** Every migrated plugin builds without a sibling Kandev checkout and
  emits a verified all-platform PR artifact. **File:** each plugin's Go tests,
  Makefile tests where present, and `.github/workflows/build.yml`.
  **How:** run repository verification and packaging with `GOWORK=off`, inspect
  the archive, and confirm Actions artifact upload on the PR.

No UI behavior changes; no new Playwright scenario is required. Existing plugin
install E2E coverage remains the host-level regression gate.

---

## Verification Results

Pending. Each task records exact commands, counts, produced artifact paths, and
the SDK tag/pseudo-version once available.

---

## Implementation Waves And Parallel Candidates

Wave 1 — initial Kandev SDK module PR, sequential:

- [x] [task-01-sdk-protocol-module](task-01-sdk-protocol-module.md)
- [x] [task-02-manifest-and-packager](task-02-manifest-and-packager.md)
- [x] [task-03-ci-release-and-docs](task-03-ci-release-and-docs.md)

Merge checkpoint:

- [ ] [task-04-publish-sdk-version](task-04-publish-sdk-version.md)

Wave 2 — after the immutable SDK version exists. Tasks 05–09 are independent
repository changes and are parallel candidates only with explicit user
authorization:

- [ ] [task-05-migrate-kandev-backend](task-05-migrate-kandev-backend.md)
- [ ] [task-06-migrate-plugin-template](task-06-migrate-plugin-template.md)
- [ ] [task-07-migrate-session-cost](task-07-migrate-session-cost.md)
- [ ] [task-08-migrate-provider-usage](task-08-migrate-provider-usage.md)
- [ ] [task-09-migrate-kandy](task-09-migrate-kandy.md)

Wave 3 — release evidence:

- Verify the merged plugin workflows expose downloadable installable artifacts
  on representative PRs and record links/results in this plan.
