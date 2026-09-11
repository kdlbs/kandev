---
created: 2026-09-10
status: implemented
requirements:
  - REQ-UI-COMPOSER-OVERLAY-001
system_design:
  - ../../specs/ui/system-design/composer-suggestion-overlays.md
legacy_specs: []
---

# Fix: iOS composer picker positioning

## Overview

Evaluate a positioning-only candidate on the reporting iPhone before adopting
a production patch. The user reports that both `@` and `#` menus appear only
after dismissing the iOS 26 Safari keyboard. Current Chromium mobile tests
pass, including a disposable active-composition probe. They do not exercise
the actual iOS keyboard.

An independently reproduced defect gives this proposal a concrete target:
`computePopupMenuStyle` returns `maxHeight: 0` for an above-anchor menu at
`y=100` with visual viewport `{offsetTop:250,height:350,width:390}`. The viewport
has ample room for a header and result row. This violates
`AC-UI-COMPOSER-OVERLAY-001.1`. The phone comparison subsequently confirmed
this coordinate-space failure: suggestions were already active with composition
false, but the current menu had zero height. The user confirmed candidate
positioning works and explicitly requested implementation on 2026-09-11.

## Scope

In scope: a disposable current/candidate comparison, phone geometry and
suggestion-state evidence, then a conditional shared-popup repair with focused
regressions. The existing requirement remains authoritative and unchanged.

Out of scope: changing search, providers, input composition, draft persistence,
submission, releases, or the main instance on port 9998. The original proposal
turn stopped before production changes; the later implementation request
authorizes Task 02 and shutdown/cleanup of the disposable environment.

## Technical approach

The test instance served the current production build through a test-owned
proxy. Both modes received the same lightweight diagnostic probe. Candidate
mode alone repositioned existing popup DOM using the installed Floating UI
1.7.4 browser-coordinate handling, `top-start`, an eight-pixel offset, shifting
inside the visible viewport, and a 280-pixel height cap. It uses the live caret
rectangle with an editor fallback. Real React menu items and insertion handlers
remain intact. It cannot create a missing menu or repair delayed activation,
making the comparison useful for distinguishing those causes.

This proxy/script is a disposable experiment, not the proposed production
integration. It lived under `/tmp/kandev-picker-eval-I97UqT`, outside source;
that runtime and its artifacts have now been removed.
The confirmed repair integrates browser-coordinate normalization and
non-collapsing containment in `PopupMenu`, preserving ordinary above adjacency
and the plan editor's explicit below placement. The reconciled system design
uses Floating UI's DOM platform as a direct dependency, with a React-owned
subscription lifetime and guarded asynchronous writes. Size before shifting so
long menus do not overlap a visible composer unnecessarily. No experimental
proxy, input probe, or mutation observer is included in the production bundle.

## Mobile design contract

Entry is the normal task chat composer on desktop and phone. Keep the existing
`PopupMenu` contextual-list pattern; a new modal drawer would steal focus from
typing and is outside the active requirement. `MobilePickerSheet` contributes
the fixed-heading/internal-scroll principle, not its modal focus behavior.
Keep one scrolling result list, touch rows of at least 44 pixels, visible
viewport insets, existing ranking, and existing selection logic. Desktop keeps
caret-adjacent presentation. Mobile prioritizes containment only when adjacency
cannot fit; the keyboard remains open through selection.

## ASCII UI preview

UI-01, proposed shared composition, normal task chat:

```text
+----------------------------+
| Suggestions                | fixed heading
| mobile-check               |
| mobile-review              | scrolling results
+----------------------------+
| Composer: @mobile          | focus retained
+----------------------------+
| iOS software keyboard      | phone only, stays open
+----------------------------+
```

The order, containment, and retained focus are requirements
(`AC-UI-COMPOSER-OVERLAY-001.1` through `.5`); labels and spacing are illustrative.
Desktop uses the same popup/composer relationship without the keyboard. Current
reported phone behavior was no visible suggestions until Done. The comparison
captured their pre-Done DOM presence and the candidate's usable geometry.

## Tests and E2E

