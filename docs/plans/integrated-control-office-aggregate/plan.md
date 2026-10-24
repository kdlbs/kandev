---
created: 2026-10-20
status: in-progress
requirements:
  - REQ-OFFICE-WORKSPACE-OVERVIEW-001
  - REQ-UI-INTEGRATED-CONTROL-002
  - REQ-UI-INTEGRATED-CONTROL-003
acceptance_criteria:
  - AC-OFFICE-WORKSPACE-OVERVIEW-001.1
  - AC-OFFICE-WORKSPACE-OVERVIEW-001.2
  - AC-OFFICE-WORKSPACE-OVERVIEW-001.3
  - AC-OFFICE-WORKSPACE-OVERVIEW-001.4
  - AC-OFFICE-WORKSPACE-OVERVIEW-001.5
  - AC-OFFICE-WORKSPACE-OVERVIEW-001.6
  - AC-UI-INTEGRATED-CONTROL-002.1
  - AC-UI-INTEGRATED-CONTROL-002.2
  - AC-UI-INTEGRATED-CONTROL-002.3
  - AC-UI-INTEGRATED-CONTROL-002.4
  - AC-UI-INTEGRATED-CONTROL-002.5
  - AC-UI-INTEGRATED-CONTROL-002.6
  - AC-UI-INTEGRATED-CONTROL-003.1
  - AC-UI-INTEGRATED-CONTROL-003.2
  - AC-UI-INTEGRATED-CONTROL-003.3
  - AC-UI-INTEGRATED-CONTROL-003.4
  - AC-UI-INTEGRATED-CONTROL-003.5
system_design:
  - ../../specs/office/system-design/workspace-overview.md
  - ../../specs/ui/system-design/integrated-control-navigation.md
legacy_specs: []
---

# Integrated control Office aggregate plan

Ship the read-only Office multi-workspace aggregate API together with the
integrated control dashboard panels that consume it. This supersedes the
standalone `/office/overview` page proposal: the API, its route guard, batched
repository reads, and their tests carry over; the standalone page, its store
slice, navigation entry, and browser tests do not.

The dashboard stays behind the existing `features.integratedControl` flag
(default off) and reads the aggregate only when `features.office` is on.

## Work order

1. [Task 01: Office workspace aggregate API](task-01-office-workspace-aggregate-api.md)
2. [Task 02: Integrated control aggregate dashboard](task-02-control-aggregate-dashboard.md)

Task 02 depends on Task 01's response shape.

Task 02's Native comparison follow-up also ports the recorded dashboard
projection on the existing workspace GET, together with its dashboard-only
reader and consumer. It preserves the Office aggregate rather than relabeling
agents as sessions. It does not depend on capacity/provider/conversation work.

## Not in this plan

Panels that need data the aggregate does not return stay out: execution grade
and capacity bars, provider usage limits, per-model activity, per-task stall
signals (queued messages without a running session, failed primary sessions,
repeated step moves), and recorded control history. Adding any of them needs its
own read and requirement.

## Verification strategy

- Backend: `go test ./internal/office/dashboard ./internal/office/repository/sqlite` and the `TestOfficeScope*`/`TestOfficeRoute*` cases in `./internal/backendapp`.
- Web: vitest for the signals, the hook, the dashboard wiring, and the activity row; typecheck, lint, and `pnpm run i18n:check`.
- Browser: `tests/coordinator/control-aggregate-dashboard.spec.ts` (chromium) and `tests/coordinator/mobile-control-aggregate-dashboard.spec.ts` (mobile-chrome).
- Docs: `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `validateCoverage` from `.github/scripts/pr-docs.cjs`.
