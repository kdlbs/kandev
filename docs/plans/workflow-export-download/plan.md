---
created: 2026-10-28
status: implemented
requirements:
  - REQ-UI-WORKFLOW-EXPORT-DOWNLOAD-001
system_design:
  - ../../specs/ui/system-design/workflow-export-download.md
legacy_specs: []
---

# Implementation Plan: Download Workflow Export Files

## Overview

One sequential work order adds a local download to the shared export dialog,
proves both real entry points on desktop and phone, and updates the short public
how-to. UI owns the local presentation; Tasks retains workflow data and export.
The qualification and complete open-PR inventory are recorded under
`/home/jcfs/.kandev/loop-evidence/61dadc35-4a42-4581-8c94-62309c160fd9/discovery046/`.
The request settles filename, exact content, failure, mobile and scope; no
material choice remains unresolved.

## Scope

### In scope

- Visible localized Download in the existing single/Export All dialog,
  exact current text, empty/whitespace disable and local retryable failure.
- Extend the existing mounted consumer suite; focused disposable desktop and
  mobile E2E, geometry and synthetic captures; concise Quick path/Use the UI
  edits in `docs/public/workflow-import-export.md` around lines 13-28.

### Out of scope

- Changes to export format, selection, import, profiles, workflow saves, backend,
  Git, sessions, download helper, clipboard hook, or adjacent navigation.
- Production or permanent test changes, installs, checks, builds, hooks, commits,
  pushes, PRs or observers during this design turn; no delegation or sessions.

## Technical approach

`WorkflowExportDialog` receives the current export `content` for both paths.
Use unchanged `triggerFileDownload` with `workflows.yaml`, raw `content`, and
`isBinary: false`; trim only to disable. Reuse `task:download` and default toast
`task:failedToDownloadFile`; no telemetry variant or new locale catalogs. A
small separate button is optional; do not refactor shared helpers or hooks.
Reuse settings action density and independently enforce phone/coarse target
size. Preserve Copy/Close, the preview, dialog primitive and scroll owners.

## ASCII UI preview

**UI-01: Workflow export dialog**, reached by either single Export or Export All;
nonempty text, criteria `.1`-`.5`. Action order and grouping are structural;
spacing is illustrative. The preview retains its existing internal scrolling.

```text
Desktop: +---------------- Workflow export ----------------+
         | current YAML preview (read only, scrolls)       |
         |                      [Close] [Download] [Copy]  |
         +-------------------------------------------------+

Phone:   +--------- Workflow export ---------+
         | current YAML preview              |
         | (read only, scrolls)              |
         | [Copy]                            |
         | [Download]                        |
         | [Close]                           |
         +-----------------------------------+
```

Phone controls may wrap instead of stacking if fully contained; they remain
visible, accessible and at least 44 px. Whitespace disables Download; a failed
initiation displays localized feedback and leaves the dialog retryable. Actual
rendered geometry, keyboard/touch activation and capture are required.

## Tests and delivery

First author a mounted consumer test and record a causal RED for the absent
Download action before production work. Extend the current dialog suite without
rewriting existing fixtures or clipboard assertions: current raw Unicode and
newlines, rerender, fixed helper arguments, empty/whitespace, actual thrown
failure/default toast/retry, and unchanged Copy/Close. Dedicated isolated
`workflow-export-download.spec.ts` and `mobile-workflow-export-download.spec.ts`
use real entry points, actual download event and file bytes/name, cover both
single and All across bounded desktop/mobile scenarios, keyboard/touch,
target/containment and narrow fine-pointer phone; capture synthetic data only.
Inspect describe overrides and enforce one worker, zero retries. Do not run
broad suites or touch live data. Work-order commands specify the sequential
same-worktree build and targeted verification.

## Dependencies and risks

[Task 01](task-01-workflow-export-download.md) is the only sequential work
order. ROOT must fully read all four documents and send a later separate
execution request plus exclusive global HEAVY release; this checkpoint is not
that release. Preserve concurrent work. Before publication refresh current main
and the complete paginated open-PR file inventory against every actually owned
path; reconcile actual overlap. Qualification found 139 PRs, 25 paginated lists,
no source/test/new-doc overlaps, and three public-guide-only patches 4922, 4714,
3440 fully read by ROOT at exact heads and disjoint from the short UI section.
Re-read current guide and preserve all other text, including protocol, security,
and profile sections. Screenshot runs clear `.pr-assets`: preserve owned
originals, manifest and hashes externally before the next managed run.

## Work orders

1. [Task 01: Download workflow export](task-01-workflow-export-download.md) (done; no dependencies).
