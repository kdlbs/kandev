---
id: 05-operational-upgrade
title: Operational latest-stable upgrade
status: completed
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

The controlled local rollout harness below remains a proposed deliverable in
the independent plugin repository, not an installed tool. Build it around the
existing verified backup/service mechanisms rather than inventing a new service
manager. Test preflight/apply/verify/rollback against fake service/fixture homes
before allowing explicit production mode. The external private plan identifies
the exact host/home/service, backup set, and package pins; it contains references
to existing secret storage, never credential values in CLI arguments or reports.

These proposed commands were not implemented or executed during the operational
rollout. The observed run used private, ignored operator scripts with exclusive
phase receipts, immutable artifact pins, generation-checked signalling, and
authoritative readback. The commands below describe the proposed reusable tool,
not the evidence for this deployment.

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

Completed the authorized operational transition on 2026-11-07. Latest stable was
rechecked as `v0.99.1`; the installed private candidate derives from stable source
`813a741b21108ea8ed10ab58954d85c2e5205999` plus the formatter extension at
`cf8579699e40e38dcb2063d1692c18444c8afbd5`. It reports
`v0.99.1+notification-formatter.1`, not an unmodified official distribution.

- Host SHA-256: `4c1efbaa687595ff204889c056c62cb092bee450cf97e3bdaec5c077da5c7283`.
- Plugin archive SHA-256: `2b7869d321d9e776c80392d18d4b0db39e1200f7e73b39c901ee157b1cbf94ac`.
- The complete paired runtime bundle includes four remote helpers and the tested
  frontend. Existing automation-history reads are present in the stable source.
- A pinned-read SQLite backup passed integrity validation. Launcher, service,
  configuration, encryption key, and all ten existing plugins were captured
  privately; plugin SQLite used the native backup API rather than a live copy.
- The actual operating DB copy passed candidate migration with all 1,327 task
  IDs, 1,327 session IDs, and two legacy templates retained. The complete staged
  bundle also passed actual-template import, explicit activation, and previews
  in the isolated copy. External networking and writes were sandbox-restricted.
- The old binary booted an original-schema snapshot copy successfully. This
  verifies that recovery boundary, not an in-place downgrade of migrated data.
- A fresh cutover snapshot and unchanged configuration digests were checked
  before atomically switching the launcher. The user-owned launchd process was
  signalled once after identity verification; KeepAlive started the new bundle.
  No privileged service reconfiguration was required.
- One provider received the two existing overrides through a prepared import,
  followed by one explicit binding activation and exact revision/digest readback.
  Six non-sending previews passed: two custom pairs and four default fallbacks.
  No additional event subscriptions or artificial real-recipient sends occurred.
- Final readback retained 1,327 tasks and sessions, the same session-state counts
  (1,309 cancelled, one failed, 17 waiting for input), three providers, nine
  subscriptions, and the unchanged legacy configuration digests. All ten existing
  plugins and the new formatter plugin are active. `agentSurvival=true` remains
  enabled; there were no running/starting sessions at the restart boundary, so
  this does not claim live in-flight agent survival coverage.
- Desktop and Pixel 5 checks on the upgraded instance matched the saved title
  and body exactly, exercised custom synthetic preview, reported no horizontal
  overflow or page errors, and left persistent UI settings unchanged. Save,
  Reset, and restore mutation checks were performed on disposable exact-candidate
  hosts rather than rewriting the operator's templates for a test.
- A subsequent operational host/plugin restart preserved the exact active
  binding and all 11 active plugins. Local and external health responses report
  the new private version after that restart.
- A post-upgrade bounded configuration lease captured and validated the full
  host/plugin recovery set: eight files, one binding, and
  `installation_checked=true`. Manifest SHA-256:
  `dd350bf706bbb686794e96cc2ca47fab80baef75489108955ded785f683b5e1a`.
  The busy operating DB required an explicitly pinned read transaction in the
  private capture path; the default public snapshot helper's live-write behavior
  is not claimed as validated by this run.

Private receipts and screenshots remain with the operator's recovery set and
are not public PR attachments. Public SDK publication, upstream merge, and the
broader matrix limitations recorded in Task 04 remain separate from this
successful operational deployment. No destructive legacy cleanup was performed.

### Review follow-up deployment

The PR #5141 fixes in `21a19ace7ca288962ad049aa7684d9893203ab61` were backported
onto the same stable base as `c846cf1027dc5e4ff08aee2281a8ef4d0421e418` and
deployed as private build `v0.99.1+notification-formatter.2`. Host SHA-256:
`197887ef12698a72ad0fa22a59d88d60972b6eaa10ed462473c7f241151e8c3d`.
The installed plugin archive and active configuration revision were retained.

A fresh pre-transition capture, rather than the earlier rollout snapshot, was
used as the preservation baseline. Task/session IDs remained at 1,327 each;
three providers, 11 subscriptions, two legacy overrides, one active binding,
and all 11 active plugins were preserved. The newer subscription and session
state changes were not reverted to the earlier rollout's values. Both private
recovery sets validated eight files and their installation/binding references;
post-transition manifest SHA-256:
`7e347b7a5ec5ccf11b8cd95a66eec916618dd7c34a9d8e8fed49238168e3f783`.

The exact candidate passed packaged desktop/phone checks, synthetic migration
and recovery, and formatter-focused stable race tests before replacement. The
upgraded instance then passed six non-sending previews, exact active-binding
and configuration-digest readback, desktop/Pixel 5 saved-template checks, and
external health verification. No reimport, template activation, subscription
write, or artificial real-recipient send was required for this patch update.

### Shared-save frontend correction

On 2026-11-08, host review fix `9503984b4` was backported onto the same private
stable candidate as `4e43977e82c52002eadf68c7e4e7ba2519c51cb2`. The candidate's
frontend passed typecheck/build and 35 formatter/coordinator tests. The actual
installed plugin package passed the disposable desktop/Pixel 5 editor, six-event
preview, migration, and three-binding recovery harness with that frontend.
The repository Playwright runner was blocked by a missing delivery fixture;
building the unrelated Go fixture exceeded 240 seconds. Its updated shared-save
scenario is not claimed as executed by that runner.

The operational change was a frontend-only atomic overlay, served through the
existing `KANDEV_WEB_DIST_DIR`. The backend process and launcher remained
unchanged; the host still reports `v0.99.1+notification-formatter.2`. Provider
configuration and bindings matched their pre-transition digest, and the installed
plugin archive remained unchanged. New shell entrypoint SHA-256:
`23a4680f4e27c04f39b155607d63cc8b14fab5f3a6dff426c41dcbeb1e29dad3`.
HTTP readback verified the new shell and all 86 entry assets, while 653 previous
content-addressed assets remain available to existing tabs. The original web
directory and a complete overlay receipt are retained privately for rollback.
