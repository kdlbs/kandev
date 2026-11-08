---
created: 2026-11-06
status: in_progress
requirements:
  - REQ-PLATFORM-NOTIFICATION-FORMATTER-001
  - REQ-PLATFORM-NOTIFICATION-FORMATTER-002
  - REQ-PLATFORM-NOTIFICATION-FORMATTER-003
  - REQ-PLATFORM-NOTIFICATION-FORMATTER-004
system_design:
  - ../../specs/platform/system-design/notification-formatter-plugins.md
legacy_specs: []
---

# Implementation plan: Apprise template plugin extraction

## Overview

Deliver a generic Kandev formatter PR and a separate community plugin PR.
The operator authorized the split and upstream proposal on 2026-11-06. The
[posted proposal](https://github.com/kdlbs/kandev/issues/5011#issuecomment-6682190312)
was read back after submission. The maintainer then
[approved the split](https://github.com/kdlbs/kandev/issues/5011#issuecomment-6683963471)
and requested two coordinated contributions with desktop/mobile screenshots.
The direction-discussion condition is satisfied. Exact SDK/API details remain
subject to PR review, not another preliminary approval gate.

Implementation is in progress on upstream main source
`5264536ba9845f966966c2a371c7e8136ccdf119`. The independent plugin's initial
implementation checkpoint is `ddf7fbfd76fcd8eb3bcbfeba3bf21e5c75f964e1`.
Latest stable was rechecked on 2026-11-07: `v0.99.1`, peeled source
`813a741b21108ea8ed10ab58954d85c2e5205999`. Work-order results distinguish
development evidence, remaining matrix checks, and production authorization.

### Confirmed grilling decisions, Q1-Q8

1. Core contribution targets freshly fetched upstream main; latest stable plus
   the minimal patch is also an isolated compatibility target.
2. Create public community repository
   `illuwa-soft/kandev-plugin-apprise-templates` from the official template.
   Canonical checkout: `<workspace>/kandev-plugin-apprise-templates`;
   task worktrees stay under its `.worktrees/`. Core PR targets
   `kdlbs/kandev:main` from the Illuwa fork; plugin PR targets its own `main`.
3. Support all six current events, including permission requests and session
   failures. Import only existing overrides; never subscribe additional events.
4. Preparation/import/preview is available before binding; first activation
   explicitly selects an exact verified configuration revision.
5. Capture host DB/config and plugin DB with a version/digest manifest while
   configuration writes are quiesced. Do not stop task agents solely for capture.
6. Rollback exports later edits, restores compatible legacy entries, and retains
   unsupported event settings in export. Unsupported downgrade stops safely.
7. Normal Save prepares an immutable revision and conditionally activates it.
   Failed activation retains the old revision and shows saved-but-not-applied;
   reconcile uncertain responses. Restore follows Save; Reset discards drafts.
8. After verification, upgrade operational Kandev to the latest stable at that
   time and verify functionality. Prefer official output when it supports the
   extension; the operator also permits a minimal patched stable build. Do not
   substitute main/nightly or mislabel it. Failed compatibility/restore blocks
   cutover.

These decisions authorize repository/PR delivery and the guarded operational
upgrade, not upstream merge, public release/tag, or destructive cleanup. Grilling
is complete; do not reopen settled questions without contradictory evidence.

## Scope

### In scope

- Generic formatter declaration, optional SDK RPC, provider binding and owner
  scope, policy-preserving dispatch, generic native editor mount, public docs.
- Separate template plugin bootstrap, product storage/renderer/editor, synthetic
  preview, operator legacy import, and host-platform package verification.
- Cross-version integration, old/new parity, restart, upgrade, backup/restore,
  and rollback evidence with mock transports and synthetic fixtures.
- Guarded operational latest-stable upgrade, migration/activation, and
  post-upgrade verification after candidate acceptance (Task 05).

### Out of scope

- Upstream merge, unrelated deployment, unrequested test mail, release/tag, marketplace submission,
  destructive legacy cleanup, and replacing any unrelated plugin.
- Claiming an unmodified v0.99.0 supports the proposed formatter contract.

## Technical approach

Use the [system design](../../specs/platform/system-design/notification-formatter-plugins.md)
and [boundary ADR](../../decisions/2026-11-06-notification-formatter-plugin-boundary.md).
Host product-neutral changes stay in Kandev. Plugin code goes into a dedicated
repository bootstrapped from the official template. Proposed repository identity
is `illuwa-soft/kandev-plugin-apprise-templates`; verify availability and exact
template commit at bootstrap. Do not create an official `kdlbs` repository.

With the split direction approved, Tasks 01 and 02 can be developed/reviewed
against the same immutable schema snapshot. They are not dependency-free:
plugin integration waits for host behavior and SDK glue. Execute sequentially
in this primary session unless the operator separately authorizes delegation.
Task 03 follows both; Task 04 closes cross-repository acceptance; Task 05 performs
the authorized operational transition after acceptance.

| Target | Intended result | Evidence/fallback |
| --- | --- | --- |
| Existing plugins on extended host | Unchanged base interface/wire behavior | Existing SDK/host tests plus an old fixture binary |
| New plugin on official v0.99.0 | Unsupported, cannot bind | Install/capability negative case; no private-API bypass |
| Latest stable without formatter extension | Unsupported unchanged; minimal patched candidate required | Re-resolve latest stable, pin artifacts, prove the patch before operation |
| Apprise on development extended host | Custom or default pair; original sender | Synthetic package smoke and service assertions |
| Local/System/Web Push/Webhook | No first-plugin selection or content change | Non-selected-provider regression cases |
| Plugin unavailable/invalid/slow | Whole default pair, one send | Deadline/cancellation/late-result tests |
| Runtime summary | Format already-claimed members only | Member-claim and transport-failure parity |
| First supporting stable host/SDK | Release-compatible plugin | Pending actual publication; do not guess version |

## ASCII UI preview

### UI-01: Notifications settings, selected Apprise provider

Proposed shared desktop/phone composition. Provider ownership, binding status,
separate subscription status, staged edits, and action order are structural;
spacing and English labels are illustrative and require localization.

```text
Notifications > Provider
Formatter [Default / Apprise templates v]
Status: Ready / Unsupported host / Plugin unavailable

Event [Agent needs an answer v]   Subscription: On
Template: Custom
Title [________________________________]
Body  [________________________________]
      [________________________________]
Variables [task.title] [session.url] [more]
Sample [Normal v] [Preview] [Restore default]
Preview (synthetic data)
+--------------------------------------+
| Title and body as plain text          |
+--------------------------------------+
Status/error: active / saved but not applied / conflict.
First import: [Prepare] [Preview] [Activate prepared revision]

[Reset] [Save]
```

Phone uses the same single-column order, wraps event/status and sample/action
rows, uses 44px coarse-pointer targets, and does not scroll horizontally. Shared
Save/Reset stays in its existing page-owned location. Loading retains drafts;
unsupported status disables binding, not ordinary provider settings. First import
has separate activation; ordinary edits use shared Save once. Test UI-01 against
AC-PLATFORM-NOTIFICATION-FORMATTER-002.1 through .6.

## Tests

New test names below are implementation targets, not existing or passed tests.
Paths under `server/` and `ui/` belong to the independent plugin repository.

| Acceptance | Planned evidence |
| --- | --- |
| 001.1-.4 | Host `service/formatter_test.go`: `TestFormatterAdmission`, `TestFormatterWholePairFallback`, `TestFormatterCancellation`, `TestFormatterRuntimeClaims`; optional SDK compatibility tests |
| 002.1 | Host `plugins/notification_formatter_actions_test.go`: `TestFormatterProviderOwnership`, `TestFormatterInstallationFence` |
| 002.2-.4 | Plugin `server/templates_test.go`, `server/preview_test.go`, `ui/template-editor.test.tsx`; UI-01 E2E |
| 002.5-.6 | Binding CAS/uncertain readback, immutable snapshots, staged import, and saved-but-not-applied UI tests |
| 003.1 | Plugin `server/import_test.go`: `TestImportAtomicity`, `TestImportIdempotency`, `TestImportConflict`; offline export fixtures |
| 003.2 | Plugin `server/parity_test.go`: `TestLegacyRenderingParity`; fixture oracle derived from pinned fork |
| 003.3-.4 | Plugin migration/compatibility harness: snapshots, readback, lifecycle, rollback |
| 003.5-.6 | Capture gate, backup integrity, reverse legacy export, unsupported downgrade/event tests |
| 004.1-.3 | Host SDK/manifest/action tests; plugin `make sdk-check`, package verifier, compatibility harness and safe-output assertions |

All abbreviated acceptance IDs above use prefix
`AC-PLATFORM-NOTIFICATION-FORMATTER-`. Detailed work-order frontmatter lists exact IDs.

## E2E tests

Task 01 adds host fixture coverage in proposed
`apps/web/e2e/tests/settings/notification-formatters.spec.ts` and
`mobile-notification-formatters.spec.ts`. Exercise owner isolation, selector,
default fallback, staged Save/Reset/conflicts, missing plugin, and lifecycle cleanup
with host-owned fixture UI. Use the repository-managed Playwright runner with
one worker. Task 02's plugin UI tests cover actual product editing; Task 04's
packaged browser smoke covers actual plugin/host composition on desktop and phone.
Mock delivery only; capture synthetic screenshots for the eventual PRs.

## Work orders

- [ ] [Task 01: Generic host formatter contract](task-01-host-contract.md)
- [ ] [Task 02: Independent template plugin](task-02-template-plugin.md)
- [ ] [Task 03: Legacy import and rendering parity](task-03-legacy-import.md)
- [ ] [Task 04: Packaged upgrade and recovery evidence](task-04-compatibility.md)
- [x] [Task 05: Operational latest-stable upgrade](task-05-operational-upgrade.md)

## Verification strategy

Each work order states its working directory and exact commands. Proposed new
harnesses must be implemented before their command counts as executable evidence.
Run targeted tests first. Before eventual host contribution, also satisfy
CONTRIBUTING.md's `make fmt`, `make typecheck test lint`, and `make test-e2e`,
recording unavailable environment/platform coverage rather than claiming success.
No PR is ready from document lint alone. Public plugin release depends on the
actual host and independently published SDK pair, not merged host source alone.

## Verification results

Design validation is recorded in [validation](validation.md). Work-order results
record the implemented host/plugin contract, exact-candidate package/browser
checks, migration/recovery drills, and the completed 2026-11-07 operational
upgrade. Task 05 records preserved data/settings and post-restart readback;
Task 04 retains the broader matrix and published-SDK limitations. An operational
private build does not establish public SDK support or an upstream merge.

The independent shared-save correction for PR #5141 is recorded in Task 01.
Pending first activation retains the selection without cancelling unrelated
provider or notification-sound saves.

## Risks

- A one-second renderer RPC can add latency on the notification path; enforce
  the deadline and preserve default delivery/cancellation under failure.
- Management scope and installation fencing are part of the host change;
  implementing only rendering leaves an unsafe or unusable editor.
- Plugin SQLite is not covered by a host DB backup; coordinated snapshots and
  explicit export are required before uninstall or rollback.
- A consistent source may contain more than the previously observed two
  overrides. Refresh counts privately; do not expose real content in fixtures.
- Maintainers approved the split direction; protocol names/shape can change
  during PR review. Reconcile implementation and package when they do.
