---
created: 2026-09-21
status: done
requirements:
  - REQ-PLUGINS-ISOLATED-WEB-APPS-012
  - REQ-PLUGINS-ISOLATED-WEB-APPS-013
  - REQ-CANVASES-AGENT-WEB-APPS-001
  - REQ-CANVASES-AGENT-WEB-APPS-006
system_design:
  - ../../specs/plugins/system-design/isolated-web-app-contributions.md
  - ../../specs/plugins/system-design/runtime-response-preservation.md
  - ../../specs/canvases/system-design/agent-authored-web-apps.md
  - ../../specs/canvases/system-design/task-entry-presentation.md
legacy_specs: []
---

# Implementation Plan: Canvas runtime and task-entry recovery

## Overview

Prevent automatic proxy injection into canvas responses. Then show unseen
published canvases when users return to their tasks. Both changes reuse the
existing runtime and host surfaces.

## Evidence and diagnosis

The incident involved task `f6136adf-46a6-422f-831c-5a35caf04b3d` and canvas
`09549bca-9ad4-4169-9d4f-2913e8bfc1b3`. The author reported active release
`21bf0113-73fe-42c8-8b11-9bbf4ca0db27` as valid.

Backend logs recorded `canvas.release.activated` at 10:19:28 UTC on September 21.
Browser logs recorded task restoration without its canvas panel at 13:14:02 UTC.
The user confirmed that they returned after publication.

The screenshot showed Canvas unavailable. The reported console error identified
a CSP-blocked Cloudflare beacon. The exact embedded startup JavaScript reproduced
the failure mechanism in an isolated Node VM. Without a resource error, it
requested context once and reported Ready. A simulated beacon script error
reported `failed/document_error` and made no context request.

This is a source-level reproduction, not a capture of the live frame handshake.
The diagnostic archive was partial and lacked publication-time browser events.
The source nonetheless establishes both relevant paths:

- `hostRuntimeBootstrap` rejects resource errors. `setRuntimeHeaders` currently
  sends only `no-store`, leaving no explicit prohibition on proxy transformation.
- `useTaskCanvasLifecycleActivation` depends on volatile lifecycle hints. Task
  entry does not reconcile the published HTTP inventory with the workbench.

