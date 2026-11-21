---
created: 2026-11-11
status: implemented
requirements:
  - REQ-UI-QUICK-TERMINAL-001
system_design:
  - ../../specs/ui/system-design/quick-terminal.md
  - ../../specs/ui/system-design/quick-terminal-recovery.md
legacy_specs: []
---

# Implementation Plan: Quick Terminal Shell Recovery

## Overview

[Issue #5260](https://github.com/kdlbs/kandev/issues/5260) reports terminal tabs that remain unusable after shell loss.
The implemented fix adds **Start new shell** inside the existing tab.
The issue explicitly permits this manual action.
The same-tab recovery path and its validation are complete.
The implementation also preserves the published terminal generation during close and keeps lifecycle writes ordered across conversation dismissal.

One vertical work order covers regression tests, the interaction, request ordering, translations, and public guidance.
It proves restart and exit recovery with real desktop and phone shells.
The first regression runs before production edits.

## Confirmed root cause and evidence

Investigation used checkout `bceeaf1c1d30ad05b953fd800b56ae46ebf5f2bf`.
The issue reports main `a268e5c409`.
Both the report and this checkout expose the same failure path.

- `quickterminal.Service.reconcileTab` calls `markUnavailable` after a persisted session disappears.
  The result has no session ID, exited status, and an unavailable error.
- `QuickTerminalTabView` only starts sessionless tabs with connecting status.
  An unavailable tab renders only its lifecycle message.
  A naturally exited tab has no recovery action either.
- `PtyTerminalView` captures its initial session in `initialSessionIdRef`.
  Changing a prop alone does not reset that mount for a replacement.
- `persistQuickTerminalState` currently issues independent PATCH requests.
  Recovery must prevent an older write from clearing a replacement's binding.

Two existing backend tests passed:
`TestListKeepsARunningQuickTerminalSessionAttached` and
`TestListMarksMissingQuickTerminalSessionUnavailable`.
The existing terminal view suite passed all four tests.

A temporary component reproduction failed twice for the expected missing
**Start new shell** button, covering exited and unavailable tabs.
A temporary real SQLite/loginpty/service reproduction passed for backend restart and natural exit.
It proved stable descriptor identity and sequence, a distinct replacement PTY, idempotent repeated start,
and rejection of an old session's exit callback.
Both temporary files were removed.
These diagnostic results do not prove an implemented UI fix.

### Requirement conformance and settled choices

The old design required **New Terminal** for replacement.
This is a missing same-tab recovery interaction, not a violation of its old no-implicit-start rule.
The package adds `AC-UI-QUICK-TERMINAL-001.15` through `001.20` to the existing requirement.
It preserves `001.8` sibling isolation and `001.14` local-start protection.

The existing UI contract owns this terminal utility interaction across backend and browser boundaries.
PTY and descriptor ownership remain with their existing services.
The existing descriptor ADR already requires an explicit new shell after restart.
Its rationale covers this action, so no new ADR is necessary.

## Scope

### In scope

- Explicit recovery of a selected exited or unavailable terminal.
- Recovery retry, progress, duplicate suppression, and stale-result protection.
- Stable descriptor identity, sequence, saved position, and active selection.
- Existing detach, close, workspace, and sibling boundaries.
- Desktop, tablet, phone, localization, and public recovery guidance.

### Out of scope

- Automatic restart when a user selects or reopens a tab.
- Shell survival, output persistence, or environment restoration across backend restarts.
- New endpoints, schema changes, runtime flags, or agent-login lifecycle changes.
- Restarting a live shell after an unclassified transport error.
- Generic revision protocols for all terminal APIs or unrelated lifecycle refactors.

## Technical approach

### Recovery and state ownership

Follow the [recovery flow](../../specs/ui/system-design/quick-terminal-recovery.md#recovery-flow).
Add a recovery handler through `QuickChatTerminalContent` and `QuickTerminalTabView`.
Keep lifecycle request ordering in the modal's terminal callback owner.
Extract that owner into `hooks/domains/session/use-quick-terminal-lifecycle.ts`.
Its input stays a narrow store/API seam.

Retire the old terminal callback generation before recovery preparation.
Drain prior writes, then read the workspace's descriptors through `listQuickTerminalTabs`.
Fail visibly if the descriptor disappeared.
Never call descriptor creation as recovery for a deleted tab.
For a missing shell, reset local session, exit code, and error, then mount a fresh PTY generation.
Bypass the initial new-tab descriptor-create effect for this confirmed existing descriptor.
For a live shell discovered during recovery, reattach to that shell.

Reuse `startHostShell` with the stable `tabId` as `client_id`.
The backend already supports replacement under that key and binds it before returning.
No backend production change is expected.
Do not change shared `PtyTerminalView` startup semantics merely to implement Quick Terminal recovery.

Keep pending preparation tied to the tab owner across presentation detach.
Explicit close cancels preparation or stops a late start through the existing pending-start registry.
Do not clear a request's pending latch because an unrelated resync arrived.
Test delayed writes at the server boundary, not only delayed response application.

### Presentation and documentation

Use the existing lifecycle region and `@kandev/ui/button`.
Keep the button name stable while the translated status announces progress.
Phone and coarse-pointer controls have 44px targets.
Fine-pointer desktop retains the ordinary 28px button height.
The existing dialog and visual-viewport handling supply safe-area and keyboard clearance.

Add the `sidebar:quickChatTerminalStartNewShell` key to all seven real catalogs.
Regenerate Traditional Chinese and pseudo catalogs with their existing scripts.
Update the Quick Chat/Quick Terminal section in `docs/public/developer-tools.md` during implementation.
That section is a how-to guide.
It must distinguish recovery in the same tab from **New Terminal** and explain that old shell state is lost.
This design-only change does not publish unimplemented public guidance.

### Existing delivery records

- `quick-terminal` is the completed shared-dialog baseline.
- `quick-terminal-refresh-persistence` is complete and its Task 04 records its actual older E2E results.
  Its New-Terminal-only restart scenario is replaced by this package's explicit recovery matrix.
- `quick-terminal-durable-lifecycle` still has draft delivery metadata, although this checkout contains its lifetime policy and `StopAll`.
  Preserve its historical status and results rather than inferring completion for unrelated work.
- `quick-chat-tab-order` is completed. Its saved-order contract remains unchanged.
- `quick-terminal-mobile-controls` supplies the existing keybar and keyboard-clearance evidence.

The refresh package links this follow-up without rewriting its historical pass counts.
New recovery results belong here and in the work order.

## ASCII UI preview

UI-01: Selected exited tab, opened through the existing Quick Terminal launcher.
The before state comes from source and the temporary component reproduction.

```text
Desktop before: Terminal 1
  Terminal error: terminal session unavailable

Desktop after: Terminal 1
  Terminal error: terminal session unavailable
  [ Start new shell ]

Phone after: existing full-height Quick Chat surface
  +--------------------------------+
  | Quick Chat                 [X] |
  | [Terminal 1] [Terminal 2]       |
  |                                |
  | Terminal session unavailable   |
  | [       Start new shell      ] |
  +--------------------------------+
```

UI-02: Recovery is pending or fails.

```text
Pending: Connecting to terminal...
         [ Start new shell ] (disabled)
Failure: Terminal error: <current error>
         [ Start new shell ] (enabled)
Success: same tab and label, usable shell; recovery action hidden
```

Required structure: status before the action, no extra dialog, unchanged tabs, and one visible recovery action.
The phone button uses its own full-width row with a minimum 44px hit target.
The lifecycle region stays fixed above keyboard occlusion and within dialog safe areas.
Running terminal output retains its existing scroll owner.
Spacing and error examples are illustrative. All rendered copy uses translations.
Views map to `001.15` through `001.19` and the work order's rendered checks.

## Tests

Task 01 adds these regression cases beside the existing tests:

| Acceptance | Test file and named case |
| --- | --- |
| `001.15` | `quick-terminal-tab-view.test.tsx`: offers same-tab recovery for exited and unavailable terminals without starting automatically |
| `001.16` | `use-quick-chat-modal-terminal-lifecycle.test.ts`: preserves the descriptor and selection during recovery |
| `001.17` | Terminal lifecycle hook tests: coalesces recovery activation and permits explicit retry after failure |
| `001.18` | Terminal lifecycle hook tests: drains an old lifecycle write before replacement and ignores retired callbacks |
| `001.14` | `quick-chat-sync.test.ts`: retains a pending recovery when an older unavailable response arrives |
| `001.8`, `001.18` | Modal/view tests: sibling remains running; close stops a late replacement; dismissal permits reattachment |
| `001.18`, `001.20` | `use-quick-chat-modal-terminal-lifecycle.test.ts` and `quick-terminal-tab-view.test.tsx`: failed DELETE retains the published generation; held DELETE fences descriptor creation and PTY startup |
| `001.18` | `quick-chat-provider-lifecycle.test.tsx`: deferred exited PATCH is drained across terminal-to-conversation dismissal, reopen, and recovery |

Put the extracted lifecycle owner's tests in
`hooks/domains/session/use-quick-terminal-lifecycle.test.ts`.
Retain the four existing view tests and shared PTY lifecycle coverage.
The first named regression must fail on the current implementation before the fix.

## E2E tests

The work order adds `quick-terminal-recovery.spec.ts` and `mobile-quick-terminal-recovery.spec.ts`
under `apps/web/e2e/tests/terminal/`.
The first uses `chromium`; the second uses the configured Pixel 5 `mobile-chrome` project.

| Scenario | Acceptance and evidence |
| --- | --- |
| Shell exits with the dialog open | `001.15`, `001.16`: same tab and sequence, distinct session, executable output after recovery |
| Backend restarts, then page reloads | `001.15`, `001.16`: persisted unavailable descriptor, no implicit start, explicit recovery succeeds |
| Browser remains open across restart | `001.14`, `001.18`: reconnect settles, stale session becomes recoverable, old state cannot overwrite recovery |
| Failed start, repeated activation, then retry | `001.17`: one effective start, visible error, no loop, successful later retry |
| One failed tab beside one live sibling | `001.8`, `001.16`: sibling retains its session and environment marker |
| Live tab reload and running reselect | `001.16`: existing session stays attached and recovery stays hidden |
| Phone touch and open keyboard | `001.19`: reachable action, 44px bounds, keyboard clearance, no horizontal overflow |
| Desktop at 767px and 768px, plus coarse tablet | `001.19`: phone sizing follows width, desktop stays 28px, touch stays at least 44px |

Use `backend.restart()` on the disposable worker fixture and preserve its database.
Scope locators and buffer reads to the active panel.
Arm causal HTTP waits before actions and inspect persisted descriptors through the real API.
Use assembled markers such as `printf 'RECOVERED%s\\n' '_SHELL'` to exclude command echo.
Close all surviving descriptors in `finally`, then allow fixture teardown.

## Work orders

- [x] [Task 01: Add explicit same-tab shell recovery](task-01-shell-recovery.md) (`done`)

## Verification results

Diagnostic evidence is recorded above.
Design-package checks passed on 2026-11-11:

- `python3 scripts/list-docs.py validate`: 448 decisions and 1982 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- Local `validateCoverage` from `.github/scripts/pr-docs.cjs`: `covered`, zero errors.
  It used this work order, its linked documents, and a representative production-path trigger.
  This proves reference coverage rather than accepting a documentation-only exemption.
- `git diff --check -- docs/specs docs/plans`: passed.
- The package remains unstaged and uncommitted.

Implementation and delivery validation completed on 2026-11-11. See [Task 01 results](task-01-shell-recovery.md#results).

## Risks

- Old lifecycle PATCH requests can erase a replacement binding unless recovery orders them before startup.
- Reusing the old PTY mount retains its initial session ref and can reattach the dead shell.
- Close during descriptor preparation can race startup unless both use the stable tab owner.
- A close must retain its fence through successful row removal and release it without a generation change after failed DELETE.
- Concurrent clients can recover the tab before the current read settles. Stable-key reuse must preserve their live shell.
- A fresh shell cannot recover process state, environment changes, or output lost at backend restart.
- The main design is near its 32 KiB limit. Recovery detail stays in the linked lifecycle design.
