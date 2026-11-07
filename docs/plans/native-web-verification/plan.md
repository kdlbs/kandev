---
created: 2026-11-03
status: in-progress
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-004
  - REQ-PLATFORM-CI-PERFORMANCE-005
  - REQ-PLATFORM-CI-PERFORMANCE-009
  - REQ-PLATFORM-CI-PERFORMANCE-010
  - REQ-PLATFORM-CI-PERFORMANCE-011
system_design:
  - ../../specs/platform/system-design/ci-performance.md
  - ../../specs/platform/system-design/web-native-verification.md
legacy_specs: []
---

# Implementation Plan: Native web verification

## Overview

Replace full web lint with Oxlint and migrated JS plugins.
Replace the web compiler invocation with `tsrs --noEmit --fast`.
Shorten existing jobs within the accepted 60-job limit.

The user selected those complete migrations after the [local investigation](evidence.md).
The full Oxlint experiment had 37 unresolved findings, and the native compilers reported 12 migration errors.
Those failed candidates do not establish accepted production savings.
The package first preserves comparison evidence, then repairs compatibility, switches entry points, and measures adoption.

Platform owns the shared verification contract.
The [new design](../../specs/platform/system-design/web-native-verification.md) defines the tool boundaries.
The [decision](../../decisions/2026-11-03-native-web-verification.md) records the selected alternatives and conditional cutover.

Implementation was requested in a later turn and is recorded in the work orders.
The package remains uncommitted.
No publication or hosted measurement was performed; Task 05 stays open for that evidence.

## Scope

### In scope

- Exact native tool pins and a separate reference-compiler installation.
- Complete rule, scope, suppression, and source inventory mapping.
- Compatible repairs for the 12 compiler and 37 lint findings.
- Real native CLI guard fixtures and compiler diagnostic comparisons.
- Existing full, focused, changed-file, and hosted command routing.
- Retained TypeScript and ESLint API consumers.
- Comparable local measurements and hosted adoption evidence.

### Out of scope

- Hybrid ESLint/Oxlint as the final lint command or ordinary tsrs as the selected compiler mode.
- Blacksmith, new runners, paid capacity, new jobs, extra shards, or a global admission service.
- New test-selection exemptions, weaker assertions, disabled isolation, longer timeouts, or more retries.
- Vite/Rolldown, desktop compiler, standalone plugin SDK compiler, runtime APIs, and release redesign.
- New UI behavior or a new phone composition. Punctuation keeps identical rendered characters.

## Technical approach

### Compatibility before command replacement

Task 01 adds `.github/scripts/web-toolchain-evidence.py` and focused standard-library tests.
It compares complete normalized diagnostics and inventories, rather than treating a green exit as conformance.
Reference packages use an isolated temporary installation with exact pins.
They cannot shadow workspace `tsc` or replace the classic TypeScript API.

Task 02 adds the pinned tsrs candidate and repairs six compiler-affected source files.
The current required compiler remains active during this repair.
Task 03 adds `apps/web/oxlint.config.mjs`, preserves required plugins, and resolves the full lint inventory.
The current full ESLint command remains active until both candidate paths pass.

The initial tool pins are tsrs `0.20.1-ts7.1.0-beta`, reference TypeScript `7.1.0-beta`, and Oxlint `1.91.0`.
The tested tsrs reference commit is `3e9236361fa9`.
New versions need a recorded reason and fresh comparisons.

### Required command routing

Task 04 preserves `pretypecheck` and `rich-output:check`, then routes the final compiler call to `tsrs --noEmit --fast`.
The required lint command uses the complete Oxlint configuration with warnings treated as failure.
Focused lint commands and the changed-file hook use the same policy and version.
The targeted upgrade-regression lint invocation uses the accepted native configuration.
ESLint remains available for RuleTester and existing API-based ratchets and inventory tools.

Task 04 performs this command routing as a conditional cutover.
Task 05 remains the adoption gate: the prior commands stay a tested rollback path,
and no performance claim is adopted until three hosted pairs pass.

The workflow graph, runner selection, permissions, required check names, and native test commands stay intact.
New helper tests join an existing validation job.
Task 04 updates tool-specific engineering guidance and the existing Query/architecture guard designs at cutover.
It does not promote old CI initiatives or change their completion status.

### Performance acceptance

Task 05 captures three successful comparable baseline/candidate pairs on identical repaired application and test source.
Full lint must reduce its median execution by at least 50%.
The complete typecheck command must reduce its median execution by at least 75%.
Combined occupied frontend execution must decrease without lost checks or a resource regression.

Cold dependency costs, plugin startup, metadata generation, and optional reference comparisons appear separately.
The final decision includes total required-command cost.
Cold and warm cache classes remain separate.
Every failed, cancelled, or incomplete attempt remains visible.

Hosted acceptance requires three comparable pairs from natural runs or separately authorized measurements.
Unknown or insufficient hosted evidence keeps Task 05 open.
Queue delay remains a separate observation, not an optimization target.
These targets do not claim a reduction for the whole CI pipeline.