The runtime design already prohibits proxy HTML injection. Preserve that
contract. Cloudflare documents the transformation directive in its
[Web Analytics FAQ](https://developers.cloudflare.com/web-analytics/faq/).
The first-release requirement covers live publication. New criteria
`AC-CANVASES-AGENT-WEB-APPS-001.13` through `.16` define later task entry.

## Scope

### In scope

- Runtime `no-transform` response policy and realistic HTTPS proxy coverage.
- HTTP inventory reconciliation on task entry, reload, and reconnect.
- Main editor placement, one focus change, and browser-tab dismissal memory.
- Phone route behavior and prevention of Back-navigation loops.
- Operator and user documentation in `docs/public/canvases.md`.

### Out of scope

- Live Cloudflare configuration changes or live canvas republishing.
- CSP relaxation, ignored script errors, new grants, or new runtime flags.
- Cross-device presentation history or changes to canvas package storage.
- Claims that this investigation verified the live deployment after correction.

## Technical approach

Task 01 changes `setRuntimeHeaders` in the shared plugin runtime. Extend the
existing authenticated proxy fixture with directive-aware HTML injection.
Its negative control removes `no-transform` and demonstrates the existing failure.
Keep package bytes, capability checks, cookie forwarding, and SSE intact.

Task 02 shares the task-page canvas inventory with the activation hook.
Reconcile only after the target environment finishes layout restoration.
Use current HTTP metadata for every candidate. Record offered canvases after
presentation, following the existing PR-panel session-storage pattern.
Receipt identity includes user, workspace, task, and canvas, not release.
Manual opening and restored panels also record presentation.

Desktop adds every eligible unseen canvas and focuses only the deterministic
last candidate. Existing panels never move. Phones open one candidate and mark
the batch offered after confirmed navigation. Other candidates remain in the picker.
The nearest phone exemplars are `task-layout.tsx` and `CanvasHostFrame`.
Their full-height route suits primary content and retains existing scroll ownership,
safe areas, accessible controls, and touch targets.

Related completed packages remain historical evidence:
[same-origin authentication](../canvas-same-origin-auth/plan.md) and
[first-release UX](../plugin-backed-canvases-ux-follow-up/plan.md).
Their existing happy-path scenarios must remain covered alongside these regressions.
No new ADR is needed: this preserves the runtime boundary and extends the existing
tab-local offered-marker pattern. Specifications retain the local rationale.

## ASCII UI preview

UI-01: Returning to a task after publication, desktop.

```text
Before: main editor [Agent] [Plan] [+]
        Canvas requires manual selection.

After:  main editor [Agent] [Plan] [Task coordinator *] [+]
        +-------------------------------------------+
        | Existing canvas toolbar                   |
        | Canvas application fills remaining space  |
        +-------------------------------------------+
```

UI-02: Returning to the same task, phone.

```text
+--------------------------------+
| Existing navigation | Canvas   |
| Existing host actions          |
|                                |
| Focused canvas application     |
| One content scroll region      |
+--------------------------------+
Back -> task, without redirecting again
```

The main editor group, one focused canvas, and existing phone route are required.
Spacing and titles are illustrative. No new toolbar or visible copy is required.
Permission-review canvases show the existing review state before execution.
Real runtime failures keep the existing unavailable state and Retry action.

## Tests

- `AC-PLUGINS-ISOLATED-WEB-APPS-013.8`: extend `runtime_test.go` with
  `TestRuntimeNoTransformHeaders` for entry HTML, nested assets, and bootstrap.
- `AC-PLUGINS-ISOLATED-WEB-APPS-012.1`: preserve startup error tests and the
  injected-script negative control.
- `AC-CANVASES-AGENT-WEB-APPS-001.2`, `.13` through `.16`: extend
  `dockview-canvas-activation.test.tsx`, `use-task-canvases.test.ts`, and a
  focused receipt-storage test. Include mixed eligible and ineligible inventory.

## E2E tests

- Desktop and phone authenticated-proxy specs prove retained runtime startup
  with transformation prevention, plus the negative control on desktop.
- Desktop and phone plugin-canvas specs prove publication before navigation,
  reload, receipt persistence, manual reopening, and no focus/navigation loops.
- Desktop also proves main editor placement after delayed layout restoration.
- Feature-disabled, wrong-task, unavailable, and permission-review paths retain
  focused unit/component coverage and existing rendered scenarios.

## Work orders

- [x] [Task 01: Preserve runtime responses through proxies](task-01-runtime-responses.md)
- [x] [Task 02: Reconcile canvases on task entry](task-02-task-entry.md)

Execute sequentially. Each work order contains exact commands and owns its tests.
No delegation is authorized.

## Verification results

Planning: the isolated bootstrap probe reproduced the failure mechanism.
Implementation and rendered verification are complete. Backend runtime tests,
focused web tests, full web lint, i18n checks, web typecheck, and the managed
desktop and phone E2E suites passed. `make -C apps/backend build` and
`pnpm --filter @kandev/web build:vite` passed. The authenticated proxy suites
passed with 5 desktop and 2 phone tests; the task-entry suites passed with 6
desktop and 4 phone tests. The focused component and hook suite passed 61 tests,
including deferred request invalidation, authenticated identity, mixed inventory,
delayed layout ownership, batch focus, restored-panel receipts, and duplicated-tab
receipt isolation. Full web lint
passed with zero warnings. `pnpm run i18n:check` passed; it reported the
repository's existing orphan catalog warnings without failing. Public-doc validation passed 62 tests and
validated 47 pages. `python3 scripts/list-docs.py validate` passed with 294
decisions and 1068 specifications, `python3 scripts/lint-spec-files.py --all`
passed, and `python3 scripts/lint-spec-files.test.py` passed all 36 tests.
`git diff --check` passed.

## Risks

- A proxy can override origin headers. The real deployment still needs an
  operator smoke check, and can require a runtime-path analytics exclusion.
- Old canvases have no presentation receipts. The first corrected task visit
  can offer them once. Existing panels count as already presented.
- Blocked session storage falls back to memory. Reload persistence then cannot
  be guaranteed, but manual access must continue to work.
- Async restoration and task switches can misplace panels unless generation
  and environment checks occur immediately before insertion.
