---
created: 2026-11-06
status: implemented
requirements:
  - REQ-TASKS-PROMPT-ATTACHMENTS-001
system_design:
  - ../../specs/tasks/system-design/prompt-attachments.md
legacy_specs: []
---

# Implementation Plan: Attachment delivery after live rescan

## Overview

Keep attachment uploads and adapter reads in the active agent directory after
a repository rescan. Reject unresolved attachments before provider dispatch.
One sequential work order covers this delivery boundary and its regression tests.
Implementation and local verification completed after the explicit user request
on 2026-11-06. The live instance was not changed.

## Evidence and root cause

Read-only evidence from the affected session established these facts:

1. Both uploaded PNGs were valid, identical 21,974-byte files measuring
   451 by 221 pixels. Stored files and materialized copies matched.
2. A live repository rescan promoted the tracking root to the parent task
   directory. The agent retained its original child directory.
3. Materialization wrote both images beneath the parent root, while the active
   adapter searched beneath the child root. Runtime logs recorded both failed
   lookups, and the native transcript recorded both omitted images.
4. A temporary test reproduced HTTP 201 followed by
   `ErrMaterializedAttachmentMissing` using the real rescan, upload handler,
   and attachment reader. Permanent regression tests now cover that failure.

Before this repair, materialization used the mutable `s.cfg.WorkDir`, while
`acp.NewAdapter` created its attachment manager from a configuration snapshot.
After a failed lookup, content construction continued with empty image data.
The affected runtime revision contained the same code paths.

Raw ACP frames and provider decoder diagnostics were unavailable. The empty
wire payload is inferred from source, runtime warnings, and the native
transcript. No live instance was changed.

## Requirement conformance

This is an implementation violation of
[REQ-TASKS-PROMPT-ATTACHMENTS-001](../../specs/tasks/requirements/prompt-attachments.md).
AC-001.1 includes native delivery and visible materialization failure.
AC-001.6 and AC-001.7 extend those guarantees to generating turns.
No requirement amendment is needed. The design now clarifies the shared root.

The existing file-backed attachment ADR and live-rescan ADR settle the boundary.
This repair needs no new ADR. It must preserve live agent continuity.
Related completed packages cover
[composer scope](../composer-attachment-scope/plan.md),
[preparation previews](../preparation-attachment-previews/plan.md), and
[fresh-start recovery](../fresh-start-recovery/plan.md).
Their acceptance scopes and completion results remain valid.

## Scope

### In scope

- Resolve materialization against the active adapter directory across live rescans.
- Reject unresolved descriptors before ordinary prompt or steer dispatch.
- Prove byte delivery and lifecycle behavior through targeted tests.

### Out of scope

- Host version reporting, live restarts, data migration, or moving existing copies.
- Changes to browser upload controls, provider capabilities, limits, or protocols.
- Stopped tasks, additional sessions, and delegation.

## Technical approach

In `process/manager_attachment_workdir.go`, expose the active adapter directory through a narrow
process-manager method with existing lifecycle synchronization. Reuse the
adapter configuration's root. Avoid a parallel attachment registry or a root
derived from descriptor input. The operation rejects stopped or replaced adapter state. Before the first
adapter exists, preserve the endpoint's configured-root materialization behavior.
`WithAttachmentWorkDir` pins startup, stop, and teardown admission during local
installation. Multipart network reads finish before those locks are acquired.

In `api/attachments.go`, use that admitted directory for the existing streaming
installation path. Preserve safe names, session directories, private permissions,
collision suffixes, byte counts, and cleanup. Rescans may still change trackers.
Explicit stopped-process rebind must establish the new root when the adapter
is constructed. Test both transition types.

In `acp/adapter_prompt.go`, return an error from content construction when a
claimed file is missing or unreadable. Forward that error through `sendPrompt`
for both prompt and steer paths. Validate before steer handoff or pending-context
consumption. Preserve turn/generation settlement and release
the prompt gate on error. Keep valid inline payloads and the existing path
fallback for present files above the native-content limit.

| Provider/transport | Shape | Required behavior | Evidence and limitation |
| --- | --- | --- | --- |
| Codex over ACP | Claimed PNG after rescan | Deliver nonempty original bytes | Real HTTP materialization and scripted ACP capture |
| Other image/audio ACP providers | Same supported native block shape | Apply the shared root and failure rules | Shared tests only, no provider capability expansion |
| Path/resource delivery | Safe file name and native session | Deliver a readable path relative to the active agent directory | Read captured path and compare bytes |
| Legacy inline content | Existing bounded data payload | Preserve current delivery | Existing adapter regression tests |
| Missing or unsupported descriptor | No readable materialized file | Reject before dispatch, retain owned error | Prompt and steer failure capture |

## Tests

`task-01-repair-delivery.md` owns these permanent regressions:

- `api/attachments_rescan_test.go`: materialize before/after real rescan,
  duplicate names, repeated rescan, and exact PNG bytes captured by an ACP peer.
- `process/manager_attachment_workdir_test.go`: replacement adapter roots and
  teardown admission draining.
- `acp/adapter_prompt_test.go`: legacy inline delivery, native file content,
  and the existing large-file path fallback.
- `acp/adapter_prompt_attachment_delivery_test.go`: scripted connection captures
  ordinary and steer content, rejects missing image/audio/resource descriptors
  and unreadable files, preserves pending context and the generating turn,
  and permits a later valid prompt after an error.

Map successful delivery to AC-TASKS-PROMPT-ATTACHMENTS-001.1 and .6.
Map rejection and settlement to AC-TASKS-PROMPT-ATTACHMENTS-001.1 and .7.

## End-to-end evidence

Use an in-process test from materialization HTTP through the real attachment
reader and ACP request capture. Include a valid generated PNG and image-only
submission. A transcript thumbnail is insufficient because it reads backend
storage rather than executor content. No rendered UI changes or browser test
are needed for this backend boundary.

## Work orders

- [x] [Task 01: Align attachment materialization and adapter delivery](task-01-repair-delivery.md)

## Verification results

Diagnostic command, run from `apps/backend`:
`go test ./internal/agentctl/server/api -run '^TestInvestigationAttachmentAfterLiveRescan$' -count=1`.
Expected failure confirmed in 0.060 seconds: successful materialization followed
by `materialized attachment is missing: image-id`. Temporary test removed.

Permanent regressions first failed on the original materializer root and on
missing descriptors reaching the provider. A further regression showed that
validating after steer handoff suppressed the generating turn's completion.
Validation now occurs before handoff and pending-context consumption.

The three targeted test commands in the work order passed. The combined race
command also passed for API, process, and ACP packages, including existing steer,
handoff, and wakeup tests. Tests use synthetic files and an in-process ACP peer;
no user image was submitted to a provider.

Final lint and documentation check results are recorded in the work order.
The user subsequently requested PR publication. No live files were repaired or moved, and no host
or agent process was restarted.

## Risks

- A root captured for the lifetime of agentctl would become stale after explicit
  process rebind. Bind it to the active adapter generation instead.
- An error after prompt admission must settle only its own turn and release gates.
- Shared ACP tests establish Kandev delivery, not every provider's decoder support.

## Public documentation

Internal docs only. The repair restores the documented delivery behavior and
introduces no command, setting, UI control, or operator procedure. Existing
`docs/public/tasks-and-workflows.md` attachment guidance and
`docs/public/automation-and-mcp.md` live-rescan guidance remain accurate.
