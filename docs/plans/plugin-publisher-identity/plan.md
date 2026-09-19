---
created: 2026-09-18
status: complete
requirements:
  - REQ-PLUGINS-PUBLISHER-001
  - REQ-PLUGINS-PUBLISHER-002
  - REQ-PLUGINS-PUBLISHER-003
  - REQ-PLUGINS-PUBLISHER-004
system_design:
  - ../../specs/plugins/system-design/publisher-identity.md
legacy_specs: []
---

# Implementation plan: Plugin publisher identity

## Overview

Separate declared author credit from publisher identity established by the official registry.
Bind that evidence to installed package bytes and show its meaning before installation.
The user accepted this direction after the impersonation investigation.
Tasks 01 through 04 are implemented and verified. The plan records the
resulting trust boundary and the remaining full-suite baseline limitation.

The Plugins system owns the
[requirements](../../specs/plugins/requirements/publisher-identity.md) and
[design](../../specs/plugins/system-design/publisher-identity.md).
The [trust decision](../../decisions/2026-09-18-plugin-publisher-trust.md) records the chosen trust root.

## Scope

### In scope

- Official registry ownership checks, archive inspection, and reserved attribution.
- Host-derived catalog projections and digest-bound native installation/update evidence.
- Canvas review, receipts, active-release attribution, and export boundaries.
- Desktop/phone attribution, translations, errors, and public documentation.
- Compatible unverified behavior for legacy, direct, and custom-source installations.
- Explicit verification of an existing native version without update or reinstall; a running process is briefly paused and restarted while its files are compared.

### Out of scope

- Mandatory signatures, enterprise roots, revocation infrastructure, and malware checks.
- Production plugin changes, external release publication, and retrospective abuse investigation.
- General brand/name/icon similarity detection and permission-policy changes.

## Technical approach

1. Add bounded native archive inspection and registry evidence generation. Keep output additive and defer live index publication until the remaining steps pass.
2. Add `plugins/provenance`, catalog resolution, native selectors, persisted evidence, and guarded automatic updates. Add explicit file comparison for existing-version verification.
3. Reuse that evidence in canvas preparations and receipt transactions. Tie visible evidence to the active imported release.
4. Update shared attribution UI and selector callers. Add desktop/phone evidence and explain the new labels in public docs.

The technical design defines exact evidence fields, endpoint payloads, error codes, legacy behavior, and the canonical-URL trust rule.
Tests must reject an `official` source label or URL override as independent proof.
No package data can deserialize into the trusted host projection.

## ASCII UI preview

### UI-01: Marketplace attribution and actions

Entry: Settings > Plugins > Browse. State: verified community release.

```text
Desktop
+----------------------------------------------------------------+
| Example plugin  v1.2.3                                [Install] |
| Publisher: acme  [Verified publisher]                           |
| Source: Kandev Official   Repository: acme/example              |
| Declared author: Example contributors                           |
+----------------------------------------------------------------+

Phone
+------------------------------------+
| Example plugin  v1.2.3              |
| Publisher: acme                    |
| [Verified publisher]               |
| Source: Kandev Official             |
| Repository: acme/example           |
| Declared author:                   |
| Example contributors               |
| [Install]                          |
+------------------------------------+
```

The source label never supplies the publisher badge.
The card belongs to the existing scrolling list. It adds no fixed region or overlay.
Phone values wrap. Actions occupy a separate row and retain touch-sized targets.
These are structural requirements under AC-PLUGINS-PUBLISHER-003.1 and 003.3. Spacing and example names are illustrative.

### UI-02: Unverified and official states

Entry: Browse, Installed, plugin details, or canvas package review.
The same semantic order applies on desktop and phone.

```text
Unverified upload                  Verified first-party release
Unverified publisher               Publisher: kdlbs
Source: Uploaded file              [Official Kandev]
Declared author: kandev             Source: Kandev Official
                                   Declared author: kandev
```

No badge appears beside declared credit. Integrity and signature facts occupy separate labeled rows.
The full verification explanation stays visible in details/review, without hover.
Existing canvas review keeps its permission groups and confirmation action.
This covers AC-PLUGINS-PUBLISHER-003.1 through 003.4.

