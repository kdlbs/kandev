---
created: 2026-10-05
status: implemented
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-001
system_design:
  - ../../specs/system-page/system-design/workspace-storage-discovery.md
legacy_specs: []
---

# Fix plan: Workspace storage discovery

## Overview

Restore task workspace measurement when unrelated directories exist beneath the tasks root.
One sequential work order corrects shared discovery and proves analysis, cleanup safety, and rendered recovery.
The system-page system owns this package because it owns storage measurements and maintenance eligibility.

## Confirmed evidence

The reported page is `/settings/system/storage`.
The host contains `/root/.kandev/tasks/kandev-host-v0.96.0/CLAUDE.md -> AGENTS.md`.
The checkout lacks a recognized task-root shape, so `discoverTaskRoot` calls `discoverScratchRoots`.
That function rejects the child link. `discoverTaskRoots` propagates the error before `Analyze` measures any valid root.
`storageOverview.summaryValue` returns `available: false`.
The existing row displays Unavailable and defaults absent subset values to zero.

A temporary Go reproduction combined a marked valid task and this unmarked checkout.
Analysis returned the screenshot error and zero bytes. Removing the fixture link allowed analysis to succeed with recognized workspace bytes.
The temporary test was removed. The reproduction changed no live workspace files.
Permanent regression tests must expect successful measurement with the link still present.

## Scope

### In scope

- Shared discovery for read-only analysis, orphan quarantine, and dependency cleanup.
- Recognition based on markers and supported legacy directory shapes.
- Existing warnings for omitted unclassified directories.
- Permission-denied unclassified paths remain preserved and do not block recognized measurements.
- Compatibility and no-follow regression coverage.
- Real-backend desktop and phone proof through existing Storage controls.

### Out of scope

- Removing, moving, or adopting the reported checkout.
- Counting unrelated checkout data as task workspace bytes.
- New API fields, persistence, migrations, settings, flags, or scan deadlines.
- UI components, translations, warning presentation, layout, navigation, and mobile composition changes.
- Broad review, full-suite verification, publication, and delegated implementation.

## Technical approach

Extract shared task-root discovery into `apps/backend/internal/system/storage/workspaces/discovery.go`.
Separate scratch-child inspection from candidate classification. Preserve valid marker precedence and
legacy semantic recognition.
Require canonical UUID parent and child names for unmarked scratch candidates.
Marker-backed scratch children remain valid with non-UUID names.

Complete scratch-container recognition before applying child-symlink rejection.
A UUID parent or at least one valid scratch marker identifies a container.
An unrecognized parent's ordinary links remain opaque. Its unmarked repository folders remain unclassified.
Invalid markers, recognized root links, unsafe ancestors, and non-permission I/O failures retain
errors. Permission-denied reads are omitted only when the affected path has no positive task-layout
evidence; recognized semantic and canonical UUID paths retain errors.
Use the existing UUID dependency and extract small helpers if complexity limits require them.

No inventory shape changes are necessary.
`Analyze`, `Cleanup`, and `CleanupDependencies` share the result.
Existing scanners skip nested links. Existing inventory, age, quarantine, and dependency guards remain mandatory.

| Layout | Expected behavior | Evidence | Unsupported fallback |
| --- | --- | --- | --- |
| Marked semantic or scratch | Preserve current recognition | Layout regression and existing suite | Invalid marker remains an error |
| Unmarked semantic | Preserve current naming rule | Existing legacy test | Other names need scratch evidence |
| Unmarked UUID workspace/task pair | Recognize legacy scratch root | UUID compatibility regression | Noncanonical names remain untouched |
| Unrelated checkout and ordinary children | Omit with warning | Analysis, cleanup, dependency regressions | No automatic adoption |
| Permission-denied unclassified checkout or scratch child | Omit with warning and keep recognized bytes available | Permission-probed discovery regressions | Recognized semantic and UUID path errors remain visible |
| Symlink under recognized scratch container | Retain rejection | Control-symlink regression | No target traversal |

The ADR [records the recognition boundary](../../decisions/2026-10-05-workspace-storage-discovery.md).
The [system design](../../specs/system-page/system-design/workspace-storage-discovery.md) owns the durable flow.

## Tests

The tests below are implemented regressions and evidence for the completed work order and PR fixup.

| Criteria | Test file and completed tests |
| --- | --- |
| `001.7`, `001.9`, `001.10` | `apps/backend/internal/system/storage/workspaces/provider_discovery_test.go`: `TestAnalyzeKeepsValidWorkspacesWithUnclassifiedCheckout` |
| `001.11` | Same file: `TestUnclassifiedDirectoriesAreNotMeasuredOrCleaned`, `TestWorkspaceDiscoveryPreservesSupportedLayoutsAndWarnsUnmarkedChildren`, `TestCleanupKeepsUnclassifiedCheckoutBesideEligibleOrphan` |
| `001.10`, `001.11` | Same file: `TestWorkspaceDiscoveryRejectsSymlinksInRecognizedScratchContainers`, `TestWorkspaceDiscoveryRejectsDirectTaskRootSymlink`, and existing incomplete-inventory and symlink-control tests |
| `001.9`, `001.11` | Same file: `TestWorkspaceDiscoveryRejectsSemanticMarkerInsideScratchContainer`, `TestWorkspaceDiscoveryOmitsUnreadableUnclassifiedCheckout`, `TestWorkspaceDiscoveryOmitsUnreadableUnmarkedScratchSibling`, `TestWorkspaceDiscoveryRetainsPermissionErrorsForRecognizedRoots` |
| `001.11` | `apps/backend/internal/system/storage/workspaces/dependency_discovery_test.go`: `TestCleanupDependenciesKeepsUnclassifiedCheckout` |

