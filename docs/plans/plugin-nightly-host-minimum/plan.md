---
created: 2026-10-14
status: done
requirements:
  - REQ-PLUGINS-PLUGINS-001
system_design:
  - ../../specs/plugins/system-design/plugins-01.md
legacy_specs: []
---

# Implementation Plan: Plugin minimum host version on npm Nightlies

## Overview

Enforce the existing plugin manifest minimum on published npm Nightly hosts.
One sequential work order adds meaningful install and shared-checker regression
coverage, corrects running-version recognition, and clarifies public minimum
policy. The reviewed package was implemented after the later explicit request.

## Evidence and root cause

Qualified baseline: `107cef5d4d1bf780f1880820baf661fe0e8c6170`.
The supervisor's real `plugins.Service.Install` reproduction on
`v1.2.4-nightly.sha123456789abc` accepted a valid package requiring `2.0.0`:
extraction, filesystem persistence, registry registration, and fake-runtime
`Start`/active status all occurred. Its Stable `v1.2.4` control rejected with
none of those residual effects. Earlier-minimum Nightly, dev, and git-describe
controls passed. Qualification reported one causal RED and four passing
controls under Go 1.26 with race, trimpath, fts5, one build worker, and bounded
resources. Evidence was supplied in `qualified-proof.json` and `proof.log`
under `/tmp/kandev-root-nightly-plugin-discovery-20261014-ktjdm48p/`.
The supervisor's private reproduction source is not an implementation input.

`manifest.CheckMinimumKandevVersion` uses the stable-only
`NormalizeReleaseVersion` for the running host, treats the published Nightly
suffix as unrecognized, and returns success before comparing the minimum.
`scripts/release/nightly-release.sh` emits the real tag, `release.yml` stamps it
into `main.Version`, and `backendapp/services.go` supplies it to both consumers.
Those producers require no changes.

## Scope

### In scope

- `AC-PLUGINS-PLUGINS-001.14` in the existing owning requirement/design pair.
- Running-version recognition in `internal/plugins/manifest/semver.go`.
- Real Install rejection and success side effects, public shared-checker
  compatibility controls, and the direct canvas compatibility seam.
- Concise policy clarification in `docs/public/plugins-manifest.md` (reference)
  and the existing floor paragraph in `docs/public/plugins-authoring.md`
  (authoring guide). Public prose ships with the implementation, not this draft.

### Out of scope

Production or permanent test changes during this design turn; prerelease
manifest minimums; stable parser or marketplace comparator changes; a new
SemVer library or broad prerelease grammar; release generation/stamping/workflows;
capability floors, feature flags, plugin protocol/runtime behavior, canvas
operations/authorization, and UI/copy/layout changes. No frontend, browser,
typecheck, or whole-backend local suite is required.

## Technical approach

The owning [host release compatibility design](../../specs/plugins/system-design/plugins-01.md#host-release-compatibility)
defines the narrow running-version extension. Preserve the stable parser and
the existing comparator; keep the Nightly suffix for comparison. The real
installer already places the gate before persistence, registration, and
activation, with fresh-directory cleanup on rejection.

| Boundary | Identity/version shape | Intended behavior | Evidence |
| --- | --- | --- | --- |
| Plugin Install | Published tagged Nightly; stable minimum | Reject future and same-core Stable minima; accept earlier minima | New real Install test, FSStore/registry/runtime assertions |
| Shared manifest checker | Tagged or unprefixed published Nightly | SemVer prerelease precedence; invalid minimum still rejected for recognized host | Public checker table |
| Existing Stable path | Existing accepted numeric/leading-v forms | Preserve acceptance, rejection, and invalid-minimum behavior | Existing and new checker/install controls |
| Development fallback | Empty, dev, git-describe, unknown/malformed host | Preserve existing comparison exemption and independent validation | Checker and existing install controls |
| Canvas distribution compatibility | Same shared checker via `SetKandevVersion` | Preserve wrapped `ErrInstallIncompatible` for rejection | Direct seam test plus existing preparation rejection test |

No new ADR is needed: the existing npm Nightly decision fixes the version
shape, and the owning design records the local recognition boundary.
`AC-PLUGINS-TASK-DEPS-001.16` supplies existing decision-impact context for
older-host rejection without partial registration; its document and migration
status are unchanged. No task-dependency implementation is included.

## Tests

All new tests map to `AC-PLUGINS-PLUGINS-001.14`.

- `apps/backend/internal/plugins/service_nightly_install_test.go`:
  `TestServiceInstall_NightlyMinimumHostVersion`, using
  `newTestServiceWithDir`, `minKandevVersionPackage`, the real FSStore and
  Registry, and `fakeRuntime.startCallCount`/`Running`. Expected pre-fix RED:
  future and same-core Stable minimums on the actual tagged Nightly are
  accepted with install/activation effects. Success controls must reach
  active status and prove persistence, registration, and Start.
- `apps/backend/internal/plugins/manifest/semver_test.go`:
  `TestCheckMinimumKandevVersion` covers the published tagged/bare shape,
  same-core/future rejection, earlier minimum acceptance, Stable/empty/dev/
  git-describe/unknown/malformed controls, and unchanged invalid minimums.
  `TestNormalizeReleaseVersion` explicitly keeps Nightly/prerelease minimums
  outside stable grammar. Existing comparator and capability-floor tests run
  with the affected manifest package.
- `apps/backend/internal/canvas/distribution_compatibility_test.go`:
  `TestDistributionInstallCompatibility` calls the existing direct seam with
  `webapp.Package`/`manifest.Manifest` and `SetKandevVersion`. Assert typed
  incompatibility for future/same-core minimums and acceptance for older
  minimums; preserve Stable/dev and nil-package/nil-manifest controls.
  Retain the existing `TestPrepareInstallRejectsPackageRequiringNewerKandev`.

The real Install boundary is the end-to-end evidence for this backend fix;
no artificial browser test is added. Detailed commands and control values are
in the work order.

## Work orders

- [x] [Task 01: Enforce Nightly host minimums](task-01-enforce-nightly-host-minimum.md)

Dependency order: one sequential task, no predecessor or delegated work.

## Verification results

Implementation completed. Real Install RED returned exit 1 for the future and
same-core Stable minimums, proving extraction, persistence, registration, and
activation were reached; all three controls passed. After the running-only
correction, affected plugin packages passed with race/trimpath/fts5/p1
(`internal/plugins`: 11.166s; `internal/plugins/manifest`: 1.148s), and new/existing
canvas compatibility tests passed (1.044s). Each original process was joined.
Canonical affected-complete-package Go lint passed with zero issues.
Design catalog: passed (381 decisions, 1596 specifications). All-spec lint and
diff checks: passed. Initial documentation-coverage invocation exited 127
because `node` was absent from PATH. The single corrected invocation with the
existing Node 24.18.0 binary passed: the actual four-document design diff is
exempt, and the prospective ten-path implementation is covered by Task 01
with its owning requirement/design references; both returned empty errors.
Public documentation validation passed for all 48 published pages. Actual
staged coverage and closing diff checks are recorded in Task 01. Commit, push,
and exact-head hosted CI/review evidence remain external delivery records.

## Risks

- Dropping the Nightly suffix would incorrectly admit a same-core Stable floor.
- Widening the stable parser would admit prerelease manifest minimums or change
  capability-floor validation. Extend only running-host recognition.
- Parsing arbitrary development suffixes would remove existing exemptions.
- A helper-only test could miss installer persistence or activation; the real
  Install regression must assert all four observable effect boundaries.
