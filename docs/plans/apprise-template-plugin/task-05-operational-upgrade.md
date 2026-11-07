---
id: 05-operational-upgrade
title: Operational latest-stable upgrade
status: pending
wave: 4
depends_on:
  - 04-compatibility
plan: plan.md
requirements:
  - REQ-PLATFORM-NOTIFICATION-FORMATTER-001
  - REQ-PLATFORM-NOTIFICATION-FORMATTER-002
  - REQ-PLATFORM-NOTIFICATION-FORMATTER-003
  - REQ-PLATFORM-NOTIFICATION-FORMATTER-004
acceptance_criteria:
  - AC-PLATFORM-NOTIFICATION-FORMATTER-001.1
  - AC-PLATFORM-NOTIFICATION-FORMATTER-001.2
  - AC-PLATFORM-NOTIFICATION-FORMATTER-002.5
  - AC-PLATFORM-NOTIFICATION-FORMATTER-002.6
  - AC-PLATFORM-NOTIFICATION-FORMATTER-003.1
  - AC-PLATFORM-NOTIFICATION-FORMATTER-003.3
  - AC-PLATFORM-NOTIFICATION-FORMATTER-003.4
  - AC-PLATFORM-NOTIFICATION-FORMATTER-003.5
  - AC-PLATFORM-NOTIFICATION-FORMATTER-003.6
  - AC-PLATFORM-NOTIFICATION-FORMATTER-004.2
  - AC-PLATFORM-NOTIFICATION-FORMATTER-004.3
system_design:
  - ../../specs/platform/system-design/notification-formatter-plugins.md
---

# Task 05: Operational latest-stable upgrade

## Summary

After candidate acceptance, perform the operator-authorized Kandev upgrade and
plugin configuration transition on the Mac host. Use the latest stable at that
time, with the minimal formatter patch when required, and verify real runtime
behavior without describing patched artifacts as official distribution output.

## In scope

- Re-read the deployed launcher/service contract, current runtime/plugins,
  settings, ongoing sessions, and actual latest stable release. Record safe
  count/digest-only inventories. Confirm existing fork-local functionality has
  a supported replacement in the candidate; do not silently drop it.
- Select official stable output if the required extension is present. Otherwise
  use the minimal patched stable candidate already tested by Task 04. If stable
  changed after Task 04, rerun its affected compatibility/restore checks for the
  new exact candidate. Record source/patch/SDK/plugin pins and package digests.
- Capture a private coherent host DB/config/launcher plus plugin-data backup set,
  reopen/validate copies, freeze only template/binding mutations for capture, and
  preserve the old runtime. Never use an ordinary live DB file copy as a backup.
- Prepare/import and verify exact template counts/content privately before
  activation. Upgrade through the verified service mechanism with session
  recovery safeguards; select the prepared revision once. Avoid legacy plus
  plugin double formatting. Read back active revision and saved/applied state.
- Verify health/readiness/version, existing task/session continuity, provider
  credentials/subscriptions, six-event synthetic previews, Save/Reset/restore,
  fallback, plugin restart, and actual desktop/phone editor behavior. Distinguish
  synthetic previews and mock send evidence from real recipient delivery.
- If verification fails, quiesce config writes, export new edits, and execute the
  tested compatible rollback. Preserve unsupported additional event settings in
  export; stop unsafe schema downgrade and report any uncertain state.

## Out of scope

Upstream merge, public tags/releases, marketplace submission, unrelated runtime
changes, destructive cleanup, and artificial notification sends to real recipients
without explicit send direction. Ordinary already-subscribed events retain their
normal production behavior after the authorized activation.

## Acceptance

1. The installed host derives from the latest stable selected at execution and
   has passed exact-candidate verification. Readback identifies official versus
   patched output, plugin revision, and active template snapshot.
2. Before/after inventories prove existing configuration and session continuity;
   templates are imported and activated once, while desktop/phone operations and
   non-sending previews work on the real upgraded runtime.
3. A coherent validated recovery set and later-edit export remain available;
   any failed transition follows the tested rollback rather than guessing at
   compatibility or retrying an uncertain mutation blindly.

## Verification

The controlled local rollout harness below is a proposed Task 05 deliverable in
the independent plugin repository, not an installed tool. Build it around the
existing verified backup/service mechanisms rather than inventing a new service
manager. Test preflight/apply/verify/rollback against fake service/fixture homes
before allowing explicit production mode. The external private plan identifies
the exact host/home/service, backup set, and package pins; it contains references
to existing secret storage, never credential values in CLI arguments or reports.

Run from the independent plugin worktree root. `KANDEV_ROLLOUT_PLAN` must point
to that reviewed private plan. Preflight stops on a changed candidate, unsafe
schema, unverified recovery, or non-matching service; apply is never inferred
from a successful preflight. Do not execute apply until Task 04 passes.

```bash
node --test scripts/operational-upgrade.test.mjs
node scripts/operational-upgrade.mjs --phase preflight --plan "${KANDEV_ROLLOUT_PLAN:?set the reviewed private rollout plan}"
node scripts/operational-upgrade.mjs --phase apply --plan "${KANDEV_ROLLOUT_PLAN:?set the reviewed private rollout plan}"
node scripts/operational-upgrade.mjs --phase verify --plan "${KANDEV_ROLLOUT_PLAN:?set the reviewed private rollout plan}"
git diff --check
```

On a failed applied transition, use the separately tested rollback phase with
the same plan/run identity, after capturing later edits. Do not rerun apply with
a new identity after an uncertain result. Retain safe receipts and manual
desktop/phone evidence; omit credentials and personal template content.

## Files likely touched

Plugin `scripts/operational-upgrade.mjs`, `scripts/operational-upgrade.test.mjs`,
operator runbook, and private external rollout plan/receipts. The explicitly
authorized service launcher, runtime/plugin artifact installation, host DB/config,
and plugin data change only during the guarded apply phase. They never enter
either repository's commits or public PR attachments.

## Dependencies

Task 04 and the Q8 operational authorization recorded in the plan. No additional
maintainer approval is needed to implement the accepted direction. Exact release
artifacts, supported active-session recovery, and coherent restore remain checks,
not assumptions satisfied by that authorization.

## Risks

A newer stable may remove a fork-local feature or require new migrations. Stop
on missing preservation evidence. A verified config backup does not prove live
session recovery; both need candidate evidence before service replacement.

## Parallelism

Sequential after isolated verification. One owner controls the transition.

## Inputs

Paired design, Tasks 03-04 receipts, current service contract, immutable candidate
artifacts, operator-approved Q1-Q8, and private coherent recovery set.

## Results

Pending exact-candidate compatibility and recovery acceptance. No production
runtime or settings have changed. The latest stable source has been resolved
for candidate preparation, but this is not an applied operational upgrade.
