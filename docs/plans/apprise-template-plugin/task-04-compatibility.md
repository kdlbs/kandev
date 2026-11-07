---
id: 04-compatibility
title: Packaged upgrade and recovery evidence
status: in_progress
wave: 3
depends_on:
  - 01-host-contract
  - 02-template-plugin
  - 03-legacy-import
plan: plan.md
requirements:
  - REQ-PLATFORM-NOTIFICATION-FORMATTER-002
  - REQ-PLATFORM-NOTIFICATION-FORMATTER-003
  - REQ-PLATFORM-NOTIFICATION-FORMATTER-004
acceptance_criteria:
  - AC-PLATFORM-NOTIFICATION-FORMATTER-002.2
  - AC-PLATFORM-NOTIFICATION-FORMATTER-002.3
  - AC-PLATFORM-NOTIFICATION-FORMATTER-002.4
  - AC-PLATFORM-NOTIFICATION-FORMATTER-003.3
  - AC-PLATFORM-NOTIFICATION-FORMATTER-003.4
  - AC-PLATFORM-NOTIFICATION-FORMATTER-004.1
  - AC-PLATFORM-NOTIFICATION-FORMATTER-004.2
  - AC-PLATFORM-NOTIFICATION-FORMATTER-004.3
  - AC-PLATFORM-NOTIFICATION-FORMATTER-002.5
  - AC-PLATFORM-NOTIFICATION-FORMATTER-002.6
  - AC-PLATFORM-NOTIFICATION-FORMATTER-003.5
  - AC-PLATFORM-NOTIFICATION-FORMATTER-003.6
system_design:
  - ../../specs/platform/system-design/notification-formatter-plugins.md
---

# Task 04: Packaged upgrade and recovery evidence

## Summary

Exercise the actual packaged plugin on isolated hosts at pinned revisions and
produce compatibility, UI, upgrade, and rollback receipts. Complete the two PR
records with actual checks and synthetic screenshots; publishing remains gated
by host/SDK support and separate release direction.

## In scope

- In the plugin repository, add a bounded compatibility harness with explicit
  executable/package paths and SHA-256 pins, isolated homes/ports/databases,
  mock transports, managed browser checks, and cleanup of owned processes/files.
- Negative install/capability case on unchanged official v0.99.0; operation on
  the exact development extended host; older plugin compatibility; plugin
  disable/re-enable/crash/uninstall/reinstall; host/plugin restart and upgrade.
- Candidate migration and coherent host+plugin backup/restore/rollback drill;
  compare ownership, bindings, templates, provider/subscription/history and
  task/session inventories. Do not claim arbitrary host downgrade compatibility.
- Verify a latest-stable-plus-minimal-patch candidate as well as the main-based
  contribution. Re-resolve latest stable at execution, record exact source/digest,
  reject unsupported unchanged hosts, and test six-event behavior on the candidate.
- Prove captured immutable revisions survive concurrent Save/activation, backup
  quiescence, lost acknowledgements, plugin restart, and delayed format responses.
  Reject mixed backup manifests; restore all selected snapshots before binding.
- After actual publication, rerun the released-SDK/stable-host lane. Before
  then, leave that lane blocked and label development evidence accordingly.

## Out of scope

Production upgrade, SMTP test mail, merge, release/tag, marketplace submission,
or deleting any active review worktree/branch.

## Acceptance

1. Receipts identify immutable inputs and distinguish unsupported, passed,
   skipped, blocked, and uncertain outcomes; failure cannot be reported as green.
2. Actual plugin UI composition passes desktop/phone UI-01 workflows, cancellation,
   conflicts, preview-no-send, ownership, and lifecycle checks with screenshots.
3. Restart/upgrade/restore preserve configuration; rollback recovers the selected
   prior candidate without duplicate sends, cross-owner access, or data loss.

## Verification

Run from the independent plugin task worktree root. The harness/matrix below
are new work-order deliverables, not existing tools. The matrix must point only
to disposable hosts, synthetic fixture DBs, and explicit package artifacts. It
must not discover or attach to the operator's primary instance.

```bash
make verify-package-host
node scripts/verify-host-compatibility.mjs --matrix testdata/host-compatibility.json --output .build/compatibility
git diff --check
```

The harness includes rendered desktop/phone checks using the repository's pinned
browser dependency and records screenshots. Use one browser worker. Add complete
UI-01 scenarios without changing its composition; this task introduces no new
UI design. Run `make verify-package` for all manifest-declared platforms before
release, recording unexecuted native runtime coverage separately from cross-builds.

In the host worktree, run Task 01's relevant checks after any integration fixes
and the CONTRIBUTING.md pre-PR commands recorded in the plan. Record each result.
Do not auto-repeat uncertain external writes or invent aggregate passes.

## Files likely touched

Plugin `scripts/verify-host-compatibility.mjs`,
`testdata/host-compatibility.json`, packaged UI browser scenarios, compatibility
and recovery docs, and the two PR verification records. No core product changes
are planned here; route required contract changes back to Task 01 and the design.

## Dependencies

Tasks 01-03, exact candidate artifacts, and a disposable test environment.
The stable-host release lane also needs the actual supporting SDK publication.

## Risks

Mail delivery cannot promise exactly once after an uncertain transport outcome.
This work proves formatter invocation does not add sends, not SMTP exactly-once.

## Parallelism

Sequential after candidate artifacts and migration parity are available.

## Inputs

Plan compatibility table, UI-01, paired design, source/package pins, and private
snapshot rules. Preserve prior operational runtime and personal data.

## Results

The actual host-platform development package passed desktop and Pixel 5 editor
scenarios, all six event previews, whole-pair fallback, preparation without
activation, explicit activation, reload, and restore-on-Save. Screenshots and
artifact digest receipts remain in the plugin worktree's ignored
`.build/formatter-browser/` directory.

A live, isolated host passed bounded-lease backup capture, configuration-write
rejection while health remained available, later-edit six-event export with
four-event projection, and same-candidate host/plugin DB restore with exact
readback of three bindings, including the migrated provider. Native backup tests also reject changed files,
foreign ownership/installation, unsupported runtime configuration, and expired
capture leases. These results do not prove arbitrary host downgrade safety.

Latest stable was resolved as `v0.99.1`, source
`813a741b21108ea8ed10ab58954d85c2e5205999`. An unmodified stable-source binary
rejected the actual formatter archive with HTTP 400 before activation. The
minimal patched stable candidate `cf8579699e40e38dcb2063d1692c18444c8afbd5`
reports `v0.99.1+notification-formatter.1` and passed the same packaged editor,
six-event preview, four-template migration, and three-binding recovery drill.
The candidate uses the normal `fts5` backend target; protobuf regeneration is
clean, SDK race tests pass, and frontend typecheck/build pass. These are local
stable-source builds, not verification of official release archive bytes.

Remaining gates are the full lifecycle/authenticated ownership matrix, the
minimum-host v0.99.0 rejection case, released-SDK lane, and actual operational
runtime/session continuity. The earlier template v0.99.0 smoke is not formatter
compatibility evidence. Synthetic screenshots and provenance are committed in
the independent plugin repository under `docs/images/formatter/`.
