---
created: 2026-09-21
status: in_progress
requirements:
  - REQ-CANVASES-AGENT-WEB-APPS-007
system_design:
  - ../../specs/canvases/system-design/canvas-host-chrome.md
legacy_specs: []
---

# Fix Plan: Name The Real Cause Of A Canvas Runtime Startup Failure

## Overview

The canvas host reports every startup failure as a release problem. Carry the cause the guest
bootstrap already reports through the frame boundary to the host state panel, and give the host a
runtime-startup-failure state that is separate from the release-unavailable state.

## Confirmed root cause

`apps/backend/internal/plugins/webapp/runtime_bootstrap.go:43,68` posts `document_error` or
`context_unavailable` with a failed startup result.
`apps/web/components/plugins/web-app-startup.ts:23` carries and validates that code.
`apps/web/components/plugins/web-app-frame.tsx:119` passes only `result.result` into
`finishAttempt`, whose signature at line 73 is `(result: "ready" | "failed")`. `onError`
(lines 23, 35) takes no argument, so the cause cannot cross the boundary even in principle.

`apps/web/components/settings/canvas-host-route.tsx:457` wires `onRuntimeError` to
`markRuntimeUnavailable` (line 320), which sets host state to `unavailable`. That is the same state
`stateForCanvas` produces from `active_release_status === "unavailable"` (line 32), so
`canvas-host-components.tsx:63` renders `canvases:unavailableDescription`, "The active canvas
release is not available." (`apps/web/src/locales/en/canvases.json:24`).

Measured receipt: a scratch component test drove all three failure paths against the current
component and recorded what the host receives.

| Driven failure | `onError` calls | argument count |
| --- | --- | --- |
| guest posts `document_error` | 1 | 0 |
| guest posts `context_unavailable` | 1 | 0 |
| no acknowledgement for 15s | 1 | 0 |

Zero arguments in every case is the defect: the cause is computed, parsed, validated, and dropped.
The scratch file was deleted after the measurement; the regression coverage lands in the work order.

There are four failure origins, not three. Besides the two guest codes and the
`WEB_APP_STARTUP_TIMEOUT_MS` deadline (`web-app-frame.tsx:105`), the iframe element's own error
handler (`web-app-frame.tsx:139`) settles a fourth.

## Scope

### In scope

- A host-facing failure-cause union in `web-app-startup.ts` covering the two guest codes, the
  startup timeout, and the iframe load error.
- Carrying the cause through `WebAppFrame` -> `CanvasPage` -> `CanvasHostBody` ->
  `CanvasHostRoute`.
- A `runtime_failed` host state with per-cause descriptions, keeping Retry and release actions.
- English copy plus the four translated catalogs and the pseudo catalog.
- Component regression coverage in the three existing test files.

### Out of scope

- The CSP and sandbox token set. #3827 owns that and it is already on `main`.
- Reporting a guest CSP violation to the host. See the decision below.
- `applyCanvasHostError` (`canvas-host-route.tsx:131`), which keeps `offline`/`unavailable` for host
  request failures and already overrides the description with the returned error text.
- `plugins:webAppUnavailable`, the frame's own overlay copy, which the canvas host never shows
  because it swaps to the state panel.
- Any backend change.

## Decisions

- **A new state, not a description variant.** `unavailable` must keep meaning "the active release is
  not available" per `AC-CANVASES-AGENT-WEB-APPS-007.1`, and the state drives the title as well as
  the description. `offline` is already taken by `canvas-host-route.tsx:131` for
  `navigator.onLine === false`; overloading it would hide a second cause behind one label.
- **Four causes, four descriptions, one title.** `AC-CANVASES-AGENT-WEB-APPS-007.10` requires the
  timeout to be distinguishable from both guest codes. The iframe load error is free once the union
  exists and is a genuinely different thing to tell a user.
