---
status: draft
system: ui
requirements:
  - REQ-UI-PALETTE-FILE-CREATE-001
---

# Palette File Creation Feedback System Design

## Boundary and current behavior

This pair owns feedback for the independent palette action. The workspace
service retains the file mutation contract; this change does not alter it.
`SessionCommands` registers `buildWorkspaceCommands` when a session has a
worktree. Its input callback calls `createFile` in `lib/ws/workspace-files.ts`;
`CommandPanel` closes before invoking the asynchronous callback. Today a thrown
request only logs, and a missing client or `success: false` produces no notice.
The desktop qualification observed a real `workspace.file.create` success and
editor, then a duplicate request rejected by agentctl without visible feedback.
Phone behavior has not yet been qualified.

## Action and failure flow

Wire the existing `useToast` provider into the workspace command builder from
`SessionCommands`. Keep handling local to the file-create action and preserve
the existing request, session/path arguments, and successful
`addFileEditorPanel(path, name)` call. Reuse the existing `runFileOp` semantics
or equivalent local handling: show the custom error toast with localized
`task:failedToCreateFile` title and the response error or thrown `Error.message`
as description, falling back to `common:anUnknownErrorOccurred` for absent or
non-Error detail. A missing client also produces the title and fallback without
issuing a request. Only a successful result opens the editor. Avoid duplicate
notices and do not change `useFileOperations` or common transport behavior.

The existing `ToastProvider` renders the notice in its live region outside the
closed palette and dismisses it on its existing timer. No new notice action,
dialog, palette state, or copy is needed. Translations are resolved at action
time from the registered command's translation function, never at module scope.

## Desktop and phone composition

**UI-01, desktop:** Mod+K opens the existing command dialog. Selecting Create
File and submitting a path closes it. On rejection the existing error toast
appears over the task, with the localized title followed by available detail;
success continues to open the editor. The palette remains independently
reopenable.

**UI-02, phone:** App navigation's Search tasks action opens the palette drawer;
the Commands tab leads to Create File. The command option is tapped and the
path is submitted with the input keyboard's Enter action. After the drawer closes,
the same toast remains visible and viewport-contained. The existing inset,
safe-area, internal list scrolling, close control and navigation are retained;
no layout or touch target is changed. The existing mobile task palette is the
closest exemplar. Verify actual entry, option hit target, dismissal and notice
containment rather than assuming desktop behavior applies on a phone.

## Verification and docs

An independently authored action/component regression covers thrown requests,
unsuccessful responses with and without detail, a missing client, and success.
Real managed desktop and phone browser tests exercise the actual palette and
real backend/agentctl response: success control and duplicate collision, with
the existing file bytes preserved. Arm the WebSocket observer before navigation
and correlate each action and response before asserting rendered feedback.
The other failure shapes are simulated only at the unit boundary. Capture the
visible result for each viewport and label which evidence is native. Public
documentation describes existing palette and file workflows; restoring a missing
failure notice adds no option, terminology, or changed instructions, so no
public page update is planned.

## Requirement mapping

| Criterion | Evidence |
| --- | --- |
| 001.1 | Action success test and native success control |
| 001.2 | Action failure tests and native duplicate rejection |
| 001.3 | Desktop and phone real palette, rendered toast and containment |
| 001.4 | Native duplicate rejection and preserved sentinel bytes |

All suffixes refer to `AC-UI-PALETTE-FILE-CREATE-001`.
