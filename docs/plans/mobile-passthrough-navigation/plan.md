---
created: 2026-10-09
status: done
requirements:
  - REQ-CLI-MOBILE-PASSTHROUGH-NAV-001
system_design:
  - ../../specs/cli/system-design/mobile-passthrough-navigation.md
legacy_specs: []
---

# Implementation Plan: Mobile Passthrough Navigation

## Overview

Deliver issue #4370 as one vertical work order: reuse phone terminal keys,
route raw bytes only to the agent socket, and verify the real interaction.
The user approved this package on 2026-10-09. Recovery and reconciliation
resume on 2026-10-20. See [recovery evidence](recovery.md).

## Scope

### In scope

- Phone passthrough Chat navigation, modifiers, owned focus, and viewport clearance.
- Connection/ownership cleanup, composer coexistence, targeted tests and docs.

### Out of scope

- Quick Terminal's separately reported floating bar, backend changes, and new
  tablet/desktop controls. No native implementation delegation is authorized.

## Technical approach

Follow the [system design](../../specs/cli/system-design/mobile-passthrough-navigation.md).
Adapt the phone branch of `AgentTerminalInput` with one manual xterm subscription and local
key-bar callbacks. Move `sendPtyInput` to the terminal library, preserving its
host-PTY behavior. `MobilePanelArea` owns keyboard/bottom-nav clearance.
`PassthroughToolbar` supplies composer/comments visibility without remounting
the phone terminal. Reuse existing labels; any new copy needs all locale catalogs.

| Surface / transport | Intended behavior | Evidence / unavailable fallback |
| --- | --- | --- |
| Phone agent / binary session socket | One transformed send to selected agent | New phone E2E and wrapper tests; unavailable means no send |
| Desktop/tablet agent / AttachAddon | Existing input and layout | Existing passthrough desktop tests; no added bar |
| Task shell / shell socket or gateway | Existing selected-shell controls | Existing mobile key-bar suite |
| Quick Terminal / host PTY socket | Preserve existing inline shortcuts | Shared helper unit tests and Quick Terminal suites |

## ASCII UI preview

UI-01: Phone task Chat, connected passthrough, panels closed.

```text
Before                     After
[Session picker]           [Session picker]
[Agent output          ]   [Agent output          ]
[                      ]   [                      ]
[Chat | Comments | ... ]   [Ctrl Shift ^C ^D Esc Tab >]
[Task bottom navigation]   [Chat | Comments | ... ]
                           [Task bottom navigation]
```

The key strip scrolls internally to arrows and remaining existing keys. It is
inline, not over output. With keyboard open, the content ends above the keyboard;
the existing task navigation can remain beneath keyboard occlusion. Output is
flexible; the strip/status row keep intrinsic height. Safe-area clearance belongs
to the mobile layout. Maps to AC-CLI-MOBILE-PASSTHROUGH-NAV-001.1-.4.

UI-02: Phone composer/comments expanded, or terminal unavailable.

```text
Expanded composer          Connecting / disconnected
[Agent output          ]   [Existing terminal overlay]
[Composer + Send       ]   [                         ]
[Chat | Comments | ... ]   [Chat | Comments | ...     ]
```

Raw navigation is absent in both states. Composer input is unmodified and drafts
are preserved. Maps to AC-CLI-MOBILE-PASSTHROUGH-NAV-001.5-.7.

UI-03: Desktop/tablet passthrough, existing composition.

```text
[Dock / session context                     ]
[Native agent terminal                      ]
[Optional composer                          ]
[Chat | Comments | workflow / provider state ]
```

No added navigation strip. Maps to AC-CLI-MOBILE-PASSTHROUGH-NAV-001.8.
Control order, grouping, visibility, and scroll ownership are required; diagram
spacing and abbreviated copy are illustrative. Rendered UI keeps localization.

## Tests

