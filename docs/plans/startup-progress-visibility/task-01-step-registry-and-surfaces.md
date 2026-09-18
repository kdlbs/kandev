---
id: "01-step-registry-and-surfaces"
title: "Startup step registry, /ready contract, and surface rendering"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-STARTUP-PROGRESS-001
  - REQ-PLATFORM-STARTUP-PROGRESS-002
  - REQ-PLATFORM-STARTUP-PROGRESS-003
  - REQ-PLATFORM-STARTUP-PROGRESS-004
  - REQ-PLATFORM-STARTUP-PROGRESS-005
  - REQ-PLATFORM-STARTUP-LIFECYCLE-001
acceptance_criteria:
  - AC-PLATFORM-STARTUP-PROGRESS-001.1
  - AC-PLATFORM-STARTUP-PROGRESS-001.2
  - AC-PLATFORM-STARTUP-PROGRESS-001.3
  - AC-PLATFORM-STARTUP-PROGRESS-001.4
  - AC-PLATFORM-STARTUP-PROGRESS-001.5
  - AC-PLATFORM-STARTUP-PROGRESS-001.6
  - AC-PLATFORM-STARTUP-PROGRESS-001.7
  - AC-PLATFORM-STARTUP-PROGRESS-001.8
  - AC-PLATFORM-STARTUP-PROGRESS-001.9
  - AC-PLATFORM-STARTUP-PROGRESS-001.10
  - AC-PLATFORM-STARTUP-PROGRESS-001.11
  - AC-PLATFORM-STARTUP-PROGRESS-001.12
  - AC-PLATFORM-STARTUP-PROGRESS-001.13
  - AC-PLATFORM-STARTUP-PROGRESS-001.14
  - AC-PLATFORM-STARTUP-PROGRESS-001.15
  - AC-PLATFORM-STARTUP-PROGRESS-001.16
  - AC-PLATFORM-STARTUP-PROGRESS-001.17
  - AC-PLATFORM-STARTUP-PROGRESS-001.18
  - AC-PLATFORM-STARTUP-PROGRESS-001.19
  - AC-PLATFORM-STARTUP-PROGRESS-001.20
  - AC-PLATFORM-STARTUP-PROGRESS-001.21
  - AC-PLATFORM-STARTUP-PROGRESS-001.22
  - AC-PLATFORM-STARTUP-PROGRESS-002.1
  - AC-PLATFORM-STARTUP-PROGRESS-002.2
  - AC-PLATFORM-STARTUP-PROGRESS-002.3
  - AC-PLATFORM-STARTUP-PROGRESS-002.4
  - AC-PLATFORM-STARTUP-PROGRESS-002.5
  - AC-PLATFORM-STARTUP-PROGRESS-002.6
  - AC-PLATFORM-STARTUP-PROGRESS-003.1
  - AC-PLATFORM-STARTUP-PROGRESS-003.2
  - AC-PLATFORM-STARTUP-PROGRESS-003.3
  - AC-PLATFORM-STARTUP-PROGRESS-003.4
  - AC-PLATFORM-STARTUP-PROGRESS-003.5
  - AC-PLATFORM-STARTUP-PROGRESS-003.6
  - AC-PLATFORM-STARTUP-PROGRESS-003.7
  - AC-PLATFORM-STARTUP-PROGRESS-003.8
  - AC-PLATFORM-STARTUP-PROGRESS-003.9
  - AC-PLATFORM-STARTUP-PROGRESS-003.10
  - AC-PLATFORM-STARTUP-PROGRESS-003.11
  - AC-PLATFORM-STARTUP-PROGRESS-003.12
  - AC-PLATFORM-STARTUP-PROGRESS-003.13
  - AC-PLATFORM-STARTUP-PROGRESS-004.1
  - AC-PLATFORM-STARTUP-PROGRESS-004.2
  - AC-PLATFORM-STARTUP-PROGRESS-004.3
  - AC-PLATFORM-STARTUP-PROGRESS-004.4
  - AC-PLATFORM-STARTUP-PROGRESS-004.5
  - AC-PLATFORM-STARTUP-PROGRESS-004.6
  - AC-PLATFORM-STARTUP-PROGRESS-004.7
  - AC-PLATFORM-STARTUP-PROGRESS-004.8
  - AC-PLATFORM-STARTUP-PROGRESS-005.1
  - AC-PLATFORM-STARTUP-PROGRESS-005.2
  - AC-PLATFORM-STARTUP-PROGRESS-005.3
  - AC-PLATFORM-STARTUP-PROGRESS-005.4
  - AC-PLATFORM-STARTUP-PROGRESS-005.5
  - AC-PLATFORM-STARTUP-PROGRESS-005.6
  - AC-PLATFORM-STARTUP-PROGRESS-005.7
  - AC-PLATFORM-STARTUP-LIFECYCLE-001.4
system_design:
  - ../../specs/platform/system-design/startup-progress-visibility.md
  - ../../specs/platform/system-design/startup-lifecycle.md
---

# Task 01: Startup step registry, /ready contract, and surface rendering

## Summary

