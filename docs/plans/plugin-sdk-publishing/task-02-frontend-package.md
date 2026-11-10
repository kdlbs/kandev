---
id: "02-frontend-package"
title: "Package standalone frontend SDK"
status: done
wave: 2
depends_on:
  - 01-go-module
plan: "plan.md"
requirements:
  - REQ-PLUGINS-SDK-DISTRIBUTION-001
  - REQ-PLUGINS-SDK-DISTRIBUTION-002
acceptance_criteria:
  - AC-PLUGINS-SDK-DISTRIBUTION-001.2
  - AC-PLUGINS-SDK-DISTRIBUTION-001.3
  - AC-PLUGINS-SDK-DISTRIBUTION-002.1
system_design:
  - ../../specs/plugins/system-design/sdk-distribution.md
---

# Task 02: Package standalone frontend SDK

## Summary

Prepare the existing npm package for real consumption outside the workspace.
Introduce one canonical release record with an automatically synchronized npm
copy, and prove the packed artifact's types resolve without the application.

## In scope

- Add `pluginsdk/sdk-release.json` with draft host values and shared SDK version
  `0.1.0`; generate/check the npm copy via new `scripts/pluginsdk/sync-record.mjs`.
- Include README, license, repository directory metadata, exported JSON record,
  and existing source exports in the npm artifact. Keep zero runtime/peer deps.
- Add packed-consumer tests for NodeNext/bundler resolution with an independent
  tsconfig; extend WO01's smoke script for a combined local consumer.
- Use temporary resolved records for consumer tests; do not assert that a draft
  source record is valid published compatibility evidence.

## Exact candidate contract

Add `scripts/pluginsdk/pack-config.json` with exact Node/npm versions and the
published-file allowlist. Select versions compatible with the planned OIDC
publish and dist-tag operations. Add `pack-candidate.mjs` and its tests. Emit
one archive path, file inventory, and SHA-512 integrity. Accept that path in the
packed-consumer test and verify its bytes before and after installation.

Include package metadata, source exports, release record, README, and license.
Reject application sources, tests, workspace files, and unexpected imports.
Do not repack inside the consumer test or publication command. Temporary
resolved records belong only to disposable test copies. WO04 adds the signed
source identity and retained release receipt around this packaging result.

For the combined local checker flow, serve this archive through a disposable
npm registry fixture. Preserve normal registry lock evidence and the strict
WO03 rejection of `file:` dependencies.

## Out of scope

New frontend contracts, runtime JavaScript, React dependencies, UI changes,
compatibility enforcement, or npm uploads.

## Acceptance

1. A packed artifact installs and typechecks outside the checkout without
   application tsconfig inheritance or private dependencies. Both resolution
   modes consume the same archive, whose integrity and allowlist are checked.
2. Go/npm release-record copies are consistent; package version equals the
   canonical shared SDK version and published files include the record/license.
3. The web host still resolves the workspace SDK types and the existing
   no-dependency/no-bare-import tests remain valid.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
node scripts/pluginsdk/sync-record.mjs --check
node --test scripts/pluginsdk/sync-record.test.mjs scripts/pluginsdk/pack-candidate.test.mjs
node --test scripts/pluginsdk/consumer-smoke.test.mjs
(cd apps && pnpm --filter @kdlbs/plugin-sdk test && pnpm --filter @kdlbs/plugin-sdk typecheck)
(cd apps/web && pnpm run typecheck)
node scripts/pluginsdk/consumer-smoke.mjs --mode local --skip-compatibility-check
git diff --check
```

Install workspace dependencies before package commands. The script and packed
test paths are new outputs of this work order; tarballs live in temporary paths.

## Files likely touched

- `pluginsdk/sdk-release.json` (new).
- `apps/packages/plugin-sdk/{package.json,sdk-release.json,README.md,LICENSE}`.
- `apps/packages/plugin-sdk/test/packed-consumer.node.mjs` (new).
- `scripts/pluginsdk/{sync-record.mjs,sync-record.test.mjs,consumer-smoke.mjs,consumer-smoke.test.mjs}`.
- `scripts/pluginsdk/{pack-config.json,pack-candidate.mjs,pack-candidate.test.mjs}` (new).
- `apps/pnpm-lock.yaml` if package metadata/dependency resolution requires it.

## Dependencies

[Task 01](task-01-go-module.md): canonical module location and local consumer harness.

## Risks

Workspace symlinks can hide missing tarball files. Source `.ts` exports must be
verified under both supported type-resolution modes. Draft records must not be
misrepresented as public release metadata.

## Parallelism

`sequential`

## Inputs

- [Design: Frontend package and release record](../../specs/plugins/system-design/sdk-distribution.md#frontend-package).
- Existing package manifest, standalone contract tests, and web SDK type bindings.

## Results

Implemented locally on 2026-10-19. The canonical draft record retains null host
values. The npm copy, license, repository metadata, exact tool pins, archive
allowlist, integrity checks, and independent registry consumer are implemented.
NodeNext and bundler consumers install the same archive. Helper tests reject
record drift, tool mismatch, unexpected files, application imports, runtime
dependencies, and modified archive bytes. The local Go/npm smoke passed.
Package contract tests, package typecheck, and web typecheck passed. The web
check required `NODE_OPTIONS=--max-old-space-size=8192` after the default heap
limit was exceeded. Implementation CI now passes, including the packed consumer and Docker builder.
CI exposed inherited `GOFLAGS` in the scoped npm-configuration test. Its fixture
now supplies clean Go inputs so the test reaches the npm boundary. Production
public-trust rejection remains unchanged.

Packaging uses Node 24.19.0 and npm 11.21.0. The npm pin supports OIDC publication
and OIDC dist-tag operations under the current
[trusted-publisher requirements](https://docs.npmjs.com/trusted-publishers/).
The package remains unpublished. Resolved host metadata and compatibility
checks are added by WO03; these tests do not certify a draft record for release.

## Completion

Completed on 2026-10-23 after local acceptance checks and implementation CI
passed. The [plan's cleanup record](plan.md#validation-and-review-cleanup)
contains the validation evidence and review dispositions. Public publication,
registry configuration, external repository migration, and author notices
remain separate follow-up work; completion here does not certify those actions.
