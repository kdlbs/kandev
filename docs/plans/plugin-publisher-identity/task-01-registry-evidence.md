---
id: "01-registry-evidence"
title: "Produce registry publisher evidence"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PUBLISHER-001
acceptance_criteria:
  - AC-PLUGINS-PUBLISHER-001.1
  - AC-PLUGINS-PUBLISHER-001.2
  - AC-PLUGINS-PUBLISHER-001.3
system_design:
  - ../../specs/plugins/system-design/publisher-identity.md
---

# Task 01: Produce registry publisher evidence

## Summary

Produce archive-bound publisher evidence from trusted GitHub ownership and curation.
Validate native archives without executing their content.

## In scope

- Add the proposed non-extracting native validator and plugin-package inspector.
- Update registry schema/parser with reviewed official designation.
- Derive evidence from the release archive and GitHub repository/owner IDs.
- Reject reserved claims, repository transfers without pointer correction, mismatches, and incomplete ownership.
- Extend PR and scheduled validation without deploying an index during this work.

## Out of scope

Host attribution UI, runtime installation, and production plugin repository edits.

## Acceptance

- Acme-owned archives claiming Kandev fail publication, including later releases and normalized reserved-name variants.
- Only explicitly approved kdlbs-owned entries receive official evidence, and every emitted digest matches inspected bytes.
- Invalid archives never execute or extract contributor code, and downloads respect existing bounds.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test ./internal/plugins/pkgtar ./cmd/plugin-package)
node --test plugin-registry/build-index.test.mjs
git diff --check
```

The new command tests must exercise the real validator.
Builder tests must invoke the trusted inspector against a minimal native fixture, not only mock descriptor JSON.

## Files likely touched

- `plugin-registry/build-index.mjs`, `build-index.test.mjs`, `plugins.yaml`, `schema.json`.
- `.github/workflows/plugin-registry-index.yml`.
- `apps/backend/internal/plugins/pkgtar/pkgtar.go` and tests.
- Proposed `apps/backend/cmd/plugin-package/main.go` and tests.
- `apps/backend/Makefile` for the inspector build target.

## Dependencies

None. The design's evidence shape is the input contract.

## Risks

Do not use a tag manifest or first-tarball fallback as the authority.
Do not require the registry runner's platform when inspecting a multi-platform native package.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/plugins/requirements/publisher-identity.md).
- [System design](../../specs/plugins/system-design/publisher-identity.md).
- [Trust decision](../../decisions/2026-09-18-plugin-publisher-trust.md).

## Results

- RED: the new archive-inventory and required-executable tests failed before the validator enforced complete archive contents; the package inspector command was absent before implementation.
- `cd apps/backend && go test ./internal/plugins/pkgtar ./cmd/plugin-package -count=1`: passed.
- `node --test plugin-registry/build-index.test.mjs`: passed (17 tests).
- `git diff --check`: passed.
- The real-fixture builder test compiled and invoked the native `plugin-package` inspector and matched the emitted digest to the downloaded archive bytes.