The primary regression must fail on the existing symlink discovery error before the production change.
Use exact expected bytes from recognized fixture roots, including existing marker files.
Cover the unrelated checkout with and without a symlink, an outside-target sentinel, and ordinary `.git`, `apps`, and `node_modules` directories.
Cleanup must still quarantine one eligible recognized orphan while preserving the unrelated checkout.
Dependency cleanup must prune an eligible marked archived workspace while preserving the unrelated checkout's dependencies.
Existing marked synthetic scratch fixtures retain their names.
The unmarked legacy test in `provider_test.go` must use canonical UUID names instead of `workspace-legacy/task-legacy`.

## E2E tests

| Flow | Criteria | File and project |
| --- | --- | --- |
| Analyze real isolated storage and expand measured Task workspaces | `001.7`, `001.9`, `001.10` | `apps/web/e2e/tests/system/workspace-storage-discovery.spec.ts`, `chromium` |
| Repeat Analyze through touch controls and inspect the same measured row | `001.7`, `001.9`, `001.10` | `apps/web/e2e/tests/system/mobile-workspace-storage-discovery.spec.ts`, `mobile-chrome` |

Both tests capture a fresh baseline before adding a valid marked root and the unrelated checkout under
`backend.tmpDir/.kandev/tasks`. They compare the new fixture's byte contribution with that baseline,
so another recognized workspace cannot invalidate the check.
Use a shared fixture helper in `apps/web/e2e/helpers/workspace-storage-discovery.ts`.
The helper owns only its unique fixture paths and removes them in `finally` or `afterEach`.
The tests must use a fresh manual Analyze result, not a cached or mocked overview.
They assert real response bytes, the omission warning, a measured workspace row, and unchanged fixture files.
Existing long-path wrapping and phone composition remain unchanged. The phone test also checks document overflow.
No ASCII UI preview is necessary because this work changes backend discovery rather than rendered structure or copy.

## Work orders

- [x] [Task 01: Correct discovery and prove safe workspace measurements](task-01-safe-discovery.md)

Task 01 was sequential and had no dependencies. It is complete.

## Verification

Run commands from the repository root. Install workspace dependencies once before the first pnpm command in a fresh worktree.

```bash
(cd apps/backend && go test -trimpath ./internal/system/storage/workspaces -run '^TestAnalyzeKeepsValidWorkspacesWithUnclassifiedCheckout$' -count=1 -v)
(cd apps/backend && go test -trimpath -race ./internal/system/storage/workspaces -count=1)
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run --project chromium tests/system/workspace-storage-discovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/system/mobile-workspace-storage-discovery.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the two managed E2E commands sequentially. Each command builds the current backend and web assets.
Record discovered test counts and actual results in the work order.
The workspaces package suite covers every changed Go test file.

## Verification results

Design-package validation passed on 2026-10-05:

- `python3 scripts/list-docs.py validate`: 349 decisions and 1,342 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: linked package returned `covered` with no errors.
  This read-only preflight used actual package content and an in-memory `provider.go` change trigger.
- `git diff --check`: passed. Package status confirms the new work order is present and uncommitted.

Implementation verification passed on 2026-10-05:

- The RED gate failed with the expected `symlink beneath tasks root` discovery error.
- `go test -trimpath -race ./internal/system/storage/workspaces -count=1`: passed.
- `pnpm install --frozen-lockfile`: passed. Playwright discovery reported zero errors.
- Desktop E2E: 1 test passed. Phone E2E: 1 test passed. Both managed runs built the backend and Vite assets.
- Targeted ESLint passed for the new E2E helper and both specs.
- `python3 scripts/list-docs.py validate`: 349 decisions and 1,342 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed. `python3 scripts/lint-spec-files.py --all`: passed.
- Final PR documentation coverage preflight: `covered`, no errors. Final `git diff --check`: passed.
- After the discovery refactor and PR review fixes, the workspace race suite and `make build` passed. `golangci-lint run ./internal/system/storage/workspaces --timeout=5m` reported 0 issues; `provider.go` and `discovery.go` are 721 and 164 effective lines, respectively. The permission regressions also passed with filesystem modes enforced as `nobody`.
- The PR documentation coverage preflight passed again after lifecycle wording was synchronized: `covered`, no errors.
- PR fixup RED evidence: with permission bits enforced as `nobody`, unreadable unclassified parents and scratch siblings both aborted analysis before the fix. The semantic-marker and recognized-root boundary regressions passed against the existing implementation.
- PR fixup verification: desktop and mobile workspace storage E2E each passed after adding an isolated baseline; targeted ESLint passed for the helper and all three affected specs. The saved-view recovery E2E's 43.99994px measurement now rounds to hundredth-pixel precision; its focused test passed three repetitions.

## Risks

- Skipping links alone leaves unsafe inference of repository folders. The regression must test both linked and link-free unknown checkouts.
- Unmarked custom non-UUID scratch paths stop participating. They remain intact, and valid marked paths retain support.
- UUID syntax is legacy layout evidence, not a replacement for inventory, liveness, or age checks.
- Entry order must not change whether a recognized scratch container rejects child links.
- Cached unavailable results persist until normal refresh or manual Analyze. E2E must force a fresh scan.
- Ignored paths are absent from this category. The page still describes counted categories rather than complete filesystem usage.

## Documentation impact

The package extends the existing [storage requirement](../../specs/system-page/requirements/storage-maintenance.md).
Its design and ADR record the classification boundary.
Public documentation and screenshots need no change for this implementation.
The intended controls and measurement contract remain the same.
