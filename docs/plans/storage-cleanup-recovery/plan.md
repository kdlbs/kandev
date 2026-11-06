---
created: 2026-11-03
status: done
requirements:
  - REQ-SYSTEM-PAGE-GO-CACHE-004
  - REQ-SYSTEM-PAGE-GO-CACHE-005
system_design:
  - ../../specs/system-page/system-design/go-cache-reclamation.md
legacy_specs: []
---

# Implementation plan: Storage cleanup recovery

## Overview

Recover safely from recreated Go cache directories between bounded cleanup passes.
Show compact maintenance results with consistent typography and optional diagnostics.
Implement cache recovery first, then the shared desktop and phone presentation.

System-page owns cleanup policy and operator feedback. The repair extends its
existing [requirements](../../specs/system-page/requirements/go-cache-reclamation.md)
and [design](../../specs/system-page/system-design/go-cache-reclamation.md).
The [direct-deletion decision](../../decisions/2026-10-02-direct-go-cache-reclamation.md)
remains authoritative. No new permission or cleanup authority is introduced.

## Evidence and root cause

The live instance uses `/root/.kandev/data/kandev.db`, identified through its open
process files. Evidence came from SQLite `mode=ro`, filesystem metadata, source,
the supplied screenshot, and a temporary Go overlay reproduction.

| Evidence | Observation |
| --- | --- |
| Run `26f90cad-f0c0-4b02-ae8e-986f143374c7` | Nov 2, 23:53:56-23:54:07 UTC. Removed 62,315,038,941 bytes. `cleanup_in_progress` with `entry_limit_reached`. Stored state: `failed`. |
| Cache directory birth times | All 256 hexadecimal shards were created on Nov 3 at 00:00:46 UTC. Root birth time: Oct 10. |
| Run `3a64920a-a820-416f-992f-99c2642f5d43` | Nov 3, 05:54:07-05:54:10 UTC. Removed 11,544,592,067 bytes. Measured 57,648,496,187 bytes remaining. Eight retained `entry_delete_failed` issues and repeated parent-change causes. Nine other providers skipped due to task activity. |
| Source compatibility | Relevant cleanup and history source matches the running build commit `f5c03760265415cc4216c01ab9e7449e1633c03e`. Investigation checkout: `4a8db14fa4eb2d1bea1621345d2ef8a41a24abc3`. |
| Diagnostic bundle | Backend bundle reported partial coverage because of its archive byte limit. It did not contain the historical failure. Database history supplies the incident evidence. |

`prepareCleanupSession` validates the root before resuming, but retains old shard
handles and inode identities. `deleteRemainder` reads the retained frame while
`inspectFrameEntry` inspects the current path. A recreated shard passes path
inspection but fails `openEntryDirectory` against the old identity. The safety
check is correct. Repeating it per file wastes the pass and produces duplicate
errors until the stale session finishes or reaches another budget limit.

The saved threshold measurement is identical across the two incident runs.
Together with the intervening shard birth times, this supports resumed stale
state as the incident cause. The identity of the process that recreated the
shards is unknown. Go's local `clean -cache` implementation removes hexadecimal
shards while preserving the root, so that command can produce this shape.
This investigation does not establish that a particular task ran it.

The temporary `TestReproStorageResumedShardIdentity` used a private cache fixture.
Its first pass crossed a one-byte threshold and exhausted a three-entry budget.
The unchanged shard resumed and removed four remaining bytes without errors.
Renaming the shard and recreating its file names produced the incident's exact
repeated error. Replacement files survived. The next fresh pass removed six bytes.
Both diagnostic subtests passed. The overlay, fixtures, and private build cache
are temporary and must not become permanent repository artifacts.

The history UI compounds the problem. It prints `run.state` and `run.trigger`
without localization, then prints the joined message and full JSON by default.
`AccordionContent` supplies 12px text and paragraph margins. Go results supply
14px text, and raw JSON supplies 11px text. These defaults explain the screenshot's
mixed text sizes and loose vertical rhythm.

## Scope

### In scope

- Revalidate retained directory frames and restart stale discovery safely.
- Stop and retire stale traversal detected during removal.
- Preserve all existing root, ownership, symlink, mount, and activity safeguards.
- Distinguish incomplete progress, partial cleanup, and failures in history.
- Deduplicate explanations, align result typography, and collapse raw diagnostics.
- Localized desktop/phone presentation and focused regression evidence.
- Update the maintenance-history explanation in `docs/public/operations.md` during implementation.

