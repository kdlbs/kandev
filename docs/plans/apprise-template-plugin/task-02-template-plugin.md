---
id: 02-template-plugin
title: Independent template plugin
status: in_progress
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-PLATFORM-NOTIFICATION-FORMATTER-001
  - REQ-PLATFORM-NOTIFICATION-FORMATTER-002
  - REQ-PLATFORM-NOTIFICATION-FORMATTER-004
acceptance_criteria:
  - AC-PLATFORM-NOTIFICATION-FORMATTER-001.3
  - AC-PLATFORM-NOTIFICATION-FORMATTER-001.4
  - AC-PLATFORM-NOTIFICATION-FORMATTER-002.1
  - AC-PLATFORM-NOTIFICATION-FORMATTER-002.2
  - AC-PLATFORM-NOTIFICATION-FORMATTER-002.3
  - AC-PLATFORM-NOTIFICATION-FORMATTER-002.4
  - AC-PLATFORM-NOTIFICATION-FORMATTER-002.5
  - AC-PLATFORM-NOTIFICATION-FORMATTER-002.6
  - AC-PLATFORM-NOTIFICATION-FORMATTER-004.1
  - AC-PLATFORM-NOTIFICATION-FORMATTER-004.3
system_design:
  - ../../specs/platform/system-design/notification-formatter-plugins.md
---

# Task 02: Independent template plugin

## Summary

Bootstrap a dedicated community repository from the official plugin template
and implement the formatter product there. Use the shared contract snapshot for
unit development while Task 01 builds the host; packaged integration follows it.

## In scope

- Verify proposed repository identity `illuwa-soft/kandev-plugin-apprise-templates`
  and template revision, read local instructions, isolate a task worktree, and
  preserve template test/package/release safeguards. Copy this work order or link
  its exact source revision into the plugin's own delivery record.
- Pure renderer/catalog/destinations, transactional SQLite revisions, provider-
  bound management, synthetic preview, host-owned native UI, translations.
- Support all six events, including the two additional keys. Store complete immutable
  event/origin snapshots. A requested old revision never resolves to the latest
  row; preparation and rendering cannot activate or change snapshots implicitly.
- One Save prepares then activates through core CAS; display saved-but-not-applied
  and uncertain state, retain later drafts, and use the same path for restoration.
- Fail-safe initialization/destroy, missing-host handling, safe diagnostics,
  unit tests, UI tests, build, and host-platform package verification.

## Out of scope

Host implementation, transport/SMTP credentials or sends, migration tooling
(Task 03), production activation, release, and marketplace entry.

## Acceptance

1. The dedicated repository contains all product code, no host `internal/...`
   imports, no private endpoints or live host SQL, and no notification sender.
2. Owner-bound revisioned edits, preview, restore, shared Save/Reset, stale
   response fencing, and unsupported-host behavior pass product tests.
3. The package preserves template safeguards and contains declared executable/UI
   assets and generated checksums; SQLite choice supports the declared builds.

## ASCII UI preview

UI-01 product excerpt from [the plan](plan.md#ascii-ui-preview), covering
AC-PLATFORM-NOTIFICATION-FORMATTER-002.2 through .4:

```text
Event [Agent needs an answer v]   Subscription: On
Template: Custom
Title [________________________________]
Body  [________________________________]
Variables [task.title] [session.url] [more]
Sample [Normal v] [Preview] [Restore default]
Preview (synthetic data): title/body, fallback status
Status/error: conflict. Reload to resolve.
Saved but not applied: [Reconcile / retry activation]
First import: [Prepare] [Preview] [Activate prepared revision]
[Reset] [Save]
```

Same single-column phone order; wrap status/actions, use 44px targets, retain
drafts on loading/errors, render preview as text, and localize labels.

## Verification

Run from the independent plugin task worktree root after bootstrap. The official
template Makefile was inspected on 2026-11-06 and defines the make targets below;
verify the pinned template still does. Keep package tooling at its separately
pinned checkout. Until the supporting SDK is published, record exact development
source pins; a published-SDK check is blocked, not replaced by a fabricated version.

```bash
npm ci
make check-format
go test -trimpath -race ./server/... -count=1
make test
make vet
make build
make verify-package-host
git diff --check
```

Before release, pin the actual published Go/npm SDK pair, remove development
replacements, and run `make sdk-check` and `make verify-package` with the
published SDK-wide host floor. Template build/package targets invoke sdk-check;
record these as blocked until the selected dependency source is usable.

## Files likely touched

In the independent repository: `manifest.yaml`, `go.mod`, lockfiles, `Makefile`,
`server/`, `ui/`, template scripts, README and delivery docs. Product test targets
include `server/templates_test.go`, `server/preview_test.go`, and
`ui/template-editor.test.tsx`; preserve the template's working UI test runner.

## Dependencies

Maintainer direction approval is satisfied. Use the frozen contract shared with Task 01.
No completed host implementation is needed for pure/product unit development;
real-package integration requires Task 01. Repository creation follows the operator's
authorized two-PR direction; do not change organization policies or permissions.

## Risks

Plugin-owned SQLite must be portable and backed up separately. Do not assume a
host DB backup covers it or Host state provides compare-and-swap semantics.

## Parallelism

May overlap host development against the same contract. Execute in the primary
session unless separately authorized to delegate.

## Inputs

Paired design, pinned legacy renderer/catalog/URL code, inspected official plugin
template, and the Task 01 contract snapshot. Legacy text is evidence, not an
importable host library.

## Results

Created `illuwa-soft/kandev-plugin-apprise-templates` and committed the initial
development implementation as `ddf7fbfd76fcd8eb3bcbfeba3bf21e5c75f964e1`.
The product supports six closed event catalogs, literal rendering, immutable
SQLite configurations, owner-bound import/export, native translated editor,
shared Save/Reset, explicit initial activation, and synthetic previews.

Go race tests, 16 UI tests, five native backup/capture tests, 14 retained recipe
tests, recipe typecheck, format/vet, package verifier negatives, and audit passed
in the development lane. The actual package passed desktop/phone workflows;
Task 04 owns the exact artifact receipts and recovery matrix.

Public Go/npm SDK `0.1.0` does not implement the formatter extension. Its
SDK-wide host floor is not support evidence. The source-labeled private lane
works; standalone released-dependency compatibility remains blocked until a
supporting SDK is published. No public plugin release is claimed.
