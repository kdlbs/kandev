---
created: 2026-10-02
status: implemented
requirements:
  - REQ-AGENTS-RUNTIME-NOTIFY-001
system_design:
  - ../../specs/agents/system-design/runtime-update-notifications.md
legacy_specs: []
---

# Fix Plan: Managed Fallback Current Version After Repair

## Overview

When OpenCode is installed natively on the host, **Update OpenCode** manages
the fallback npm package used by remote and container launches. After a
successful repair, closing and reopening the dialog always shows
`Unknown → <latest>` with **Repair runtime**. This happens even though
**Active version** shows the validated selection. Derive the fallback's
current version from that validated selection so the dialog classifies
against it (AC-AGENTS-RUNTIME-NOTIFY-001.8).

## Confirmed root cause

`previewAgentUpdate` in
`apps/backend/internal/agent/settings/controller/agent_update.go` and the job
classification in `agent_update_job.go` both blank `current` when the agent is
in managed-fallback mode. They do this because the host capability observation
belongs to the native binary. `runExactCandidate` sets
`job.CurrentVersion = caps.AgentVersion` from the candidate probe, so while the
finished job is still in the dialog, the version looks right. A fresh preview
recomputes `current = ""`. `ClassifyEffectiveOperation` then returns `repair`
for every target, and the UI renders the localized **Unknown** placeholder.
Nothing ever records the validated fallback version as current.

## Scope

### In scope

- Backend: in managed-fallback mode, use the persisted active selection as the
  fallback's current version in `previewAgentUpdate` and in the job's
  pre-run classification. Use one shared helper so the two paths cannot
  diverge.
- Update the existing fallback controller test, which currently asserts an
  empty preview current version while a selection exists. Add a regression for
  the reopen sequence: repair, job gone, then a fresh preview.

### Out of scope

- The installed-agent catalogue (`buildRuntimeUpdateDTO`) and runtime
  update-status projections keep their native-host semantics.
- With no selection (Kandev default, never validated), the fallback stays
  unknown with **Repair runtime**. This was confirmed with the user on
  2026-10-02.
- Frontend code, copy, locale catalogs, and layout. The dialog already renders
  `current_version` and the backend operation.
- Native host capability publication and non-fallback runtimes.

## Technical approach

Add a small controller helper that returns the managed current version:

- not fallback: the host capability `AgentVersion` (unchanged);
- fallback: the active selection version, or `""` when absent.

`previewAgentUpdate` already reads `active` through `runtimeVersions`. Pass it
to the helper after that read, before catalogue resolution, so `extras` and
classification see the derived value. In `AgentUpdateJobStore.run`, compute
`currentVersion` after the selection read, set `job.CurrentVersion` from the
same rule, and classify with it. With current equal to effective, the
classification yields `update`, `rollback`, or `up_to_date`. Selecting the
already-validated fallback version becomes `up_to_date` and starts no job
through the existing `finishAlreadyUpToDate` path.

## ASCII UI preview

### UI-01: Reopened fallback dialog after a successful repair (desktop and phone)

The desktop dialog and the phone drawer share `UpdateBody`, and composition is
unchanged. Only the values change.

Before (source and screenshot verified):

```text
Update OpenCode
Repair runtime
Unknown → 1.18.34
Active version: 1.18.32   Effective version: 1.18.32   Kandev default: 1.18.32
...                                         [Cancel] [Repair runtime]
```

After:

```text
Update OpenCode
Update runtime
1.18.32 → 1.18.34
Active version: 1.18.32   Effective version: 1.18.32   Kandev default: 1.18.32
...                                         [Cancel] [Update runtime]
```

If the operator selects `1.18.32`, the dialog shows `1.18.32` once with
**Up to date**, and the approval is disabled. These states already exist for
non-fallback runtimes.

## Work orders

- [Task 01: Report validated fallback version as current](task-01-fallback-current-version.md): done
- [Task 02: Record the validated fallback version, including the default](task-02-record-validated-default.md): done

Execution is sequential in the primary conversation. No subagents are
authorized.

## Follow-up from user testing (2026-10-06)

Testing Task 01 showed a gap. After **Use Kandev default** succeeds, the dialog
returns to `Unknown` with **Repair runtime**. Return-to-default validates the
default before it deletes the selection, so this version is known. Task 01
treated "no selection" as "never validated", which is wrong in this case. The
user confirmed the rule: Unknown only when no successful activation has
validated the version that future fallback launches use.

Task 02 persists a validation record on every successful fallback activation,
in the same settings namespace as the selection
(`managed_runtime.validated.<agent>`), and derives the current version from it
while it equals the effective version. A default changed by a Kandev upgrade is
therefore unknown until it is validated again. The record is not an operator
selection, so the default is still never persisted as a selection.

UI-02: Reopened fallback dialog after a successful **Use Kandev default**
(desktop and phone share `UpdateBody`):

```text
Before (user screenshot):  Repair runtime   Unknown → 1.18.34   [Repair runtime]
After:                     Update runtime   1.18.32 → 1.18.34   [Update runtime]
```

## Verification strategy

Go controller tests exercise the real controller, job store, fake updater,
and selection store end to end: enqueue, terminal job, then a fresh preview.
The existing desktop and phone fallback E2E scenarios mock the preview API, and
Playwright cannot reach a native-host fallback backend, so they cannot observe
this backend derivation. Rendering of `current_version` and operation labels is
already covered by `agent-runtime-update-control.test.tsx` and
`lib/agent-runtime-update.test.ts`. No new browser test is added.

## Risks

- Task 02 adds one key per fallback agent to the install-wide settings store.
  Records from a removed agent or package are ignored because reads check the
  trusted package.

- Same-version fallback repair is no longer offered once a selection is
  validated. Cache breakage for that version remains governed by managed npm
  runtime recovery, and the operator can choose another version or the default.
- The selection proves only that the version passed its probe at activation
  time. A later out-of-band cache deletion is not detected by the dialog. The
  same is true of the cached host observation for non-fallback runtimes.
