---
id: "01-enforce-nightly-host-minimum"
title: "Enforce Nightly host minimums"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PLUGINS-001
acceptance_criteria:
  - AC-PLUGINS-PLUGINS-001.14
system_design:
  - ../../specs/plugins/system-design/plugins-01.md
---

# Task 01: Enforce Nightly host minimums

## Summary

Recognize the published npm Nightly running-version form in the shared minimum
checker. Prove the real plugin installer rejects incompatible packages without
residual install effects and that the canvas compatibility seam preserves its
typed error contract.

## In scope

- Tests first at real Install, public checker, and direct canvas seam boundaries.
- Minimal running-version recognition change in `manifest/semver.go`.
- Public minimum-policy clarification in the existing manifest reference and
  authoring floor paragraph, assessed through `/docs-maintainer`.
- Accurate plan/work-order results after authorized implementation.

## Out of scope

All exclusions in [the plan](plan.md#scope) apply. Do not change the Stable
minimum grammar, marketplace `CompareVersions`, release producers, protocol,
capability floors, runtime lifecycle, canvas operations, or UI. Do not use
the supervisor's private test source; author regression coverage independently
from the existing repository helpers and supplied behavior evidence.

## Acceptance

1. The independently authored real Install regression fails causally before
   the correction and passes afterward. For running
   `v1.2.4-nightly.sha123456789abc`, minima `2.0.0` and `1.2.4` reject with nil
   record, absent fresh version directory, absent FSStore record and registry
   entry, zero runtime Start calls, and inactive runtime. Minimum `1.2.3`
   succeeds with a persisted/registered active record and one Start call.
   Stable `v1.2.4` rejects `2.0.0` and accepts `1.2.4`.
2. Public checker and direct canvas controls pass with existing behavior
   preserved. Checker controls include empty minimum/host, case-insensitive dev,
   git-describe, unknown/malformed host, stable shortened/leading-v forms, and
   invalid manifest minimums on recognized Stable and Nightly hosts. Include
   Nightly hashes containing letters and hashes containing only digits. Reject
   prerelease minimums on recognized hosts; preserve existing early-return
   behavior on exempt hosts. Malformed running controls include short/long or
   nonhex/uppercase hashes, noncanonical or missing core segments, generic
   prerelease, and appended git-describe/build-metadata suffixes, which retain
   the exemption. `NormalizeReleaseVersion` still rejects Nightly values.
   Canvas tests use `errors.Is(err, ErrInstallIncompatible)` and preserve
   nil-package/nil-manifest `ErrInstallInvalid` and development controls.
3. Only the narrow running path changes. Required checks pass and public docs
   explain that a published Nightly enforces Stable minima but cannot satisfy
   its same-core Stable release, while development exemptions and manifest
   grammar remain as documented. Draft specification statuses stay unchanged.

## Verification

Run from the repository root after the later explicit implementation request.
Serialize local heavy commands, retain every original process/session handle,
and join each before the next check. No heavy command runs during design.

Write permanent tests first, then run the causal RED command before production
changes (expected failure from actual incompatible installation):

```bash
(cd apps/backend && env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 280s /home/jcfs/.local/share/mise/installs/go/1.26.0/bin/go test -timeout=2m -race -trimpath -tags=fts5 -p=1 -count=1 -run '^TestServiceInstall_NightlyMinimumHostVersion$' ./internal/plugins)
```

Implement the correction, then run GREEN for the affected plugin packages and
only the canvas compatibility boundaries:

```bash
(cd apps/backend && env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 280s /home/jcfs/.local/share/mise/installs/go/1.26.0/bin/go test -timeout=2m -race -trimpath -tags=fts5 -p=1 -count=1 ./internal/plugins ./internal/plugins/manifest)
(cd apps/backend && env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 280s /home/jcfs/.local/share/mise/installs/go/1.26.0/bin/go test -timeout=2m -race -trimpath -tags=fts5 -p=1 -count=1 -run '^(TestDistributionInstallCompatibility|TestPrepareInstallRejectsPackageRequiringNewerKandev)$' ./internal/canvas)
env PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH" GOTOOLCHAIN=local timeout --kill-after=10s 360s bash scripts/lint-go-changed apps/backend/internal/plugins/manifest/semver.go apps/backend/internal/plugins/manifest/semver_test.go apps/backend/internal/plugins/service_nightly_install_test.go apps/backend/internal/canvas/distribution_compatibility_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
env PATH="/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH" node scripts/validate-public-docs.mjs
git diff --check
git status --short -- docs/plans/plugin-nightly-host-minimum
```

The lint helper analyzes complete affected packages and owns its resource
settings; hosted CI owns full-backend lint. If owned Go paths change, pass every
actual changed Go path to the helper.
Run documentation coverage via `.github/scripts/pr-docs.cjs`'s `validateCoverage`
on actual changed paths and actual linked document contents, including untracked
files. At design time also check the explicit prospective production/test/doc
path set below without claiming those files were changed or tested. Require
`ok: true`, covered status when production paths are included, and no errors.
No unchanged validator unit suites are needed for the prose change.

### Delivery prerequisites after implementation

Use the existing Go and Node directories on PATH for ordinary hooks; do not
install a toolchain. If normal-hook dependencies are missing, perform exactly
one frozen workspace install before hooks, under the same serialized local
resource lease. Skip installation when dependencies already exist. Do not run
these commands during design:

```bash
(cd apps && env PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH" timeout --kill-after=10s 600s /home/jcfs/.nvm/versions/node/v24.18.0/bin/corepack pnpm install --frozen-lockfile)
```

After required checks pass, stage only owned files and use ordinary hooks with
no bypass. Bounded delivery commands are:

```bash
env PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH" GOTOOLCHAIN=local timeout --kill-after=10s 600s git commit -m "fix(plugins): enforce minimum host version on nightly builds"
env PATH="/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH" timeout --kill-after=10s 300s git push --set-upstream origin HEAD
```

## Files likely touched

- `apps/backend/internal/plugins/manifest/semver.go`
- `apps/backend/internal/plugins/manifest/semver_test.go`
- `apps/backend/internal/plugins/service_nightly_install_test.go` (new)
- `apps/backend/internal/canvas/distribution_compatibility_test.go` (new)
- `docs/public/plugins-manifest.md`
- `docs/public/plugins-authoring.md`
- `docs/specs/plugins/requirements/plugins.md`
- `docs/specs/plugins/system-design/plugins-01.md`
- `docs/plans/plugin-nightly-host-minimum/plan.md`
- `docs/plans/plugin-nightly-host-minimum/task-01-enforce-nightly-host-minimum.md`

Existing `service_install.go`, `backendapp/services.go`, and
`canvas/distribution_install.go` are consumer evidence, not planned production
edits. Existing package test helpers are reused without modifying them.

## Dependencies

None. Design package review and a later explicit implementation request precede
execution.

## Risks

See [the plan](plan.md#risks). New real Install tests belong in their own file
because existing service test files are large. Test observable behavior at the
public checker, not a private recognition helper or regex implementation.

## Parallelism

`sequential`

## Inputs

- [Owning requirement](../../specs/plugins/requirements/plugins.md),
  `AC-PLUGINS-PLUGINS-001.14`.
- [Owning design](../../specs/plugins/system-design/plugins-01.md#host-release-compatibility).
- [Existing floor decision-impact criterion](../../specs/plugins/requirements/task-dependency-projection.md),
  `AC-PLUGINS-TASK-DEPS-001.16` (reference only; no status promotion or edits).
- Existing `service_install_test.go` package builder and
  `service_test.go` service/FSStore/registry/runtime helpers.
- Existing `manifest/semver_test.go` and
  `canvas/distribution_export_test.go` compatibility control.

## Results

Implementation completed after the reviewed design checkpoint and later
explicit implementation request. Production and permanent tests were untouched
during design.

- Real Install RED command: exit 1, causal failures for future and same-core
  Stable minima on the published tagged Nightly. Both showed persisted,
  registered, extracted, active packages and one Start call; three controls
  passed. Original test process joined before production edits.
- Plugin/manifest GREEN command: exit 0, 11.166s and 1.148s respectively.
  Includes new real Install and public checker controls, existing Stable/dev/
  unwired install behavior, unchanged comparator and capability-floor coverage.
- Bounded canvas GREEN command: exit 0, 1.044s. New typed direct-seam tests and
  existing preparation rejection test passed.
- Canonical affected-complete-package Go lint: exit 0, zero issues. Original
  test and lint process handles were joined before subsequent heavy work.
- Catalog and all-spec lint: passed (381 decisions, 1596 specifications).
- Public-doc validator: passed, 48 published pages. Manifest reference and
  authoring guide clarify the published Nightly minimum policy.
- Actual staged documentation coverage: passed for all ten owned paths,
  `status: covered`, owning requirement/design accepted, and empty errors.
  Closing diff/gofmt checks passed. Commitlint was absent, so the single frozen
  apps install completed successfully with pnpm 9.15.9 (937 packages reused,
  zero downloaded). Both normal Git hooks are active. Commit/push and hosted
  CI/review state are external delivery records.

Design checks passed: catalog (381 decisions, 1596 specifications), all-spec
lint, and diff checks. Initial coverage invocation exited 127 because `node`
was absent from PATH; the single corrected invocation used the existing
Node 24.18.0 binary and passed: actual four-document diff exempt, prospective
ten-path implementation covered, both with empty errors.
