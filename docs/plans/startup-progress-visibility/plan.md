---
created: 2026-09-17
status: done
requirements:
  - REQ-PLATFORM-STARTUP-PROGRESS-001
  - REQ-PLATFORM-STARTUP-PROGRESS-002
  - REQ-PLATFORM-STARTUP-PROGRESS-003
  - REQ-PLATFORM-STARTUP-PROGRESS-004
  - REQ-PLATFORM-STARTUP-PROGRESS-005
  - REQ-PLATFORM-STARTUP-LIFECYCLE-001
system_design:
  - ../../specs/platform/system-design/startup-progress-visibility.md
  - ../../specs/platform/system-design/startup-lifecycle.md
legacy_specs: []
---

# Implementation Plan: Startup progress visibility

## Overview

On 2026-09-17, an incident left the backend apparently hung during startup
with no way to distinguish stuck from slow: two large migrations ran with no
progress reporting at all. This initiative adds a protocol-owned `Step` layer
below the existing startup `Phase`, a stable `/ready` snapshot contract, and
step-level rendering on every startup-visible surface except the desktop
shell (an authorized, separately tracked deferral).

## Scope

### In scope

- A step registry of seven normative identifiers, each declaring phase,
  measure (`opaque`/`counting`/`counted`), unit, label key, and applicable
  dialects. Task 01 shipped ten; upstream `2eee90d38` deleted the work three of
  them measured, and the spec was amended on 2026-09-18 to retire those three
  (AC-PLATFORM-STARTUP-PROGRESS-005.3 and 005.6). Task 02 carries that out.
- Lifecycle API (`BeginStep`/`EndStep`/`SeedDone`/`SetTotal`/`Advance`/
  `Degrade`), 30s trailing-window rate, ETA, and a 120s stall threshold.
- `GET /ready` carrying the snapshot in both its `starting` 503 and `ready`
  200 bodies, never 404.
- RFC 7231 Accept-header content negotiation for the Go-rendered startup
  page, the page itself, the Settings restart dialog's step-level detail,
  and the launcher's stdout reporting.
- Full i18n coverage (backend + web, all five locales plus pseudo) for every
  step label key.
- Amending `AC-PLATFORM-STARTUP-LIFECYCLE-001.4` in place to cite the new
  step-level percentage contract.

### Out of scope

- The desktop status bridge — deferred to follow-up card
  `c99c6111-e982-4313-b165-5c9b8aa03e46`, the only authorized deferral in
  this initiative's history.
- Making startup faster, or cancelling/skipping/retrying/reordering startup
  work.
- Progress for work after readiness (background maintenance, retention
  sweeps).

## Technical approach

### Step registry and lifecycle

`internal/startup` gains a step registry declaring, per identifier: phase,
measure, unit, label key, applicable dialects, and — where applicable — a
corpus, an outstanding-work predicate, or ordering/tiebreak columns. A step
intending `counted` reports `opaque` until `SetTotal` promotes it; `SeedDone`
is valid only before that promotion. Steps never nest — every apparent
nesting bug found during spec review was a call-site sequencing defect, fixed
by relocating call sites rather than by adding nesting support.

### Store-admission sweeps

Two sweep steps (`stores.repositories`, `stores.services`) count every
required persisted store exactly once; `internal/persistence/requiredstores`
carries an automated completeness check that fails if an entry is counted by
zero or more than one sweep.

### `/ready` contract and content negotiation

`GET /ready` answers from listener bind, carrying the snapshot in both its
503 (`starting`) and 200 (`ready`) bodies. `internal/webapp/accept.go` parses
the `Accept` header into media ranges and resolves whether `text/html` clears
a strictly higher quality than `application/json` before serving the
Go-rendered startup page instead of JSON.

### Surface rendering

The Go-rendered startup page (`internal/backendapp/startup_page.go`) polls
`/ready` every second and reloads once ready. The web Settings restart
dialog (`apps/web/components/settings/system/restart-progress-dialog.tsx`,
`apps/web/hooks/domains/system/use-startup-progress.ts`) polls the same
endpoint independently of the existing boot-id completion polling. The
launcher (`internal/launcher/health.go`) writes one stdout line per observed
phase/sequence/stall change, plus a periodic floor line while none change.
All three surfaces render the same stall state after 120s without an
advance.

## Tests

- `AC-PLATFORM-STARTUP-PROGRESS-001.*`: `internal/startup/step_lifecycle_test.go`,
  `step_snapshot_test.go`, `registry_completeness_test.go`.
- `AC-PLATFORM-STARTUP-PROGRESS-002.*`: `internal/backendapp/startup_order_test.go`,
  `httpserver_test.go`.
- `AC-PLATFORM-STARTUP-PROGRESS-003.*`: `internal/webapp/accept_test.go`,
  `internal/backendapp/startup_page_test.go`,
  `apps/web/hooks/domains/system/use-startup-progress.test.ts`,
  `apps/web/components/settings/system/restart-progress-dialog.test.tsx`.
- `AC-PLATFORM-STARTUP-PROGRESS-004.*`: `internal/startup/step_snapshot_test.go`
  (stall math), `internal/launcher/health_startup_line_test.go`.
- `AC-PLATFORM-STARTUP-PROGRESS-005.*`: `internal/startup/registry_completeness_test.go`,
  `internal/persistence/requiredstores/sweep_test.go`.
- `AC-PLATFORM-STARTUP-LIFECYCLE-001.4`: covered by the same
  `internal/startup` snapshot tests plus `startup_order_test.go`.

## E2E tests

- `apps/web/e2e/tests/layout/restart-progress-startup-detail.spec.ts`: restart
  dialog step-level detail across base, last-known, and stalled states.
- `apps/web/e2e/tests/layout/startup-page.spec.ts`: the real Go-rendered
  startup page through a debug-only fixture route, covering initial render,
  poll-driven re-render, and reload-on-ready.

## Work orders

- [x] [Task 01: Startup step registry, /ready contract, and surface rendering](task-01-step-registry-and-surfaces.md) (done)
- [x] [Task 02: Retire the three step identifiers whose work upstream deleted](task-02-retire-deleted-step-identifiers.md) (done)

## Verification results

- Full backend gauntlet (`make fmt`, `make typecheck`, `make lint`,
  `make lint-format`) clean; `make test`'s unrelated-package failures
  confirmed pre-existing/environmental via a merge-base comparison.
- `pnpm run i18n:ratchet` clean (4 added + 1 modified locale file).
- Targeted e2e run (`restart-progress-startup-detail.spec.ts`,
  `startup-page.spec.ts`, chromium project): 3/3 passed.
- Independent adversarial review (four parallel review legs plus a
  cross-vendor outside-voice pass): no production-bug residual survives
  verification; all open items are test-rigor gaps on already-correct code,
  tracked as non-blocking.

## Risks

- Reporting must stay best-effort: a reporting failure must never fail
  startup itself.
- Every rendering surface must derive stall/progress state from the same
  snapshot fields to avoid surfaces disagreeing about whether startup is
  stuck.
- The desktop deferral's scope must stay visible in the shipped system-design
  doc's prose so a future reader does not mistake it for a finished surface.
