---
id: "02-retain-sdk-release"
title: "Retain and finalize signed SDK release evidence"
status: done
wave: 2
depends_on:
  - 01-reuse-immutable-host
plan: "plan.md"
requirements:
  - REQ-PLUGINS-SDK-DISTRIBUTION-001
  - REQ-PLUGINS-SDK-DISTRIBUTION-003
acceptance_criteria:
  - AC-PLUGINS-SDK-DISTRIBUTION-001.1
  - AC-PLUGINS-SDK-DISTRIBUTION-001.2
  - AC-PLUGINS-SDK-DISTRIBUTION-003.1
  - AC-PLUGINS-SDK-DISTRIBUTION-003.2
  - AC-PLUGINS-SDK-DISTRIBUTION-003.3
  - AC-PLUGINS-SDK-DISTRIBUTION-003.4
  - AC-PLUGINS-SDK-DISTRIBUTION-003.5
  - AC-PLUGINS-SDK-DISTRIBUTION-003.6
system_design:
  - ../../specs/plugins/system-design/sdk-distribution.md
---

# Task 02: Retain and Finalize Signed SDK Release Evidence

## Summary

Retain new SDK evidence on its own draft release, bind it to the original signed
source, then finalize it before npm publication. Recover `0.1.0` from the existing
`v0.99.0` source without changing either version or the receipt schema.

## In scope

- Extend `ReleaseAssets` to discover exact SDK drafts/releases with complete
  pagination, bind their release ID, retain assets without replacement, and
  finalize with `make_latest=false`. Resolve uncertain writes through reads.
- Make `CandidateStore` accept the verified evidence location associated with
  the complete source identity. Preserve all existing signing, rebuild, tool
  pin, inventory, digest, and missing-receipt guards.
- Reuse complete verified legacy host evidence. Detect conflicts between
  locations and reject unrelated partial sets before mutation.
- In `preparePublication`, retain all three assets before signed SDK tagging,
  then finalize and verify the SDK release before returning `prepared`.
- Require finalized evidence in the read-only GitHub/OIDC publish job.
  Bootstrap summaries and public verification must use the resolved asset URLs.
- Update the public SDK bootstrap and repair how-to. Keep the current trusted
  publisher identity, direct-publish/dist-tag permissions, and manual origin.

## Out of scope

- SDK code, API, package version, host floor, namespace, or metadata restamping.
- Runtime npm package arrays, new release channels, and new SDK workflows.
- First npm publication, token creation, or release dispatch during implementation.
- Replacing an asset, changing a signed tag, or weakening immutability.

## Acceptance

1. An immutable host with no SDK evidence recovers through a distinct SDK draft.
   Three verified assets precede signed SDK tagging and finalization.
   Application latest stays unchanged. npm publication succeeds or reports
   `bootstrap_required`.
2. Matching retries and lost responses reuse exact evidence. Conflicting tags,
   receipts, archives, release identities, incomplete published evidence, and
   unreadable remote states fail before unsafe mutation. Legacy reuse remains valid.
3. End-to-end adapter fixtures prove correct source/tool pins, canonical asset
   links, read-only publication permissions, manual-bootstrap identity, both
   public consumers, and unchanged Stable completion/mode exclusions.

## Verification

Run from the repository root. The first regression must model a host upload
returning HTTP 422. It must fail on the old storage behavior.