### UI-03: Update failure and publisher change

Entry: Installed plugin with a candidate update. State: verification failed or publisher differs.

```text
Installed v1.2.3: acme [Verified publisher]
Update v1.3.0: new-owner [Verified publisher]
This update changes the publisher from acme to new-owner.
[Update]

On verification failure:
The package does not match the selected release. Refresh and retry.
Installed v1.2.3: acme [Verified publisher]
[Check for updates]
```

Phone uses stacked text and a separate action row as in UI-01.
No optimistic badge appears while downloading. Installed attribution remains tied to the installed version.
An automatic publisher change fails and directs the user to manual review.
Source outages preserve installed attribution and existing management controls.
This covers AC-PLUGINS-PUBLISHER-002.3, 002.5, and 003.5.

### UI-04: Verify an installed version

Entry: native plugin details. State: an unverified installation viewed by an administrator.

```text
Desktop
Unverified publisher  [Verify publisher]
Source: Uploaded file

Phone
Unverified publisher
Source: Uploaded file
[Verify publisher]

Progress (both)
Verifying installed files...  [Verify publisher: disabled]

Success (both)
Publisher: acme [Verified publisher]
Source: Uploaded file
Matched against: Kandev Official

Unavailable version (both)
No trusted package is available for this installed version.
[Retry verification]

Mismatch (both)
Installed files differ from the published package.
The plugin was not changed.
[Retry verification]
```

This adds no overlay or scroll owner. Phone actions occupy their own touch-sized row.
Keep the version and runtime status unchanged throughout the flow.
The action is absent for read-only members. Labels and status announcements are localized.
These structural requirements cover AC-PLUGINS-PUBLISHER-004.1 through 004.5.

## Tests

Coverage is recorded against the acceptance criteria below.

| Acceptance criteria | Planned executable evidence |
| --- | --- |
| 001.1, 001.2, 001.3 | `plugin-registry/build-index.test.mjs`: publisher ownership, curated official designation, transferred repository, changed author, tag/archive mismatch, malformed archive, and ownership failure cases |
| 001.4, 003.2, 003.4 | `internal/plugins/marketplace/publisher_test.go`: `TestPublisherTrustBoundary`; fake source ID/name/header, override URL, redirect, custom evidence, and legacy entries |
| 002.1, 002.2, 002.4, 002.5 | `internal/plugins/publisher_install_test.go`: `TestPublisherCatalogInstall`; manifest/digest mismatch, stale selection, upload, sync, restart, record failure, and mixed request rejection |
| 002.3, 002.5 | Existing installation and autoupdate suites plus `internal/plugins/publisher_install_test.go`: publisher continuity, digest rejection, stale selection, and lifecycle protection |
| 004.1 through 004.4 | `internal/plugins/publisher_existing_test.go`: `TestPublisherExistingVersionVerification`; unchanged final release, exact inventory, unavailable version, forged evidence, symlinks, mutation races, source retention, runtime pause/restart, and restart persistence |
| 002.2, 002.6, 003.4 | `internal/canvas/canvas_publisher_test.go`: receipt carry-through/replay, forged upload fields, receipt migration, release projection, and rollback |
| 001.4, 002.1 | `internal/backendapp/canvas_publisher_test.go`: `TestCanvasInstallRequestDoesNotAcceptBrowserPublisherEvidence` plus canvas route coverage |
| 003.1 through 003.5 | Shared component tests plus existing row, update-hook, API-client, and canvas-review suites |

All criterion prefixes in this table are `AC-PLUGINS-PUBLISHER-`.
Use TDD for changed logic. Run each work order's commands and record actual results there.

## E2E tests

| File and project | Outcome and criteria |
| --- | --- |
| `tests/settings/plugin-publisher-identity.spec.ts`, chromium | Browse/install/reload attribution, upload impersonation, source substitution, failed update preservation. 001.4, 002.1-002.5, 003.1-003.5 |
| `tests/settings/mobile-plugin-publisher-identity.spec.ts`, mobile-chrome | Same visible attribution by tap, long author wrapping, reachable install/retry, no horizontal overflow. 003.1-003.5 |
| Existing `tests/canvas/canvas-marketplace.spec.ts` and `mobile-canvas-marketplace.spec.ts` | Add publisher assertions through package review, install/Open, local edit, and upload. 002.6, 003.1-003.4 |

