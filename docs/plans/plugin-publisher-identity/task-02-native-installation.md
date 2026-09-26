---
id: "02-native-installation"
title: "Bind native installs and updates to evidence"
status: done
wave: 2
depends_on:
  - "01-registry-evidence"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PUBLISHER-001
  - REQ-PLUGINS-PUBLISHER-002
  - REQ-PLUGINS-PUBLISHER-004
acceptance_criteria:
  - AC-PLUGINS-PUBLISHER-001.4
  - AC-PLUGINS-PUBLISHER-002.1
  - AC-PLUGINS-PUBLISHER-002.2
  - AC-PLUGINS-PUBLISHER-002.3
  - AC-PLUGINS-PUBLISHER-002.4
  - AC-PLUGINS-PUBLISHER-002.5
  - AC-PLUGINS-PUBLISHER-004.1
  - AC-PLUGINS-PUBLISHER-004.2
  - AC-PLUGINS-PUBLISHER-004.3
  - AC-PLUGINS-PUBLISHER-004.4
system_design:
  - ../../specs/plugins/system-design/publisher-identity.md
---

# Task 02: Bind native installs and updates to evidence

## Summary

Resolve catalog selections on the backend and bind verified attribution to installed bytes.
Use the same path for manual and automatic updates.

## In scope

- Add shared provenance types and canonical transport validation.
- Extend catalog DTOs with backend-owned projections and explicit source resolution.
- Add the catalog selector to InstallRequest and Service.InstallFromCatalog.
- Validate download digest and manifest identity before lifecycle mutation.
- Persist provenance with native records and guard publisher continuity under the lifecycle lock.
- Default uploads, URLs, sync, and old records to unverified.
- Add explicit same-version verification through `VerifyInstalledPublisher` and the admin-only verification endpoint.
- Compare the complete installed file inventory with a digest-verified reference archive, then persist attribution without lifecycle changes.

## Out of scope

Canvas transactions, frontend callers, and signing-key infrastructure.

## Acceptance

- Forged source metadata, URL overrides, redirects, and client publisher fields cannot establish verification.
- A mismatch or failed fresh resolution leaves existing runtime, record, preferences, and attribution unchanged.
- Restart preserves valid evidence. Existing-version verification changes only attribution and preserves original origin, without reinstall; a running process is briefly paused and restarted while installed files are compared.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -tags fts5 ./internal/plugins/provenance ./internal/plugins/marketplace ./internal/plugins/store)
(cd apps/backend && go test -tags fts5 ./internal/plugins -run 'TestPublisher|TestInstall|TestAutoUpdate|TestEligibleForAutoUpdate|TestSync')
git diff --check
```

Use existing service/handler fixtures and injected transport for canonical HTTPS tests.
Prove a late disable/uninstall/publisher replacement prevents stale automatic installation.
Add `publisher_install_test.go` with catalog success, digest mismatch, and
request-field rejection coverage. Add `publisher_existing_test.go` with
`TestPublisherExistingVersionVerification`.
Cover success/reload, missing historical version, modified files, and lifecycle
races. Existing installation and autoupdate suites cover continuity and update
eligibility.
Prove sibling runtime data is excluded, in-package data is included, and persistence failure publishes no verified projection.
Assert no install, reinstall, permission, or configuration mutation on success or failure. A running process is stopped for comparison and restarted with its active lifecycle state preserved.

## Files likely touched

- Proposed `apps/backend/internal/plugins/provenance/`.
- `apps/backend/internal/plugins/marketplace/types.go`, `catalog.go`, and tests.
- `apps/backend/internal/plugins/dto.go`, `handlers.go`, `service_install.go`, `autoupdate.go`.
- `apps/backend/internal/plugins/store/store.go`, `fs_store.go`, and tests.
- `apps/backend/internal/plugins/service_sync.go` and current install/update tests.
- Proposed `apps/backend/internal/plugins/publisher_existing.go` and `publisher_existing_test.go`.

## Dependencies

Task 01 evidence format and native archive validation.

## Risks

Do not let embedded catalog JSON populate host-owned projections.
Keep ordinary unverified installations backward compatible.
Preserve the current lifecycle and approval rollback behavior.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/plugins/requirements/publisher-identity.md).
- [System design](../../specs/plugins/system-design/publisher-identity.md).
- [Trust decision](../../decisions/2026-09-18-plugin-publisher-trust.md).

## Results

- RED: catalog installs initially trusted the client package URL and did not
  retain publisher provenance; the new selector and provenance tests failed
  before the backend-owned resolution path was added.
- `cd apps/backend && go test -tags fts5 ./internal/plugins/... ./internal/canvas ./internal/backendapp -count=1`: passed.
- Focused publisher tests cover canonical-source trust, catalog digest and
  selector validation, persisted unverified provenance, automatic-update
  continuity, and existing-version verification with reload and changed-file
  rejection.
- `make -C apps/backend build`: passed.
- `git diff --check`: passed.
- Review fixes: direct-install and legacy provenance URLs now strip userinfo,
  query, and fragment data on persistence and projection; automatic updates
  recheck the installation snapshot under the lifecycle lock; and existing
  version verification rejects same-size in-place mutations while leaving
  provenance unchanged.
- `go test -race -tags fts5 ./internal/plugins -run 'TestRunAutoUpdatePassRejectsMutationDuringDownload|TestCompareInstalledPackageRejectsSameSizeMutationAfterRead|TestInstallFromURLDoesNotPersistCredentialOrQueryData|TestFSStore_GetSanitizesLegacyPublisherURLs' -count=1`: passed.