## Tests

| Acceptance | Evidence owner |
| --- | --- |
| 009.1, 010.1 | Task 01 inventory and source-provenance cases. |
| 009.2–009.5 | Task 03 native CLI rule ledger and guard fixtures. |
| 010.2, 010.5 | Tasks 01–02 reference, fast-mode, negative-corpus, and stale-cache cases. |
| 010.3 | Task 02 existing observer, socket, resize-buffer, and terminal tests. |
| 010.4 | Tasks 02 and 04 classic API resolution, SDK, and unchanged package checks. |
| 011.1, 011.4 | Task 04 package/hook/workflow contract checks and full frontend suite. |
| 011.2, 011.3 | Task 05 complete-command measurements and hosted evidence. |
| 011.5 | Task 05 rollback rehearsal and recorded acceptance disposition. |

The table uses the `AC-PLATFORM-CI-PERFORMANCE-` prefix.
Work orders name exact files and commands.
Native fixture commands must exercise the shipped configuration and original owner filenames.
Unit rule tests remain useful but cannot replace command-wiring evidence.

The compiler corpus begins with the eight injected errors from the investigation.
It adds positive cases and relevant generic, recursive, inference, and checker-history regressions.
Both default and fast tsrs remain comparison subjects, even though fast is the selected delivery.
Any relevant missing reference diagnostic blocks adoption.

There is no new user-facing journey or rendered UI behavior.
Existing affected component tests prove unchanged text and interaction assertions.
The duplicate E2E helper repairs receive focused unit coverage and retain ordinary E2E selection.
No extra browser suite is added solely to validate a compiler binary.

## Work orders

All work is sequential. The tasks share package metadata, configuration, or acceptance evidence.

- [x] [Task 01: Preserve native verification evidence](task-01-preserve-verification-evidence.md).
- [x] [Task 02: Repair native compiler compatibility](task-02-repair-compiler-compatibility.md).
- [x] [Task 03: Migrate the complete lint ruleset](task-03-migrate-full-oxlint.md).
- [x] [Task 04: Route required checks to native tools](task-04-route-native-checks.md).
- [ ] [Task 05: Measure adoption and rollback](task-05-measure-adoption.md).

Tasks 02 and 03 keep the existing required commands active.
Task 04 must not replace them until candidate coverage and compatibility pass.
Task 05 owns final performance adoption and remains open until its hosted evidence is complete.

## Related delivery packages

- [CI performance](../ci-performance/plan.md) retains its earlier cache, partition, and complete-CI acceptance.
- [E2E efficiency](../e2e-ci-efficiency/plan.md) retains its browser-cache rollout work.
- [External runner capacity](../external-e2e-runner-capacity/plan.md) retains runner and trust boundaries.

This package does not reopen their implementation or mark their pending work complete.

## Verification results

Design validation passed on November 3, 2026:

- The documentation catalog validated 429 decisions and 1,894 specifications.
- All 36 specification-linter tests passed. Full specification lint passed.
- Relative links and the curated 12/37 diagnostic inventories passed validation.
- The actual documentation-only diff passed the PR documentation preflight as exempt.
- All five work orders passed the same validator against a representative planned source change.
- The prose self-check found no sentence beyond the descriptive word limit.
- Package whitespace, document discovery, and final status checks passed.

Implementation checks passed for Tasks 01-04 on November 3, 2026:

- The evidence helper tests (40), routing contract (9), upgrade-regression
  contract (3), existing workflow contracts, `actionlint`, full web lint,
  full typecheck, i18n checks, the E2E wait ratchet, the complete frontend
  suite (3,261 files, 29,610 tests), the Vite build, and plugin SDK checks pass.
- Compiler comparison passed for TypeScript 5.9.3, reference 7.1.0-beta, tsrs
  default, and tsrs fast on the complete project and the curated corpus.
- Lint comparison reported equal 9,810-file inventories and zero findings.
- Three local complete-command pairs reduced median lint by 65.62%, complete
  typecheck by 94.46%, and combined occupied execution by 81.61%.

Task 05 remains open because three comparable hosted pairs have not been
collected. No workflow was dispatched. The [baseline](baseline-2026-11-03.json)
retains the investigation record; the [evidence](evidence.md) records the
implementation and local adoption results. Temporary diagnostic fixtures did
not modify repository source or CI configuration.

## Risks

- Fast mode can differ from the pinned reference near checker-history or instantiation limits.
- JS plugin compatibility is incomplete and requires exact scope and entity handling checks.
- A native compiler alias can replace a workspace binary accidentally.
- Removing duplicate methods can change behavior unless the effective last implementation remains intact.
- Local timings on a shared host cannot establish hosted adoption or a queue improvement.
- New native dependencies can increase cold installation costs despite lower command execution.

## Documentation impact

This turn changes internal requirements, design, decision, plan, and evidence only.
The later implementation updates tool-specific engineering guidance and guard-host descriptions.
The public `make typecheck` command and runtime behavior remain unchanged.
