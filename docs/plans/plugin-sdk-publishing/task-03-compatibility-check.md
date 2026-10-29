---
id: "03-compatibility-check"
title: "Enforce consumer compatibility"
status: done
wave: 3
depends_on:
  - 02-frontend-package
plan: "plan.md"
requirements:
  - REQ-PLUGINS-SDK-DISTRIBUTION-002
acceptance_criteria:
  - AC-PLUGINS-SDK-DISTRIBUTION-002.1
  - AC-PLUGINS-SDK-DISTRIBUTION-002.2
  - AC-PLUGINS-SDK-DISTRIBUTION-002.3
  - AC-PLUGINS-SDK-DISTRIBUTION-002.4
system_design:
  - ../../specs/plugins/system-design/sdk-distribution.md
---

# Task 03: Enforce consumer compatibility

## Summary

Ship a read-only author CI command that validates actual resolved SDK versions
against the plugin manifest. Prove it fails on incompatible or untrustworthy
dependency evidence rather than reporting the verifier's own version.

## In scope

- Add `pluginsdk/cmd/kandev-sdk-check` and `pluginsdk/internal/compatibility`.
  Parse only the manifest minimum/API fields using existing YAML/SemVer libraries.
- Resolve the selected Go dependency with `GOWORK=off` and the installed frontend
  package when supplied; support frontend-only use and mixed SDK versions.
- Reject low/missing/malformed minima, unsupported API, draft/null records,
  version disagreement, SDK pseudo-versions, Go replacement directives, and
  workspace/file frontend resolutions. Check npm/pnpm lock evidence, allowing
  normal pnpm store symlinks; missing/unsupported lock evidence fails. Show
  precise failures and never mutate.
- Add manifest-API consistency tests against current legacy/current constants;
  extend isolated consumers to run positive and negative checker cases.

## Preserved lockfile policy

The review's optional lockfile simplification is not accepted. Keep npm/pnpm
registry-origin checks and explicit failures for missing or unsupported lock
formats. Preserve normal pnpm store symlinks. Local consumer fixtures use a
registry response serving the tested archive, not a `file:` exception in the
checker. Use the shared local/public environment contract from WO01 and WO04.

## Out of scope

Full manifest validation, runtime installation-policy changes, per-feature
compatibility declarations, auto-upgrades, and publication.

## Acceptance

1. The command checks the selected dependencies rather than its compiled SDK
   version, and enforces the maximum consumed baseline plus API-set membership.
2. Go-only, frontend-only, combined, and mixed-version consumers have meaningful
   success/failure cases, including missing/draft/local dependency evidence.
3. The checker does not change manifests or dependencies, the npm package stays
   runtime-free, and existing host API/capability gates remain authoritative.

## Verification

```bash
(cd pluginsdk && go test -trimpath -race ./... && go vet ./...)
(cd apps/backend && go test -trimpath ./internal/plugins/manifest)
node --test scripts/pluginsdk/consumer-smoke.test.mjs scripts/pluginsdk/sync-record.test.mjs
node --test scripts/pluginsdk/npm-workspace.test.mjs
node scripts/pluginsdk/consumer-smoke.mjs --mode local
node scripts/pluginsdk/sync-record.mjs --check
git diff --check
```

Local smoke uses a temporary resolved release record and a module archive.
Keep its mode distinct from the public-registry evidence produced by WO04.

## Files likely touched

- `pluginsdk/{go.mod,go.sum,internal/compatibility/**,cmd/kandev-sdk-check/**}`.
- `apps/backend/internal/plugins/manifest/sdk_release_test.go` (new).
- `scripts/pluginsdk/consumer-smoke{,.test}.mjs`.

## Dependencies

[Task 02](task-02-frontend-package.md): canonical/copied metadata and packed consumers.

## Risks

The checker must inspect selected versions after Go minimum-version selection,
not just requested versions. Missing metadata or local links cannot silently
be treated as supported. A manifest minimum must use a complete Stable version;
prerelease and build suffixes are rejected, as they are by the runtime host.

## Parallelism

`sequential`

## Inputs

- [Design: Consumer compatibility command](../../specs/plugins/system-design/sdk-distribution.md#consumer-compatibility-command).
- Backend manifest minimum/version helpers as behavior reference only; do not
  import them into the SDK module.

## Results

Implemented locally on 2026-10-19. The command resolves the selected Go module
with workspace mode disabled. It verifies installed npm metadata and npm
lockfile versions 2/3 or pnpm lockfile version 9. Normal pnpm store symlinks are
accepted. Missing, unsupported, local, draft, and conflicting evidence fails.

SDK race tests and vet passed. Manifest consistency tests passed. The full
isolated Go/npm consumer passed Go-only, frontend-only, combined, and low-floor
negative checks. A real Go minimum-version-selection fixture proves the checker
reads version 0.2.0 selected through a dependency instead of the direct 0.1.0
request. Checker runs leave the consumer go.mod unchanged.

Canonical host values remain null. Only disposable local archive copies resolve
them for these tests. CI now requires the checker in backend and combined
consumers. Implementation CI and review cleanup now pass.

## Completion

Completed on 2026-10-23 after local acceptance checks and implementation CI
passed. The [plan's cleanup record](plan.md#validation-and-review-cleanup)
contains the validation evidence and review dispositions. Public publication,
registry configuration, external repository migration, and author notices
remain separate follow-up work; completion here does not certify those actions.

## Adversarial review correction

On 2026-10-24, the [implementation review](adversarial-review.md) found that the
checker accepted prerelease/build host minima and rejected valid hoisted npm
workspace installations. Both were reproduced before fixing. Stable-minimum
tests and the real npm workspace/CLI regression now pass, including shadow,
symlink, version, and lock-root rejection cases. The strict lock policy remains
required. SDK race tests and vet passed after the fixes.


## Workspace file and path review corrections

Review of the ready-for-review merge found false failures for lockfiles above
1 MiB and checkout-directory symlinks. Both npm and pnpm regressions failed
before the fix. Separate lockfile limits and canonical checkout-root comparisons
now preserve published dependency checks. SDK race tests, vet, and the real npm
workspace integration passed. See the [plan's correction record](plan.md#workspace-file-and-path-review-corrections)
for boundary coverage and the pending delivery checks.


The final alias regression also covers workspace members whose lockfile lives
in a real parent directory. The checker resolves the consumer directory before
lock discovery. npm/pnpm regressions and real npm CLI checks through the member
alias pass; local SDK substitutions and shadow-path bypasses remain rejected.
