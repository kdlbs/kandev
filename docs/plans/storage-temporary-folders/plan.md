---
created: 2026-09-11
status: complete
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-003
  - REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-004
system_design:
  - ../../specs/system-page/system-design/storage-temporary-folders.md
legacy_specs: []
---

# Implementation Plan: Temporary storage visibility and cleanup

## Overview

Deliver temporary-folder visibility first, then optional cleanup of registered artifacts, then integrated browser evidence.
Each slice preserves the distinction between measured storage and cleanup ownership.
All work stays sequential in the primary session.

- [Requirements](../../specs/system-page/requirements/storage-maintenance.md): active extensions 003 and 004.
- [System design](../../specs/system-page/system-design/storage-temporary-folders.md).
- [Accepted decision](../../decisions/2026-09-11-temporary-storage-visibility-policy.md).

## Scope

### In scope

- Effective service temp folder and distinct Unix `/tmp`, measured without mutation.
- Informational footprint, partial states, cached progress, and wrapped path details.
- Off-by-default saved cleanup option for existing registered artifact kinds.
- Fresh ownership and filesystem identity checks, saved retention, safe cross-filesystem quarantine,
  quarantine result wording, and desktop/phone access.

### Out of scope

- General shared-temp cleanup, arbitrary paths, shell commands, legacy adoption, and new producers.
- Direct deletion before retention and changing agent temp variables.
- Host-wide reconciliation, hard-link deduplication, and physical-block accounting.

## Technical approach

Task 01 adds `tempstore`, bounded scanner options, overview projection, and resource rendering.
The broad footprint is excluded from Total counted, so existing overlap attribution stays intact.
Task 02 extends the saved settings document and the existing `tempartifacts.Provider`.
Both explicit and scheduled actions share eligibility and quarantine behavior.
Task 03 supplies browser evidence and reconciles operator documentation.

### Companion packages

The completed packages remain historical evidence:
[owned artifacts](../storage-temp-artifacts/plan.md),
[progressive analysis](../storage-analysis-progress/plan.md),
[database footprint](../storage-database-footprint/plan.md),
and [original maintenance](../storage-maintenance/plan.md).
Do not reopen their completed work orders or overwrite their recorded test counts.
This package supersedes their manual-only expectation. The affected current design passages and
companion manifests link this completed package.

## ASCII UI preview

UI-01: Settings > Storage, analysis expanded. Proposed desktop composition:

```text
Storage analysis                              [Analyze]
Total counted: <classified GB>

v System temporary folders          <GB>  Read-only
  Informational. Can overlap counted categories.
  /tmp                              <GB>  Complete
  <distinct service temp folder>    <GB>  Partial
  Some entries could not be measured.

> Kandev temporary artifacts         <GB>
```

UI-01 phone: one page scroll owner, root details beneath the value:

```text
Storage analysis
[Analyze]
Total counted: <classified GB>

v System temporary folders
  <GB>  Read-only
  Informational. Can overlap
  counted categories.
  /tmp
  <GB>  Complete
  <long service path wraps>
  <GB>  Partial
  Some entries could not
  be measured.
```

UI-02: Settings > Storage, policy. Shared semantic order:

```text
Temporary artifacts
Clean stale Kandev temporary artifacts     [off]
Only registered, inactive artifacts older than 24 hours.
Files move to quarantine before permanent deletion.
Shared temporary files are excluded.

[Clean stale artifacts]
[Save changes]  (existing page action)

Result: <GB> moved to quarantine.
Space is freed after permanent deletion.
```

On phones, the switch remains beside its wrapped label.
The manual action fills the available width and has a 44-pixel minimum hit target.
Desktop actions retain normal 28-pixel density.
Required explanations stay inline, with no tooltip dependency or new overlay.
The existing Storage accordion is the mobile exemplar.

Loading: the root row shows Measuring, never zero before a result.
Empty: a complete zero measurement shows 0 GB.
Partial: the measured bytes remain visible with a Partial label.
Unavailable: no byte value appears; Analyze can retry.
Policy disabled: manual cleanup remains available to an authorized user.
Busy or unauthorized: existing disabled reasons apply to mutations.
Cleanup failure: report the failed move and preserve the original.

