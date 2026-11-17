---
created: 2026-10-27
status: implemented
requirements:
  - REQ-UI-MOBILE-SHELL-ARROWS-001
system_design:
  - ../../specs/ui/system-design/mobile-shell-arrow-navigation.md
legacy_specs: []
---

# Implementation Plan: Mobile Shell Application Cursors

## Overview

The dedicated phone shell keybar currently sends normal CSI arrows even after a full-screen program enables application cursor mode. ROOT observed a real curses program report Escape on a keybar Up tap while a native ArrowUp control passed. Select the sequence from the current shell xterm at tap time and preserve the existing raw sender boundary. One sequential work order owns the end-to-end repair.

The existing CLI navigation pair owns agent PTY behavior and explicitly specifies CSI; Quick Terminal owns its own host PTY, and terminal rendering excludes input. This task-shell interaction has no existing requirement/design pair, so the new bounded UI pair owns it without changing those separate contracts. Public `docs/public/developer-tools.md` already describes the task terminal arrow row and selected-terminal behavior; restoring it requires no correction or new public copy.

## Scope

In scope: four arrows, dynamic mode changes, active shell and session isolation, normal fallback, existing modifier and native-input behavior, focused real-PTY phone and desktop controls. Out of scope: agent and Quick Terminal mode changes, Home/End TUI support, new modifier chords, layout/copy changes, dependencies, backend API, and broad terminal refactoring.

## Technical approach

Extend `lib/terminal/mobile-active-terminal.ts` registration to bind the current shell xterm's live `modes.applicationCursorKeysMode` reader to its raw sender and its existing session and cleanup identity. `MobileTerminalPane` owns registration; `MobileTerminalKeybar` identifies arrow actions, and the task-shell send path chooses CSI or SS3 for that registration at each tap. The default-shell fallback uses CSI when no matching live registration exists, including when a sender throws. Keep native xterm `onData` and ordinary strings untouched; preserve existing modifiers, consumption and sender routing. Inline `MobileAgentTerminal` and `PtyTerminalView` callbacks stay unchanged. Do not infer their application-mode support from this fix.

## Tests and E2E

Add targeted unit/component tests for all four normal/application mappings, mode toggles after registration, multiple terminals/sessions, stale release, fallback and sender exceptions, latched/sticky consumption, native generated input and unchanged multi-character text/paste. Keep existing Shift+Tab and Ctrl-letter checks. Map them to AC .1-.4.

In `e2e/tests/terminal/mobile-terminal-keybar.spec.ts` (`mobile-chrome`), independently author an actual shell PTY consumer using Python stdlib curses with `keypad(True)`, a visible readiness marker, bounded waits and explicit cleanup. Make each arrow a separately reported case so an initial failure cannot hide the other directions; tap the real keybar and require the program to report its corresponding `KEY_*`, not merely captured bytes. Establish pre-fix behavioral RED for all four, and a separately reported native ArrowUp positive control on the same program. A separate real PTY normal-mode consumer asserts all four keybar results in normal mode and after exiting application mode. Assert active terminal isolation and mode switching with focused component/registry tests; add a phone switch control when the fixture permits deterministic distinct PTYs. Assert the phone key target is at least 44px in both dimensions, visible/reachable within its scrolling row, and document horizontal overflow is zero (AC .1-.5).

In `e2e/tests/terminal/terminal-keyboard.spec.ts` (`chromium`), use the real dedicated shell and native keyboard with an application-mode consumer as desktop compatibility control. Do not claim an unrun agent, Quick Terminal, physical-device, or unrelated mode test. Require nonzero discovery for each exact project and test name. The behavioral RED must fail on expected key vs observed key after readiness, not setup or missing selectors. Preserve ROOT's qualification as independent evidence; do not read or reproduce its private artifacts.

## Mobile parity and docs assessment

The existing phone surface is retained: the terminal owns vertical output scrolling, the key row owns horizontal scrolling, and the viewport-aware fixed row clears the software keyboard and bottom navigation. The nearby mobile task layout is the curated focused-view exemplar. A tap and measured hit target, row containment and overflow check verify ergonomics; no CSS or rendered control changes are planned. Desktop native keyboard is a separate control. Existing public how-to wording already describes the arrows, so no public documentation change is needed.

## Work orders

- [x] [Task 01: Restore mobile shell arrow navigation](task-01-mobile-shell-arrows.md)

## Verification results

The work order records the pre-fix real-PTY RED, focused phone and desktop GREEN, unit, typecheck, lint, ratchet, specification, coverage, and capture results. The exact CI runtime image was probed for Python curses and xterm terminfo; hosted CI remains independent verification.

## Risks

- A registration may outlive or precede its xterm or socket; bind the reader to the same active owner and verify stale cleanup and reconnect.
- Shared raw input must not be rewritten, or native xterm keys and pasted escape sequences could be altered.
- A test that sees only normal CSI bytes does not prove navigation in a real application-mode consumer.