The two settings suites also cover UI-04: explicit same-version verification, progress, errors/retry, unchanged version, and attribution after reload (004.1-004.5).

Use isolated existing E2E fixtures. A URL override must remain unverified in browser tests.
Use backend integration tests with an injected HTTP transport to test the canonical trust root without production network access.
Mocked frontend responses can prove verified-label rendering, but cannot serve as evidence that backend verification works.
Do not add a production trust-bypass environment variable for tests.

## Work orders

- [x] [Task 01: Produce registry publisher evidence](task-01-registry-evidence.md)
- [x] [Task 02: Bind native installs and updates to evidence](task-02-native-installation.md)
- [x] [Task 03: Preserve canvas publisher receipts](task-03-canvas-receipts.md)
- [x] [Task 04: Show publisher attribution and document it](task-04-attribution-ui.md)

Order: 01 -> 02 -> 03 -> 04. Execution completed sequentially. Tasks 01 and 02
define contracts consumed by both later tasks.

## Verification results

Implementation: complete. Product test commands and work-order outcomes are
recorded in the linked work orders.

Implementation verification on 2026-09-18:

- `make -C apps/backend build`: passed.
- `make -C apps/backend e2e-plugin-package`: passed.
- `go test -tags fts5 ./internal/plugins/... ./internal/canvas ./internal/backendapp -count=1`: passed.
- Frontend Vitest focused suites: passed (21 files, 150 tests).
- Review follow-up verification on 2026-09-18: focused plugin backend tests,
  race checks, registry builder tests (including the pull-request entry point),
  frontend tests, typecheck, lint, i18n checks, production/E2E builds, and
  desktop/mobile Chromium flows all passed. The review also added authenticated
  member coverage for trust information and the absence of verification
  controls.
- `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and
  `pnpm run i18n:ratchet`: passed.
- Chromium settings and canvas E2E plus mobile settings, update, and canvas E2E:
  passed (two desktop tests in isolated runs and three mobile tests).
- Public-doc validation, specification validation, specification lint, and
  `git diff --check`: passed.

The full backend `make -C apps/backend test` command was also attempted. Its
failures are confined to pre-existing process-probe timing tests and config or
launcher discovery tests that inherit the task runner's `/root/.kandev/config.yaml`.
The config and launcher packages pass with the inherited Kandev environment
unset. The process-probe package remains unchanged and retains its existing
real-tree timing failures.
Design validation on 2026-09-18:

- `python3 scripts/list-docs.py validate`: passed (288 decisions, 1002 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs docs/decisions docs/plans/plugin-publisher-identity`: passed.
- Work-order requirement IDs, acceptance IDs, design paths, and Markdown file links: checked locally.
- `git status --short -- docs/plans/plugin-publisher-identity`: all five package files present and untracked.

Production code and permanent tests are included in Tasks 01 through 04.
Public docs updates belong to Task 04.

## Related delivery records

The [canvas marketplace package](../canvas-marketplace/plan.md) and
[plugin preview package](../canvas-marketplace-plugin/plan.md) describe the current distribution and gallery foundations.
This package adds publisher evidence without reopening or rewriting their historical results.
Reconcile changed fixture expectations and run their affected canvas/preview suites in Task 04.

## Risks and rollout

- The canonical endpoint and registry workflow are trust dependencies. This does not establish independent publisher signatures.
- Ownership failures omit entries. Existing installations continue with historical attribution.
- Existing releases that claim Kandev incorrectly need maintainer correction before inclusion in the enriched catalog.
- Legacy installs can gain verification through explicit same-version file comparison or a later catalog installation. No name-based backfill is allowed.
- Same-version verification needs that version in the official catalog. Historical-version discovery is outside this package.
- Registry readers, enforcement, UI, and publication must roll out together. A builder-only change does not complete this fix.
- New host provenance cannot be serialized into exported package manifests or accepted from browser inputs.
- Publisher continuity checks must run after download under the existing lifecycle lock to avoid stale update decisions.
