---
id: "04-curated-owner-attribution"
title: "Derive native catalog attribution from curation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-MARKETPLACE-001
  - REQ-PLUGINS-MARKETPLACE-002
acceptance_criteria:
  - AC-PLUGINS-MARKETPLACE-001.4
  - AC-PLUGINS-MARKETPLACE-001.5
  - AC-PLUGINS-MARKETPLACE-001.9
  - AC-PLUGINS-MARKETPLACE-002.3
system_design:
  - ../../specs/plugins/system-design/marketplace.md
  - ../../specs/plugins/system-design/native-plugin-attribution.md
---

# Task 04: Derive native catalog attribution from curation

## Summary

Correct native catalog attribution on fresh and retained records. Use the
reviewed repository owner while preserving package verification and publication
failure behavior. The [plan](plan.md#confirmed-diagnosis) records the reproduction.

## In scope

- `buildEntry`, `buildIndex`, and `trustedPriorRecord` behavior at the attribution boundary.
- Native entries with explicit `kind: plugin` or omitted kind.
- Deterministic registry tests and registry README attribution guidance.
- Results and status updates in this work order and the plan.

## Out of scope

- Canvas author semantics, package verification changes, and index schema changes.
- Backend services, frontend presentation, and external plugin releases.
- Publisher verification and live registry deployment.

## Acceptance

1. Fresh native records use the owner from `spec.repo`. Manifest names and
   missing, failed, or conflicting GitHub metadata cannot replace it.
   First-party owner `kdlbs` remains `kdlbs`. Canonical repository links and
   existing canvas attribution remain unchanged.
2. Eligible retained native records receive only an author correction.
   The prior input remains unchanged. Mixed builds advance healthy peers.
   All-failed builds still refuse publication, without counting retention as fresh.
3. Named regression tests fail for the attribution defect before the change
   and pass afterward. Registry documentation explains the owner/declaration
   distinction and the publication/cache delay.

## Implementation sequence

1. Read the active requirement and both designs.
2. Resolve current remote main and [PR 3793](https://github.com/kdlbs/kandev/pull/3793).
   If their contracts changed, reconcile the package before implementation.
3. Mark this work order `in_progress` and update the plan.
4. Add `buildEntry attributes native plugins to the curated repository owner`
   to `plugin-registry/build-index.test.mjs`. Use an `acme` or `Fclem` pointer
   with `author: kandev` and a deterministic package verifier.
5. Add `buildIndex replaces retained native attribution without changing release evidence`.
   Use both a failed retained entry and a healthy fresh peer in one build.
6. Run the focused RED command below before changing source.
7. Correct the native projection and eligible retention path.
8. Extend the owner cases and run the final verification block.
9. Record actual results before marking this order `done`.

The retained case must assert package version, package URL, digest, other
metadata, and the original prior object. Preserve tests for delisting,
ambiguous prior records, repository mismatches, and all-failed publication.

The owner cases include a normal declaration, a spoofed first-party name,
missing author, repository metadata HTTP failure, missing metadata owner, and
a conflicting metadata login. Keep the native manifest repository-claim
protection covered. Existing inspected-canvas author coverage must still pass.

## Verification

Run from the repository root. The RED command must fail for an owner mismatch:

```bash
node --test --test-name-pattern='buildEntry attributes native plugins|buildIndex replaces retained native attribution' plugin-registry/build-index.test.mjs
```

After the correction:

```bash
node --test plugin-registry/build-index.test.mjs plugin-registry/check-releases.test.mjs plugin-registry/workflow-contract.test.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the [coverage preflight](plan.md#verification-commands) and record its result.
The browser proof belongs to Task 05. This order does not require a live GitHub
release, real registry publication, or browser process.

## Files likely touched

- `plugin-registry/build-index.mjs`
- `plugin-registry/build-index.test.mjs`
- `plugin-registry/README.md`
- This work order and `plan.md`, for results and status only.

## Dependencies

None. This replaces the remaining host verification work from cancelled Tasks
01-03. Their external releases are not prerequisites.

## Risks

- The builder also handles canvases. An unconditional author replacement expands
  scope and violates their existing descriptor contract.
- Normalization of retained attribution must not bypass identity checks or make
  an all-retained rebuild publishable.
- PR 3793 overlaps this source and proposes different publisher fields.

## Parallelism

`sequential`

## Inputs

- [Marketplace requirements](../../specs/plugins/requirements/marketplace.md), criteria listed in frontmatter.
- [Native attribution design](../../specs/plugins/system-design/native-plugin-attribution.md), catalog projection and retained records.
- [Marketplace design](../../specs/plugins/system-design/marketplace.md), release propagation and failure behavior.
- [Central publication decision](../../decisions/2026-08-30-central-curated-plugin-release-polling.md).
- Existing `stubGitHub`, `verifiedPackage`, and `priorRecord` test helpers.
- Existing `buildEntry` canvas test and mixed-retention `buildIndex` tests.

## Results

Status: done.

The RED run used the unchanged pre-fix builder with the new regressions and
failed on the expected owner mismatch (`kandev` instead of `Fclem`) plus the
unchanged retained author. Both pass after the correction.

`buildEntry` now takes a native plugin's `author` from the curated repository
owner. Manifest author, missing author, metadata failure, missing metadata
owner, and a conflicting metadata login cannot replace it. Canvas entries keep
their original inspected-archive author chain: an archive author wins, then the
repository metadata owner, then the curated owner when metadata fails. Canvas
canonical repository validation is unchanged. Native records never read the
manifest or metadata author.

`buildIndex` copies an eligible retained native record and replaces only its
author. Package version, URL, digest, description, categories, stars, and update
time remain unchanged, and the caller's prior document is not mutated. Canvas
retention is unchanged. Retention still counts as no fresh release, so an
all-retained rebuild refuses publication.

Added coverage for the curated-owner projection, all owner edge cases, the
native manifest repository claim, retained attribution and evidence,
canvas retention, the canvas metadata-owner fallback for an empty inspected
author (metadata available and unavailable), unique prior matching with
repository mismatch, delisting, and all-failed publication refusal.

```text
node --test plugin-registry/build-index.test.mjs plugin-registry/check-releases.test.mjs plugin-registry/workflow-contract.test.mjs
tests 51, pass 51, fail 0
```

`plugin-registry/README.md` now documents curated-owner attribution, the
`Declared author` distinction, and the rebuild/cache/outage behavior. Catalog
and specification validation passed, including the coverage preflight.