| Criteria | Evidence |
| --- | --- |
| 001.2-.3, 001.6 | `lib/terminal/send-pty-input.test.ts`: exact bytes, closed sockets, modifier consumption; `mobile-agent-terminal.test.tsx`: one sender and replacement races |
| 001.1, 001.5, 001.7-.8 | Wrapper/toolbar tests: visibility, ownership activation/cleanup, composer focus/draft, desktop path |
| 001.4 | Mobile layout tests: keyboard open/close and baseline clearance; bar tests: focus and targets |

## E2E tests

Add `e2e/tests/cli-mode/mobile-passthrough-navigation.spec.ts` using the existing
passthrough fixture pattern and `MobileTerminalKeybarPage`. Observe sockets before
navigation. Cover arrows/Esc/Tab/^C/^D, Ctrl latch/sticky, Shift+Tab, single delivery,
active-session routing, composer expansion/draft, disconnect/reconnect, and panel
switch cleanup (001.1-.7). Verify actual terminal output for at least one native
interaction and no persisted user message for raw keys.

Measure 44px targets, internal strip scrolling, zero document overflow, keyboard
open/closed geometry, and 767/768px boundary behavior, including fine pointers.
Preserve managed-Chat hidden-bar expectations in `mobile-terminal-keybar.spec.ts`.
Run adjacent mobile composer/rendering and Quick Terminal regressions; extend
`passthrough-toolbar.spec.ts` with a desktop absence/physical-input check (001.8).
Capture and inspect the phone states against UI-01/UI-02; record real mobile
browser smoke evidence or the exact lack-of-device limitation.

## Work orders

- [x] [Task 01: Add phone passthrough navigation](task-01-phone-navigation.md)
  (done; wave 1; no dependencies; sequential).

## Verification results

Completed locally on 2026-10-20 against verified main `e3ef99a98f8`.
The recovered implementation was integrated into the existing component.

- 118 unit/component tests passed across seven files.
- 27 mobile and 12 desktop browser tests passed with retries disabled.
- Typecheck, changed-file ESLint, formatting, and localization checks passed.
- Specification catalog/lint, public documentation checks, and local PR
  documentation coverage passed.
- Phone captures cover connected controls, simulated keyboard occlusion, and
  the expanded composer. The captures were inspected against UI-01/UI-02.
- Physical iPhone Safari and Android Chrome keyboard testing was unavailable.
  Simulated viewport tests do not replace native keyboard smoke testing.

See [recovery evidence](recovery.md) for provenance, historical gaps, environment
repairs, and local log paths. The user subsequently authorized branch delivery and an isolated test instance.
The user later authorized PR #4625 and review fixup.

## PR review fixup

Greptile identified shared Ctrl state being cleared by a background agent
reconnect while Quick Terminal was open. The agent now releases modifier
ownership while the Quick Chat overlay is open. CodeRabbit identified a ref
write during render that could change input routing before a React transition
committed. A layout effect now synchronizes that input state. Unit regressions
reproduced both findings before their fixes. The browser regression also
reproduced the Quick Terminal failure against the prior build.

Claude later recommended removing the single-use JSX factory. The toolbar now
renders the terminal inline, and its existing composer hook owns composer state,
focus, and shortcut handling. This preserves behavior and removes 26 source lines.

The frontend CI project-selection contract exposed a stale reviewed test path
after the PTY input helper moved. The manifest now retains that test in its
existing browser project under its new path.

Remote checks and review disposition remain pending until the final fix is pushed.
See the work order for local verification.

## Risks

- Default task-shell fallback would route keys incorrectly; local callbacks are mandatory.
- AttachAddon plus a manual listener can duplicate input unless receive-only mode is used.
- Shared modifiers need cleanup across composer, task, and Quick Terminal ownership.
- Real software-keyboard behavior requires device smoke evidence.
- Quick Terminal changes are complete and excluded from this task.

## Public documentation

Updated `docs/public/developer-tools.md` with phone passthrough navigation,
modifier behavior, and composer visibility. All labels reuse existing translations.