- **Translations ship with this change, against the card's instruction.** The card asks for
  `en/canvases.json` only. Measured against the current tree, that leaves CI red:
  adding one `en` key and running `node apps/web/scripts/check-i18n-keys.mjs` exits 1 with
  "pseudo catalog is out of sync with en (1 missing, 0 extra)" and "4 real-locale catalog issue(s)"
  naming `pt-pt`, `zh-cn`, `zh-hk`, `zh-tw`. The probe key was reverted and the checker returns to
  exit 0. Locale catalogs are documentation-coverage exempt, so this widens no delivery obligation.
- **Guest CSP violations stay out.** Forwarding `securitypolicyviolation` detail would add a new
  guest-controlled string field to the host wire contract, which needs bounding, validation, and a
  security review of rendering untrusted text in host UI. `context_unavailable` already routes an
  investigator to the runtime rather than the release, which is the entire saving this card is
  about. File separately if the blocked-URI detail is wanted.

## ASCII UI preview

`UI-01: Canvas host state panel`. Entry point: a canvas whose active release is valid and whose
application fails its startup handshake. Structure is required; spacing is illustrative. The panel is
`CanvasHostStatePanel`; Retry is the existing button. Desktop and phone share this composition, so
one view covers both.

Current behavior, evidenced by `canvases.json:23-24` and `canvas-host-components.tsx:63`:

```text
+--------------------------------------------------+
|                 Canvas unavailable               |
|   The active canvas release is not available.    |
|                  [ Try again ]                   |
+--------------------------------------------------+
```

Proposed, cause = cannot reach the runtime capability API:

```text
+--------------------------------------------------+
|            Canvas did not start                  |
|   The canvas application could not reach the     |
|   Kandev runtime. Its release is valid.          |
|                  [ Try again ]                   |
+--------------------------------------------------+
```

Proposed, cause = no acknowledgement at the deadline:

```text
+--------------------------------------------------+
|            Canvas did not start                  |
|   The canvas application did not finish          |
|   starting in time. Its release is valid.        |
|                  [ Try again ]                   |
+--------------------------------------------------+
```

A genuinely unavailable release keeps the first drawing unchanged. Final wording is the work order's;
the required structure is one title naming the application, one description naming the cause, and the
existing recovery action. Criteria: `AC-CANVASES-AGENT-WEB-APPS-007.9` for the distinct state and
`AC-CANVASES-AGENT-WEB-APPS-007.10` for the per-cause description.

## Work orders

- [ ] [Task 01: Surface the canvas runtime startup failure cause](task-01-surface-runtime-startup-failure-cause.md)

## Existing E2E assertions on the current copy

Three specs assert the title `Canvas unavailable`, and they do not all exercise the same path.

- `apps/web/e2e/tests/canvas/plugin-canvas.spec.ts:398` fails
  `/api/v1/canvases/<id>/runtime` with 503. That is the host's own capability request, which settles
  through `applyCanvasHostError` and is out of scope, so this assertion should survive unchanged.
- `apps/web/e2e/tests/canvas/canvas-authenticated-proxy.spec.ts:178` and
  `apps/web/e2e/tests/canvas/mobile-canvas-authenticated-proxy.spec.ts:102` clear proxy
  authentication and reload. Either the canvas metadata request fails first, keeping the current
  title, or the guest document fails and the new state applies. Which one wins is not decidable by
  reading; the work order runs them and updates the assertion only for the path that actually moves.

## Verification strategy

Component tests in the three existing files, the repository i18n gate, and the two proxy E2E specs
above. Commands are in the work order.

## Risks

- `onError` gaining a parameter is a component-contract change. `CanvasPage` is the only
  `WebAppFrame` consumer and `CanvasHostBody` the only `CanvasPage` consumer, so the blast radius is
  three files; an optional parameter keeps existing call sites compiling.
- A new `CanvasHostState` member must be added to `STATE_COPY`, which is a total
  `Record<CanvasHostState, ...>`; omitting it is a type error rather than a silent gap.
- New copy must avoid U+2014, which `check-no-em-dash-ui.mjs` gates.

## Verification results

Pending implementation.