```bash
node --test scripts/release/sdk-assets.test.mjs scripts/release/sdk-candidate-store.test.mjs scripts/release/sdk-release-io.test.mjs scripts/release/publish-plugin-sdk.test.mjs scripts/release/sdk-artifact.test.mjs
node --test scripts/pluginsdk/consumer-smoke.test.mjs scripts/pluginsdk/pack-candidate.test.mjs
node --test scripts/release/sdk-source.test.mjs scripts/release/sdk-registry.test.mjs scripts/release/sdk-runtime.test.mjs
python3 .github/scripts/release-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
bash -n scripts/release/publish-plugin-sdk.sh
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
zizmor .github/workflows
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Add stateful draft create/finalize tests and retain real signed-source fixtures.
Prove zero host writes and no SDK asset replacement with positive controls.
Before live recovery, verify GitHub's draft/tag behavior using approved API
evidence or an explicitly authorized disposable probe. Never accept a draft
creation path that automatically exposes an unsigned SDK tag.

## Files likely touched

- `scripts/release/sdk-assets.mjs` and `.test.mjs`.
- `scripts/release/sdk-candidate-store.mjs` and `.test.mjs`.
- `scripts/release/sdk-publication-state.mjs` and `publish-plugin-sdk.test.mjs`.
- `scripts/release/sdk-release-io.mjs` and `.test.mjs`.
- `scripts/release/publish-plugin-sdk.mjs` and `sdk-artifact.mjs` for shared location handling.
- `.github/workflows/release.yml` and `.github/scripts/release-workflow-contract_test.py`.
- `docs/public/release-process.md` and `.agents/skills/release/SKILL.md`.

## Dependencies

[Task 01](task-01-reuse-immutable-host.md) keeps an immutable host from blocking
this SDK path during a new backfill.

## Risks

Draft discovery differs from published-release lookup. Pagination, ambiguous
identities, uncertain writes, and legacy partial evidence need explicit tests.
First-package npm setup remains a maintainer prerequisite after archive retention.
Do not infer successful publication from local or mocked consumer checks.

## Parallelism

`sequential`

## Inputs

- [SDK evidence design](../../specs/plugins/system-design/sdk-distribution.md#sdk-release-lifecycle-and-storage-selection).
- [Evidence ADR](../../decisions/2026-11-05-sdk-release-evidence.md).
- Existing SDK source, receipt, candidate, registry, and consumer fixtures.
- [Original publication work order](../plugin-sdk-publishing/task-04-stable-publication.md).

## Results

Implemented on 2026-11-05. The first storage regression failed on the original
HTTP 422 immutable-host upload. Separate ordering regressions failed before
finalization was added. Release-ID substitution and conflicting assets appearing
after draft creation also failed before their guards were added.

The adapter now discovers SDK drafts with full pagination, binds fresh reads and
writes to the release ID, and verifies every retained byte. Lost create, upload,
and publish responses trigger reads rather than duplicate writes. Complete
legacy evidence stays read-only. Signed tagging precedes SDK release finalization;
the read-only npm job requires finalized evidence. Bootstrap links use validated
GitHub asset URLs, including encoded SDK tags.

- Focused SDK publication, storage, candidate, adapter, and receipt suite:
  34 tests passed with Node 24.19.0 and npm 11.21.0.
- Packaging and consumer suite: 12 tests passed, including real isolated Go and
  frontend installations. Signed-source, registry, and runtime suite: 7 passed.
- Workflow contracts: 74 passed. Action-pinning tests: 9 passed. Shell syntax: passed.
- Public documentation validator: 63 tests passed; 55 pages validated.
- Harness validator: 19 tests passed; all 205 harness files passed lint.
- Specification validator: 36 tests passed. Catalog and full specification lint passed.
- Zizmor: the same 10 baseline findings, with no added or changed findings.
  The scanner's exit code 14 is recorded rather than reported as a clean scan.
- Whitespace checks passed. No live SDK draft, tag, npm upload, or release dispatch
  was created during implementation. Public availability remains unverified until
  the maintainer completes the documented recovery and bootstrap.

PR review added cross-phase release-ID binding and temporary draft URL handling.
Matching bytes cannot authorize a replacement release between retention and
finalization or read-only publication. Draft assets download through their IDs;
only finalized, validated URLs appear in bootstrap instructions. Ordering tests
require every event, and the unfinalized-evidence test covers first-package
bootstrap with OIDC readiness disabled.