The 2026-09-17 incident left the backend apparently hung during two large,
unreported startup migrations with no way to tell stuck from slow. This task
adds a `Step` layer below the existing startup `Phase`: a protocol-owned
registry of ten identifiers, each declaring phase, measure
(`opaque`/`counting`/`counted`), unit, label key, and applicable dialects. The
lifecycle API (`BeginStep`/`EndStep`/`SeedDone`/`SetTotal`/`Advance`/`Degrade`)
computes rate over a 30s trailing window, an ETA, and a 120s stall threshold.
`GET /ready` carries the snapshot in both its `starting` 503 and `ready` 200
bodies. Three of the four described rendering surfaces ship in this task: the
Go-rendered localized startup page (with RFC 7231 Accept-header content
negotiation), the web Settings restart dialog widened with step-level detail,
and the launcher's stdout reporting. The fourth, the desktop status bridge, is
an authorized out-of-scope deferral tracked on a separate follow-up card (see
Out of scope).

## In scope

- `internal/startup`: step registry (`step.go`), the ten normative
  identifiers as the contract stood when this task shipped (three were retired
  by the 2026-09-18 spec amendment; see Task 02), lifecycle state machine (`step_lifecycle.go`), and
  rate/ETA/stall snapshot math (`step_snapshot.go`), each best-effort and
  never able to fail startup.
- Two store-admission sweep steps (`stores.repositories`, `stores.services`)
  in `internal/persistence/requiredstores`, each required-store entry counted
  by exactly one sweep, enforced by an automated completeness check.
- `GET /ready`'s stable snapshot contract in both its 503 and 200 bodies.
- `internal/webapp/accept.go`: RFC 7231 Accept-header content negotiation
  deciding `text/html` vs `application/json` for the startup page.
- `internal/backendapp/startup_page.go`: the Go-rendered, localized,
  polling startup page.
- Web surface: `apps/web/hooks/domains/system/use-startup-progress.ts` and
  `apps/web/components/settings/system/restart-progress-dialog.tsx`, widened
  with step-level progress, rate/ETA, and stall detail.
- Launcher stdout reporting (`internal/launcher/health.go`): one line per
  observed phase/sequence/stall change, plus a periodic floor while none of
  those change.
- Backend i18n (`internal/i18n`) and web i18n (`startup.json`) catalogs across
  all five supported locales plus pseudo, for every step label key.
- Amend `AC-PLATFORM-STARTUP-LIFECYCLE-001.4` in place to cite the new
  step-level percentage contract instead of restating it.

## Out of scope

- The desktop status bridge (F76/F77 in this initiative's spec-review
  history) — deferred to follow-up card `c99c6111-e982-4313-b165-5c9b8aa03e46`,
  the only authorized deferral.
- Making startup faster, or cancelling/skipping/retrying/reordering startup
  work — every surface here is read-only.
- Progress for work after readiness (background maintenance, retention
  sweeps).

## Acceptance

- A registered step's snapshot always carries phase, measure, and label key;
  a `counted` step also carries done/total and, once rate is measurable, an
  ETA; an `opaque` step omits both counts and is never reported stalled.
- `GET /ready` never 404s during startup and always carries the current
  snapshot in both its 503 and 200 responses.
- The startup page, the Settings restart dialog, and the launcher's stdout
  all render the same step, phase, progress, and stall state from the same
  snapshot, each localized through `label_key`.
- `PrefersHTML` resolves the startup page only when `text/html` clears a
  strictly higher quality than `application/json`, matching RFC 7231
  media-range specificity and quality-parameter parsing.
- An automated check fails if a required persisted store is counted by zero
  or more than one sweep step, if a registered identifier falls outside the
  normative list of ten, or if a step's label key is missing from any
  supported locale.

## Verification

```bash
cd apps/backend && go test ./internal/startup/... ./internal/webapp/... ./internal/backendapp/... ./internal/persistence/requiredstores/... ./internal/launcher/... ./internal/i18n/... -count=1
cd apps/web && pnpm vitest run hooks/domains/system/use-startup-progress.test.ts components/settings/system/restart-progress-dialog.test.tsx lib/startup-progress
cd apps/web && e2e/scripts/run-e2e.sh --host --no-build --no-strict --shards 1 --project chromium -- e2e/tests/layout/restart-progress-startup-detail.spec.ts e2e/tests/layout/startup-page.spec.ts
```

## Files likely touched

- `apps/backend/internal/startup/step.go`, `step_lifecycle.go`, `step_snapshot.go`, `progress.go`, `duration.go`
- `apps/backend/internal/persistence/requiredstores/catalog.go`, `tracker.go`
- `apps/backend/internal/webapp/accept.go`
- `apps/backend/internal/backendapp/startup_page.go`, `httpserver.go`
- `apps/backend/internal/launcher/health.go`
- `apps/backend/internal/i18n/locales/*.json`
- `apps/web/hooks/domains/system/use-startup-progress.ts`
- `apps/web/components/settings/system/restart-progress-dialog.tsx`
- `apps/web/lib/startup-progress/*.ts`
- `apps/web/src/locales/*/startup.json`
- `apps/web/e2e/tests/layout/restart-progress-startup-detail.spec.ts`, `startup-page.spec.ts`
- `docs/specs/platform/requirements/startup-lifecycle.md` (AC-...-001.4 amendment only)