The disposable failing helper proof was recorded before artifact cleanup.
Candidate browser checks covered real `@mobile` and `#MOB` selection,
focus, containment, and an off-screen-anchor stimulus. Browser emulation is
reported separately from the real iPhone result. Mobile test sources remain
`mobile-prompt-mention-composer.spec.ts` and
`mobile-entity-reference-composer.spec.ts`. The original prompt test covered
only page-layout resizing; both files now cover offset-only viewport changes.

Permanent coverage lives in `popup-menu.test.tsx` and
the existing mobile `@`/`#` scenarios, with below-placement and `/` sibling
regressions. Add a targeted WebKit project or job only with actual runnable
WebKit dependencies, and retain a real-device keyboard-open check: WebKit
desktop emulation does not reproduce the iOS software keyboard itself.

## Work orders

1. [x] [Record iPhone comparison](task-01-record-iphone-comparison.md)
2. [x] [Repair confirmed popup positioning](task-02-repair-popup-positioning.md)

Sequential. The user authorized production implementation in a later turn;
delegation remains unauthorized.

## Verification results

- Current helper proof failed as expected: zero-height menu despite a 350-pixel
  visible viewport. This is a synthetic contract case, not an iPhone trace.
- Isolated backend used a new home, database, and fictional repository.
- Test proxy listened only on Tailscale `100.105.155.17:48762`; backend listened
  on `127.0.0.1:48761`. All test ports are now closed. Main `:9998` retains
  PID 3960526.
- WebKit launch is unavailable locally because required GTK/GStreamer libraries
  are absent. No system packages were installed or changed.
- Headless mobile Chromium: both current and candidate modes passed real
  `@mobile` and `#MOB` touch insertion, 44-pixel rows, focus retention and menu
  dismissal (four flows). The initial transcript was not submitted again.
- The browser offset-viewport contract probe reproduced a zero-height current
  popup, while candidate mode rendered a 134-pixel surface at y=708..842 inside
  the simulated visible region y=700..850. This does not simulate the iOS
  keyboard or prove its event/composition behavior.
- Candidate phone-sized screenshot was inspected; ordinary above-composer
  adjacency remains intact. The probe captured suggestion-active flags and
  real menu geometry, including pre-results and populated states.
- `git diff --check -- docs/plans/ios-composer-picker-evaluation` passed. All
  requirement/acceptance references and the system-design path exist.
- Real iPhone evaluation confirmed candidate positioning. Exact measurements
  and evidence limits are recorded in Task 01. Public docs need no update for
  this behavior-restoring presentation fix.

### Integrated patch and cleanup

- 18 popup unit tests pass, including WebKit coordinate normalization through
  the actual DOM positioning platform, viewport resize/scroll, missing-to-ready
  anchors, tiny viewports, above/below placement, and disposal.
- PR review added coverage for initially narrow menus and stable virtual-caret
  callbacks during same-size result updates, plus shared sizing constants and
  stronger E2E selectors/assertions. Task 02 records the verification.
- Fresh E2E build: all 7 targeted mobile Chromium tests and all 3 selected
  desktop Chromium keyboard-selection tests pass. Commands are in Task 02.
- Full web lint, final changed-file lint, typecheck, i18n new-code ratchet,
  specification lint, and `git diff --check` pass.
- The integrated phone-sized screenshot was inspected against UI-01: one
  heading and result list immediately above the focused composer, contained
  horizontally. It is a resized Chromium capture, not an iOS keyboard capture.
- The temporary evaluation root (1.6 GB) and two task-owned diagnostic ZIPs
  were deleted, without a recovery archive. Key phone measurements remain in
  Task 01. The main service was not modified or restarted.
- User confirmed the physical-device candidate before requesting shutdown.
  No second physical-device run of the integrated patch is claimed; local
  WebKit remains unavailable due to missing system libraries.
- Implementation is ready for the requested Open PR workflow step. Commit,
  push, and PR creation belong to that step; none occurred in this phase.

## Risks and next decision

The cause is confirmed as geometry, not activation/composition. Keep regression
coverage at the browser coordinate boundary; synthetic viewport tests alone
cannot certify the real keyboard. Prototype DOM styling is not shipped.
Main-instance data and credentials must never enter a disposable environment.

The earlier [viewport repair](../mobile-composer-suggestion-viewport/plan.md)
remains a historical implemented package; this proposal extends its untested
above-viewport and real-device cases without rewriting its recorded results.
