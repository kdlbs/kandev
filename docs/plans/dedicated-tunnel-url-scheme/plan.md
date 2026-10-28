---
created: 2026-10-27
status: implemented
requirements:
  - REQ-UI-PORT-FORWARDING-ACTIVE-FIRST-001
system_design:
  - ../../specs/ui/system-design/port-forwarding-active-first.md
legacy_specs: []
---

# Implementation Plan: Dedicated tunnel URL scheme

## Overview

Correct the dedicated listener link on HTTPS dashboards in one sequential work
order. The active-first UI contract owns the displayed URL and its actions;
port-forwarding discovery owns entry points and the proxy Browser contract owns
its existing action. The gateway listener already serves HTTP. There is no
material unresolved product choice: the requested scheme and exclusions are
explicit, and current code confirms the listener and shared UI boundary.

## Scope

### In scope

Dedicated URL construction, independently authored rendered regression coverage,
and a concise public explanation of dedicated and proxy URLs.

### Out of scope

TLS, new proxy modes, listener or lifecycle changes, IPv6 feature work, port
input cleanup, UI redesign, new copy or locale changes, and broad refactoring.

## Technical approach

In `apps/web/components/task/port-forward-list.tsx`, derive the dedicated URL
from HTTP, the parsed backend hostname, and the returned dedicated port. Preserve
`buildPortProxyUrl` and `PortUrlActions` semantics. The shared row serves both
the desktop Dialog and the phone Drawer; no composition, navigation, pointer,
scroll, or breakpoint behavior changes. The existing phone Panels entry and
bottom Drawer are the mobile exemplar, with the same shared data and actions.
The previous active-first plan's HTTPS tunnel examples describe its historical
planning illustration; the updated active-first contract governs this correction.

During implementation, update `docs/public/tasks-and-workflows.md` to state that
dedicated links use HTTP and the returned port while proxy links use the
configured backend URL. Do not imply that remote access to that separate port
is guaranteed. No ADR is needed for a correction within existing boundaries.

## Rendered outcome

For an HTTPS backend such as `https://host:8443/base`, a forwarded row displays
and opens `http://host:49152/`; its proxy retains
`https://host:8443/base/port-proxy/session/3000/`. For an HTTP backend the
same dedicated URL remains HTTP. Desktop and phone keep their existing row
composition and localized labels. This changes URL data only; no UI layout or
interaction preview is necessary.

## Tests

For AC-UI-PORT-FORWARDING-ACTIVE-FIRST-001.3, .6, and .9, independently add
`apps/web/components/task/port-forward-list.test.tsx` using the real rendered
list and real action elements. Exercise an HTTPS backend with a distinct backend
port and base path, a returned tunnel port, and a target port; assert displayed
URLs, dedicated and proxy link destinations, copy targets, and proxy-only
Browser callback. Exercise HTTP as a control. The regression must fail on the
current HTTPS dedicated link before the fix and pass afterward. Do not reuse
private qualification fixtures. Keep the existing dialog suite as an adjacent
regression check.

## E2E tests

This is shared URL data normalization inside the existing component: it changes
no layout, touch behavior, scrolling, navigation, or viewport-dependent
interaction. The mobile-parity data-only exception applies; the targeted rendered
component test covers the shared row and its user-facing URL actions. Existing
`e2e/tests/session/mobile-port-forwarding.spec.ts` covers the phone manager
and `e2e/tests/session/port-forward-dialog.spec.ts` covers desktop management;
neither is claimed to verify the HTTPS scheme. Qualification separately observed
an actual dedicated listener accepting HTTP and rejecting HTTPS. No new mobile
Playwright test or local production build is required for this narrow correction.

## Work orders

- [x] [Task 01: Correct dedicated tunnel links](task-01-correct-dedicated-tunnel-links.md)

## Verification results

Design validation on 2026-10-27: `python3 scripts/list-docs.py validate` passed
(408 decisions, 1760 specifications); `python3 scripts/lint-spec-files.py --all`
passed; `git diff --check` passed. Status showed the two modified owning
documents and the untracked plan directory. The [work order](task-01-correct-dedicated-tunnel-links.md#results)
records the implementation's red/green test and targeted check results.

## Risks

- A test that only checks a URL helper can miss the rendered Open or Copy target;
  exercise the actual row actions.
- HTTPS dashboards do not make the separate dedicated HTTP listener encrypted;
  document the distinction accurately without promising remote reachability.