These structures are required by AC-003.3, .5, .6, .8 and AC-004.7, .8.
Here, AC prefixes mean `AC-SYSTEM-PAGE-STORAGE-MAINTENANCE`.
Spacing and placeholder values are illustrative.

## Tests

The test names below record implementation evidence for the completed package.

| Criteria | Evidence |
| --- | --- |
| 003.1, .2, .7, .9 | `tempstore/provider_test.go: TestAnalyzeTemporaryRoots`, alias/nesting, refreshed nested-mount, and root-replacement tests |
| 003.5, .6 | `TestAnalyzeTemporaryPartialResults`, `TestAnalyzeTemporaryCancellation`, scanner empty/nested-root and interrupted-partition tests, overview progress/cache tests |
| 003.3, .4, .8 | `storage-totals.test.ts` informational exclusion and `storage-overview-card.test.tsx` state/path cases |
| 004.1, .2, .3 | settings roundtrip tests and `TestProviderCleanupPolicyMatrix` |
| 004.4, .5, .6 | `TestCleanupRevalidatesArtifact`, owner-death per-run reconciliation, uncertain-owner, replacement, age, and EXDEV staged-copy cases |
| 004.7, .8 | retention/recovery tests, run-history wording, policy persistence, and phone flows |

Task 01 and Task 02 add TDD tests for their changed logic.
Tests use disposable files and injected clocks, owner probes, and mount information.

## E2E tests

Add `tests/system/storage-temporary-folders.spec.ts` for chromium.
Add `tests/system/mobile-storage-temporary-folders.spec.ts` for mobile-chrome.
Cover all visible states in UI-01 and UI-02 using isolated fixtures.
Use real backend analysis against injected fixture roots for the refresh case.
Use controlled responses only for partial/unavailable presentation and long paths.
Provider integration tests prove mutation safety separately from browser presentation.

The phone flow inspects paths, saves the option, runs explicit cleanup, and reads the result.
Assert no page horizontal overflow and actual 44-pixel action targets.
Capture and inspect a phone screenshot during the guarded run.

## Work orders

- [x] [Task 01: Temporary-folder visibility](task-01-temporary-folder-visibility.md)
- [x] [Task 02: Owned temporary cleanup policy](task-02-owned-temporary-cleanup-policy.md)
- [x] [Task 03: Temporary storage flow evidence](task-03-temporary-storage-flow-evidence.md)

## Verification results

Design validation passed on 2026-09-11:

- `rtk python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `rtk python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `rtk git diff --check`: passed.
- Work-order requirement IDs, document links, and whitespace: passed.
- System index size: 2199 bytes, within its limit.

Implementation and rendered verification completed on 2026-09-12:

- Backend storage and backend-app tests passed for the temporary-folder scanner, overview wiring,
  saved cleanup policy, lifecycle revalidation, owner-death reconciliation, and quarantine result
  contract: 1,139 tests across 9 packages.
- Focused web tests passed for overview rendering, totals, policy settings, maintenance settings,
  and run history. Web typecheck and i18n checks passed.
- Chromium temporary-storage E2E passed: 3 tests. Mobile Chromium temporary-storage E2E passed:
  2 tests, including phone screenshot and overflow checks.
- Public documentation and specification validation passed. `git diff --check` passed.
- Generated E2E diagnostics were removed; no diagnostic artifacts remain.
- The repository-wide `make -C apps/backend test` command still has unrelated existing failures in
  process probing, config discovery, office migrations, and an orchestrator test panic.

## Risks

- Shared temp traversal can be slow or incomplete; the source has its own deadline and partial state.
- Mount identity differs by platform; unsupported discovery must fail closed.
- A partial informational measurement must not silently become zero or alter classified totals.
- Cleanup cannot reclaim unregistered caches, even when those dominate the broad footprint.
- Cross-filesystem quarantine stages a verified copy and temporarily needs space in both locations;
  failures preserve the source and leave a retryable failed intent.