### Out of scope

Live cleanup, service restarts, schedule changes, background catch-up loops,
automatic build retries, hard quotas, module-cache cleanup, and changes to shared
accordion/card primitives. Other providers retain existing cleanup admission.

## Technical approach

Task 01 changes `gocache/cleanup_session.go` and its safety tests. Validate the
active frame chain before reuse. On ordinary replacement, close stale handles
and perform fresh threshold discovery within the original budget and deadline.
Allow at most one restart per invocation. Keep mutation-time checks authoritative.
Retire a cursor invalidated later in the pass and report one stable issue.

Task 02 adds a pure storage history summary helper and updates
`StorageRunHistory` and `StorageGoCacheResult`. Keep stored states and JSON intact.
All provider outcomes participate in status derivation. A non-Go error must
prevent an expected-progress classification. Unknown errors remain visible as
failures. Bound user-facing explanations to distinct issue categories.

Reuse the existing inline accordion. Keep the raw message and JSON inside
Technical details, initially closed. Apply feature-local typography and paragraph
spacing overrides. Phone uses labelled vertical facts and touch-sized disclosures.
The existing page owns scrolling. An opened diagnostic region contains its own
overflow. Share the result model and state across viewports.

## ASCII UI preview

Entry point for all views: Settings > System > Storage > Host > Maintenance history.
Labels show illustrative English copy. Grouping, disclosure order, and information
hierarchy are structural. Spacing is illustrative. All product labels use i18next.

UI-01: Current expanded failure, supported by source and the supplied screenshot.

```text
[failed] Scheduled  Nov 3, 05:54
go_cache: parent changed ... parent changed ... entry_delete_failed ...
10.75 GB removed from the Go build cache.

53.69 GB remains in the Go build cache.

Go cache cleanup during active tasks was enabled for this run.

Cleanup was partial.

Other cleanup was skipped because tasks are active.
{ full diagnostic JSON shown immediately }
```

UI-02: Proposed desktop, same historical failure expanded.

```text
[Partial] Scheduled  Nov 3, 05:54                         [Collapse]
Go build cache
Removed: 10.75 GB             Remaining: 53.69 GB
Some cache entries changed during cleanup.
Other cleanup skipped: tasks were active.
Cleanup during active tasks was enabled.
[Technical details >]
```

UI-03: Proposed phone, same result expanded.

```text
[Partial] Scheduled                            [Collapse]
Nov 3, 05:54
Go build cache
Removed: 10.75 GB
Remaining: 53.69 GB
Some cache entries changed
during cleanup.
Other cleanup skipped:
tasks were active.
Cleanup during active tasks
was enabled.
[Technical details >]
```

The page owns vertical scrolling in UI-02 and UI-03. Technical details opens
inline with the original message and JSON in a bounded diagnostic region.
Phone controls have at least 44px hit targets. The nearest mobile exemplars
are the current storage history accordion and storage policy card.

UI-04: Expected progress, empty history, and actual failure.

```text
[Incomplete] Scheduled  Nov 2, 23:54
Removed: 58.04 GB             Remaining: Unknown
The entry limit was reached. Cleanup can continue on a later run.
[Technical details >]

No storage maintenance runs yet.

[Failed] Manual  Nov 3, 06:00
Cleanup could not complete. View technical details.
[Technical details >]
```

Unknown errors must not become Incomplete. Loading and section-error states
remain independent. Result facts and error explanations use 14px body text.
Metadata and status badges retain their smaller, intentional type scale.
Views UI-02 through UI-04 map to `AC-SYSTEM-PAGE-GO-CACHE-005.5` through `.9`.

## Tests

| Acceptance | Planned regression |
| --- | --- |
| `004.8`, `.11`, `.13` | `TestCleanupRestartsRecreatedShardBeforeResume`: small-budget pass, shard replacement, fresh discovery, no repeated parent errors. Pair with unchanged-cursor progress. |
| `004.7`, `.12`, `.13` | Below-threshold replacement survives. Symlink, root replacement, unavailable mount identity, and same-device mount substitutions preserve sentinel files. |
| `004.8`, `.13` | Replacement during removal retires the cursor, preserves successful-byte accounting, and reports one issue. Cancellation and repeated churn remain bounded. |
| `005.5`, `.7` | `storage-run-summary.test.ts`: expected progress, mixed provider failures, malformed/unknown errors, legacy payloads, unknown remaining measurement. |
| `005.6`, `.8`, `.9` | History component tests: distinct explanations, translated labels, diagnostics initially closed, keyboard disclosure, intact raw data. |

## E2E tests

Extend `tests/system/storage-maintenance.spec.ts` and
`tests/system/mobile-storage-maintenance.spec.ts` with maintenance-history cases.
Use isolated API response fixtures matching the incident. Assert collapsed raw
details, deduplicated explanations, status semantics, and known/unknown bytes.
Open details, inspect the retained diagnostic message, then close them.
Measure computed text sizes, paragraph gaps, touch geometry, and page containment.
Include phone widths immediately below and above the 768px layout boundary.
Capture desktop and phone screenshots for comparison with UI-02 and UI-03.
Restore any modified fixture settings. Do not interact with the live instance.

## Work orders

- [x] [Task 01: Recover stale cache scans](task-01-cache-scan-recovery.md)
- [x] [Task 02: Clarify maintenance history](task-02-maintenance-history.md)

Both work orders execute sequentially. No delegation is authorized.
Each work order owns its exact verification commands and implementation results.

## Verification results

Design-stage checks passed on 2026-11-03:

- Temporary Go overlay diagnostic: both unchanged and recreated-shard cases passed.
  Command: `(cd apps/backend && GOCACHE=/tmp/kandev-storage-repro-cache go test -trimpath -tags fts5 -overlay=/tmp/kandev-storage-repro-overlay.json ./internal/system/storage/gocache -run '^TestReproStorageResumedShardIdentity$' -count=1 -v)`.
- `python3 scripts/list-docs.py validate`: 427 decisions and 1,893 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- Local `.github/scripts/pr-docs.cjs` `validateCoverage` preflight: both work orders
  and their plan, requirement, acceptance, and design references passed. The probe
  included the planned Go production path so a documentation-only exemption did
  not bypass reference validation. It did not publish a GitHub status.
- `git diff --check -- docs/plans/storage-cleanup-recovery docs/specs/system-page`: passed.
- `git status --short -- docs/plans/storage-cleanup-recovery`: confirmed all three new package files.

Implementation results on 2026-11-03:

- `go test -trimpath ./internal/system/storage/gocache -run '^TestCleanupRestartsRecreatedShardBeforeResume$' -count=1 -v`:
  failed before the fix with three `go-cache parent changed after scan` errors and
  passed after it. The package race run and all `internal/system/storage/...`
  tests passed. Changed-file Go lint reported 0 issues.
- The frontend summary, history component, Go result component, seven catalogs,
  and desktop/phone maintenance specs are implemented. Vitest ran 180 storage
  tests, `pnpm run typecheck` passed, and changed-file ESLint reported no errors
  or warnings. Raw-only and partial-only Go failures keep a visible generic
  explanation, read/delete failures no longer claim directory replacement, and
  the incident still deduplicates to one changed-entry explanation.
- Review remediation also made the below-threshold replacement regressions start
  from an established above-threshold delete cursor and a partial-unlink cursor,
  so stale threshold authority cannot pass unnoticed.
- `pnpm run i18n:zh-hant`, `pnpm run i18n:pseudo`, `pnpm run i18n:check`, and
  `pnpm run i18n:ratchet` passed.
- `pnpm e2e:run --host --project chromium -- tests/system/storage-maintenance.spec.ts`
  passed all 11 tests. The mobile project passed all 10 tests, including the
  767px and 768px history geometry checks.
- `node scripts/validate-public-docs.mjs` validated 54 pages, and the specification
  catalog and linter checks passed.

The temporary overlay and diagnostic extraction are removed. The live instance
remains unchanged.
The earlier [Go cache package](../go-cache-reclamation/plan.md) remains the historical
delivery record. Its results are not evidence for this repair. Related fallback,
quarantine, and original-maintenance packages need no scope or status changes.

## Risks

- Relaxing inode or mount checks can redirect deletion. Preserve them at mutation boundaries.
- A fresh threshold scan can need multiple passes. Do not inherit stale byte totals.
- An unrelated provider error can coexist with Go progress. Status derivation must inspect every provider result.
- The eight retained issue samples are not a total failure count.
- Continuous external cache replacement can prevent completion. Recovery must remain bounded.
- The original shard-replacement actor cannot be identified from retained evidence.
